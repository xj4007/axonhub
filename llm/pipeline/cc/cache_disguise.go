package cc

import (
	"container/list"
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/pipeline"
	"github.com/looplj/axonhub/llm/streams"
	"github.com/looplj/axonhub/llm/transformer/shared"
	"log/slog"
)

const (
	MIN_CACHE_CREATION int64 = 110

	promptCacheDisguiseName = "claudecode-prompt-cache-disguise"

	defaultConversationStateTTL     = 45 * time.Minute
	defaultConversationStateCleanup = 3 * time.Minute
	defaultConversationStateMax     = 4096

	defaultRequestMarkerTTL     = 10 * time.Minute
	defaultRequestMarkerCleanup = time.Minute

	simulateCacheModeEphemeral5M = "ephemeral_5m_input_tokens"
	simulateCacheModeEphemeral1H = "ephemeral_1h_input_tokens"

	CacheDisguiseEnabledMetadataKey                   = "cc_simulate_cache_enabled"
	CacheDisguiseModeMetadataKey                      = "cc_simulate_cache_mode"
	CacheDisguiseMergeCacheTokensIntoInputMetadataKey = "cc_merge_cache_tokens_into_input"
)

// ConversationState tracks prompt token progression for a conversation.
type ConversationState struct {
	LastPromptTokens int64
	UpdatedAt        time.Time
	Seq              uint64
}

type stateNode struct {
	conversationID string
	state          ConversationState
}

type PromptCacheDisguiseMiddleware = promptCacheDisguiseMiddleware

// StateManager keeps conversation states in memory with TTL cleanup and LRU eviction.
type StateManager struct {
	mu               sync.Mutex
	byConversationID map[string]*list.Element
	lru              *list.List
	ttl              time.Duration
	cleanupInterval  time.Duration
	maxEntries       int
	lastCleanup      time.Time
}

func NewStateManager(ttl, cleanupInterval time.Duration, maxEntries int) *StateManager {
	if ttl <= 0 {
		ttl = defaultConversationStateTTL
	}
	if cleanupInterval <= 0 {
		cleanupInterval = defaultConversationStateCleanup
	}
	if maxEntries <= 0 {
		maxEntries = defaultConversationStateMax
	}

	return &StateManager{
		byConversationID: make(map[string]*list.Element),
		lru:              list.New(),
		ttl:              ttl,
		cleanupInterval:  cleanupInterval,
		maxEntries:       maxEntries,
	}
}

func (m *StateManager) Load(conversationID string, now time.Time) (ConversationState, bool) {
	if conversationID == "" {
		return ConversationState{}, false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.cleanupLocked(now)

	elem, ok := m.byConversationID[conversationID]
	if !ok {
		return ConversationState{}, false
	}

	node := elem.Value.(*stateNode)
	if now.Sub(node.state.UpdatedAt) > m.ttl {
		m.removeElementLocked(elem)
		return ConversationState{}, false
	}

	m.lru.MoveToFront(elem)

	return node.state, true
}

func (m *StateManager) Store(conversationID string, state ConversationState, now time.Time) {
	if conversationID == "" {
		return
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = now
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.cleanupLocked(now)

	if elem, ok := m.byConversationID[conversationID]; ok {
		node := elem.Value.(*stateNode)
		node.state = state
		m.lru.MoveToFront(elem)
	} else {
		elem := m.lru.PushFront(&stateNode{
			conversationID: conversationID,
			state:          state,
		})
		m.byConversationID[conversationID] = elem
	}

	for m.lru.Len() > m.maxEntries {
		m.removeElementLocked(m.lru.Back())
	}
}

// ForgeAndAdvance computes forged usage and atomically advances conversation state.
func (m *StateManager) ForgeAndAdvance(conversationID string, currentPromptTokens int64, now time.Time) (forgedUsage, bool) {
	if conversationID == "" {
		return forgedUsage{}, false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.cleanupLocked(now)

	var (
		state    ConversationState
		hasState bool
	)

	if elem, ok := m.byConversationID[conversationID]; ok {
		node := elem.Value.(*stateNode)
		if now.Sub(node.state.UpdatedAt) > m.ttl {
			m.removeElementLocked(elem)
		} else {
			state = node.state
			hasState = true
			m.lru.MoveToFront(elem)
		}
	}

	forged := forgeUsage(conversationID, currentPromptTokens, state, hasState)

	nextSeq := uint64(1)
	if hasState {
		nextSeq = state.Seq + 1
	}

	nextState := ConversationState{
		LastPromptTokens: currentPromptTokens,
		UpdatedAt:        now,
		Seq:              nextSeq,
	}

	if elem, ok := m.byConversationID[conversationID]; ok {
		node := elem.Value.(*stateNode)
		node.state = nextState
		m.lru.MoveToFront(elem)
	} else {
		elem := m.lru.PushFront(&stateNode{
			conversationID: conversationID,
			state:          nextState,
		})
		m.byConversationID[conversationID] = elem
	}

	for m.lru.Len() > m.maxEntries {
		m.removeElementLocked(m.lru.Back())
	}

	return forged, true
}

func (m *StateManager) cleanupLocked(now time.Time) {
	if !m.lastCleanup.IsZero() && now.Sub(m.lastCleanup) < m.cleanupInterval {
		return
	}

	expireBefore := now.Add(-m.ttl)
	for elem := m.lru.Back(); elem != nil; {
		prev := elem.Prev()
		node := elem.Value.(*stateNode)
		if node.state.UpdatedAt.Before(expireBefore) {
			m.removeElementLocked(elem)
		}
		elem = prev
	}

	m.lastCleanup = now
}

func (m *StateManager) removeElementLocked(elem *list.Element) {
	if elem == nil {
		return
	}

	node := elem.Value.(*stateNode)
	delete(m.byConversationID, node.conversationID)
	m.lru.Remove(elem)
}

type requestMarker struct {
	Eligible                  bool
	Enabled                   bool
	MergeCacheTokensIntoInput bool
	ConversationID            string
	Mode                      string
	IsSubAgent                bool
	IsClaudeCode              bool
	CreatedAt                 time.Time
}

type requestMarkerStore struct {
	mu              sync.Mutex
	byRequestKey    map[string]requestMarker
	ttl             time.Duration
	cleanupInterval time.Duration
	lastCleanup     time.Time
}

func newRequestMarkerStore(ttl, cleanupInterval time.Duration) *requestMarkerStore {
	if ttl <= 0 {
		ttl = defaultRequestMarkerTTL
	}
	if cleanupInterval <= 0 {
		cleanupInterval = defaultRequestMarkerCleanup
	}

	return &requestMarkerStore{
		byRequestKey:    make(map[string]requestMarker),
		ttl:             ttl,
		cleanupInterval: cleanupInterval,
	}
}

func (s *requestMarkerStore) Put(requestKey string, marker requestMarker, now time.Time) {
	if requestKey == "" {
		return
	}

	marker.CreatedAt = now

	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanupLocked(now)
	s.byRequestKey[requestKey] = marker
}

func (s *requestMarkerStore) Take(requestKey string, now time.Time) (requestMarker, bool) {
	if requestKey == "" {
		return requestMarker{}, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanupLocked(now)

	marker, ok := s.byRequestKey[requestKey]
	if !ok {
		return requestMarker{}, false
	}

	delete(s.byRequestKey, requestKey)

	if now.Sub(marker.CreatedAt) > s.ttl {
		return requestMarker{}, false
	}

	return marker, true
}

func (s *requestMarkerStore) cleanupLocked(now time.Time) {
	if !s.lastCleanup.IsZero() && now.Sub(s.lastCleanup) < s.cleanupInterval {
		return
	}

	for requestKey, marker := range s.byRequestKey {
		if now.Sub(marker.CreatedAt) > s.ttl {
			delete(s.byRequestKey, requestKey)
		}
	}

	s.lastCleanup = now
}

type forgedUsage struct {
	InputTokens   int64
	CacheCreation int64
	CacheRead     int64
}

type promptCacheDisguiseMiddleware struct {
	stateManager *StateManager
	markers      *requestMarkerStore
}

var _ pipeline.Middleware = (*promptCacheDisguiseMiddleware)(nil)

func PromptCacheDisguise() pipeline.Middleware {
	return &promptCacheDisguiseMiddleware{
		stateManager: NewStateManager(
			defaultConversationStateTTL,
			defaultConversationStateCleanup,
			defaultConversationStateMax,
		),
		markers: newRequestMarkerStore(
			defaultRequestMarkerTTL,
			defaultRequestMarkerCleanup,
		),
	}
}

func (m *promptCacheDisguiseMiddleware) Name() string {
	return promptCacheDisguiseName
}

func (m *promptCacheDisguiseMiddleware) OnInboundLlmRequest(ctx context.Context, request *llm.Request) (*llm.Request, error) {
	if request == nil {
		return request, nil
	}

	requestKey := requestMarkerKey(ctx)
	if requestKey == "" {
		return request, nil
	}

	now := time.Now().UTC()
	mergeCacheTokensIntoInput := isMergeCacheTokensIntoInputEnabled(request)
	conversationID := extractConversationID(ctx, request)
	if conversationID == "" && !mergeCacheTokensIntoInput {
		return request, nil
	}

	// NOTE: enable/mode is finalized later by orchestrator-selected channel config.
	// We only snapshot stable request signals here to avoid coupling to raw request metadata propagation.
	mode := normalizeSimulateCacheMode(extractSimulateCacheMode(request))
	marker := requestMarker{
		Eligible:                  false,
		Enabled:                   isSimulateCacheEnabled(request),
		MergeCacheTokensIntoInput: mergeCacheTokensIntoInput,
		ConversationID:            conversationID,
		Mode:                      mode,
		IsSubAgent:                isSubAgentRequest(request),
		IsClaudeCode:              isClaudeCodeRequest(request) || looksLikeClaudeCodeRequest(request),
		CreatedAt:                 now,
	}
	marker = m.reconcileMarkerEligibility(ctx, marker)
	m.markers.Put(requestKey, marker, now)

	slog.InfoContext(ctx, "simulate-cache marker captured",
		slog.Bool("enabled", marker.Enabled),
		slog.String("mode", marker.Mode),
		slog.Bool("is_claudecode", marker.IsClaudeCode),
		slog.Bool("is_subagent", marker.IsSubAgent),
		slog.Bool("eligible", marker.Eligible),
		slog.String("conversation_id", marker.ConversationID),
	)

	return request, nil
}

func (m *promptCacheDisguiseMiddleware) OnInboundRawResponse(ctx context.Context, response *httpclient.Response) (*httpclient.Response, error) {
	return response, nil
}
func (m *promptCacheDisguiseMiddleware) OnOutboundRawRequest(ctx context.Context, request *httpclient.Request) (*httpclient.Request, error) {
	if request == nil {
		return request, nil
	}

	requestKey := requestMarkerKey(ctx)
	if requestKey == "" {
		return request, nil
	}

	now := time.Now().UTC()
	marker, ok := m.markers.Take(requestKey, now)
	if !ok {
		return request, nil
	}

	rawMode := normalizeSimulateCacheMode(extractSimulateCacheModeFromMetadata(request.Metadata))
	rawEnabled := isSimulateCacheEnabledFromMetadata(request.Metadata)
	marker.Enabled = rawEnabled
	marker.Mode = rawMode
	marker.MergeCacheTokensIntoInput = isMergeCacheTokensIntoInputEnabledFromMetadata(request.Metadata)
	marker = m.reconcileMarkerEligibility(ctx, marker)

	slog.InfoContext(ctx, "simulate-cache raw-request metadata",
		slog.Bool("raw_enabled", rawEnabled),
		slog.String("raw_mode", rawMode),
		slog.Bool("merge_cache_tokens_into_input", marker.MergeCacheTokensIntoInput),
		slog.Bool("marker_enabled", marker.Enabled),
		slog.String("marker_mode", marker.Mode),
		slog.Bool("is_claudecode", marker.IsClaudeCode),
		slog.Bool("is_subagent", marker.IsSubAgent),
		slog.Bool("eligible", marker.Eligible),
		slog.String("conversation_id", marker.ConversationID),
	)

	m.markers.Put(requestKey, marker, now)
	return request, nil
}

func (m *promptCacheDisguiseMiddleware) OnOutboundRawError(ctx context.Context, err error) {
}

func (m *promptCacheDisguiseMiddleware) OnOutboundRawResponse(ctx context.Context, response *httpclient.Response) (*httpclient.Response, error) {
	return response, nil
}

func (m *promptCacheDisguiseMiddleware) OnOutboundLlmResponse(ctx context.Context, response *llm.Response) (*llm.Response, error) {
	marker, ok := m.takeRequestMarker(ctx)
	if !ok || response == nil || response.Usage == nil {
		return response, nil
	}

	if marker.MergeCacheTokensIntoInput {
		mergeCacheTokensIntoInputUsage(response.Usage)
	}

	if !marker.Eligible {
		slog.InfoContext(ctx, "simulate-cache skipped", slog.String("reason", "marker_not_eligible"))
		return response, nil
	}

	forged, applied := m.forgeAndApply(marker.ConversationID, marker.Mode, response.Usage)
	if applied {
		slog.InfoContext(ctx, "simulate-cache applied",
			slog.String("mode", marker.Mode),
			slog.Int64("forged_input", forged.InputTokens),
			slog.Int64("forged_cache_creation", forged.CacheCreation),
			slog.Int64("forged_cache_read", forged.CacheRead),
			slog.Int64("prompt_tokens", response.Usage.PromptTokens),
		)
	} else {
		slog.InfoContext(ctx, "simulate-cache skipped", slog.String("reason", "forge_failed"))
	}

	return response, nil
}

func (m *promptCacheDisguiseMiddleware) OnOutboundRawStream(ctx context.Context, stream streams.Stream[*httpclient.StreamEvent]) (streams.Stream[*httpclient.StreamEvent], error) {
	return stream, nil
}

func (m *promptCacheDisguiseMiddleware) OnOutboundLlmStream(ctx context.Context, stream streams.Stream[*llm.Response]) (streams.Stream[*llm.Response], error) {
	marker, ok := m.takeRequestMarker(ctx)
	if !ok {
		return stream, nil
	}

	if !marker.Eligible && !marker.MergeCacheTokensIntoInput {
		return stream, nil
	}

	if marker.Eligible && marker.ConversationID == "" && !marker.MergeCacheTokensIntoInput {
		return stream, nil
	}
	return &cacheDisguiseStream{
		inner:                     stream,
		middleware:                m,
		conversationID:            marker.ConversationID,
		mode:                      marker.Mode,
		enableSimulation:          marker.Eligible && marker.ConversationID != "",
		mergeCacheTokensIntoInput: marker.MergeCacheTokensIntoInput,
	}, nil
}

func (m *promptCacheDisguiseMiddleware) takeRequestMarker(ctx context.Context) (requestMarker, bool) {
	requestKey := requestMarkerKey(ctx)
	if requestKey == "" {
		return requestMarker{}, false
	}

	return m.markers.Take(requestKey, time.Now().UTC())
}

func requestMarkerKey(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	if v := reflect.ValueOf(ctx); v.Kind() == reflect.Pointer && v.Pointer() != 0 {
		return fmt.Sprintf("ctx:%x", v.Pointer())
	}

	if sessionID := extractSessionID(ctx); sessionID != "" {
		return "sid:" + sessionID
	}

	return ""
}

func (m *promptCacheDisguiseMiddleware) forgeAndApply(conversationID, mode string, usage *llm.Usage) (forgedUsage, bool) {
	if conversationID == "" || usage == nil {
		return forgedUsage{}, false
	}

	now := time.Now().UTC()
	currentPromptTokens := usage.PromptTokens

	forged, ok := m.stateManager.ForgeAndAdvance(conversationID, currentPromptTokens, now)
	if !ok {
		return forgedUsage{}, false
	}

	applyForgedUsage(usage, mode, forged)

	return forged, true
}

func forgeUsage(conversationID string, currentPromptTokens int64, state ConversationState, hasState bool) forgedUsage {
	if currentPromptTokens <= 0 {
		return forgedUsage{}
	}

	if !hasState {
		cacheCreation := currentPromptTokens * 9 / 10
		inputTokens := currentPromptTokens - cacheCreation

		return normalizedForgedUsage(currentPromptTokens, inputTokens, cacheCreation, 0)
	}

	last := state.LastPromptTokens
	if currentPromptTokens < last {
		cacheCreation := currentPromptTokens / 10
		cacheRead := currentPromptTokens - cacheCreation

		return normalizedForgedUsage(currentPromptTokens, 0, cacheCreation, cacheRead)
	}

	delta := currentPromptTokens - last
	if delta < MIN_CACHE_CREATION {
		return normalizedForgedUsage(currentPromptTokens, delta, 0, last)
	}

	inputTokens := deterministicRandomIntN(conversationID, state.Seq+1, 6)
	cacheCreation := delta - inputTokens
	cacheRead := last

	return normalizedForgedUsage(currentPromptTokens, inputTokens, cacheCreation, cacheRead)
}

func normalizedForgedUsage(totalPrompt, inputTokens, cacheCreation, cacheRead int64) forgedUsage {
	if totalPrompt < 0 {
		totalPrompt = 0
	}
	if inputTokens < 0 {
		inputTokens = 0
	}
	if cacheCreation < 0 {
		cacheCreation = 0
	}
	if cacheRead < 0 {
		cacheRead = 0
	}

	sum := inputTokens + cacheCreation + cacheRead

	if sum > totalPrompt {
		overflow := sum - totalPrompt

		if inputTokens >= overflow {
			inputTokens -= overflow
			overflow = 0
		} else {
			overflow -= inputTokens
			inputTokens = 0
		}

		if overflow > 0 {
			if cacheCreation >= overflow {
				cacheCreation -= overflow
				overflow = 0
			} else {
				overflow -= cacheCreation
				cacheCreation = 0
			}
		}

		if overflow > 0 {
			if cacheRead >= overflow {
				cacheRead -= overflow
			} else {
				cacheRead = 0
			}
		}
	} else if sum < totalPrompt {
		inputTokens += totalPrompt - sum
	}

	return forgedUsage{
		InputTokens:   inputTokens,
		CacheCreation: cacheCreation,
		CacheRead:     cacheRead,
	}
}

func deterministicRandomIntN(conversationID string, seq uint64, n int64) int64 {
	if n <= 0 {
		return 0
	}

	return int64(deterministicRandomUint64(conversationID, seq) % uint64(n))
}

func deterministicRandomUint64(conversationID string, seq uint64) uint64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(conversationID))

	var seqBytes [8]byte
	binary.LittleEndian.PutUint64(seqBytes[:], seq)
	_, _ = hasher.Write(seqBytes[:])

	return hasher.Sum64()
}

func applyForgedUsage(usage *llm.Usage, mode string, forged forgedUsage) {
	if usage == nil {
		return
	}

	if usage.PromptTokensDetails == nil {
		usage.PromptTokensDetails = &llm.PromptTokensDetails{}
	}

	mode = normalizeSimulateCacheMode(mode)

	usage.PromptTokens = forged.InputTokens + forged.CacheCreation + forged.CacheRead
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens

	usage.PromptTokensDetails.CachedTokens = forged.CacheRead
	usage.PromptTokensDetails.WriteCachedTokens = forged.CacheCreation
	usage.PromptTokensDetails.WriteCached5MinTokens = 0
	usage.PromptTokensDetails.WriteCached1HourTokens = 0

	switch mode {
	case simulateCacheModeEphemeral1H:
		usage.PromptTokensDetails.WriteCached1HourTokens = forged.CacheCreation
	default:
		usage.PromptTokensDetails.WriteCached5MinTokens = forged.CacheCreation
	}
}

func mergeCacheTokensIntoInputUsage(usage *llm.Usage) {
	if usage == nil {
		return
	}

	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	if usage.PromptTokensDetails == nil {
		return
	}

	usage.PromptTokensDetails.CachedTokens = 0
	usage.PromptTokensDetails.WriteCachedTokens = 0
	usage.PromptTokensDetails.WriteCached5MinTokens = 0
	usage.PromptTokensDetails.WriteCached1HourTokens = 0
}

func isEligibleForCacheDisguise(request *llm.Request) bool {
	if request == nil {
		return false
	}

	if !isSimulateCacheEnabled(request) {
		return false
	}

	return isClaudeCodeRequest(request) && !isSubAgentRequest(request)
}

func isSimulateCacheEnabled(request *llm.Request) bool {
	if request == nil || len(request.Metadata) == 0 {
		return false
	}

	enabled := strings.TrimSpace(strings.ToLower(request.Metadata[CacheDisguiseEnabledMetadataKey]))
	return enabled == "true"
}

func isMergeCacheTokensIntoInputEnabled(request *llm.Request) bool {
	if request == nil || len(request.Metadata) == 0 {
		return false
	}

	enabled := strings.TrimSpace(strings.ToLower(request.Metadata[CacheDisguiseMergeCacheTokensIntoInputMetadataKey]))
	return enabled == "true"
}

func extractSimulateCacheMode(request *llm.Request) string {
	if request == nil || len(request.Metadata) == 0 {
		return ""
	}

	return request.Metadata[CacheDisguiseModeMetadataKey]
}

func normalizeSimulateCacheMode(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case simulateCacheModeEphemeral1H:
		return simulateCacheModeEphemeral1H
	default:
		return simulateCacheModeEphemeral5M
	}
}

func isClaudeCodeRequest(request *llm.Request) bool {
	if request == nil {
		return false
	}

	for _, msg := range request.Messages {
		if msg.Role != "system" {
			continue
		}

		if msg.Content.Content != nil && containsClaudeCodeBillingHeader(*msg.Content.Content) {
			return true
		}

		for _, part := range msg.Content.MultipleContent {
			if part.Type != "text" || part.Text == nil {
				continue
			}
			if containsClaudeCodeBillingHeader(*part.Text) {
				return true
			}
		}
	}

	return false
}

func containsClaudeCodeBillingHeader(text string) bool {
	normalized := strings.TrimSpace(strings.ToLower(text))
	if normalized == "" {
		return false
	}

	if !strings.HasPrefix(normalized, billingHeaderPrefix) {
		return false
	}

	return strings.Contains(normalized, "cc_version=")
}

func isSubAgentRequest(request *llm.Request) bool {
	if request == nil {
		return false
	}

	if !strings.Contains(strings.ToLower(request.Model), "haiku") {
		return false
	}

	missingTools := len(request.Tools) == 0
	missingSystem := !hasSystemPrompt(request)

	return missingTools || missingSystem
}

func hasSystemPrompt(request *llm.Request) bool {
	if request == nil {
		return false
	}

	for _, msg := range request.Messages {
		if msg.Role != "system" {
			continue
		}

		if msg.Content.Content != nil && strings.TrimSpace(*msg.Content.Content) != "" {
			return true
		}

		for _, part := range msg.Content.MultipleContent {
			if part.Type != "text" || part.Text == nil {
				continue
			}
			if strings.TrimSpace(*part.Text) != "" {
				return true
			}
		}
	}

	return false
}

func extractSessionID(ctx context.Context) string {
	if sessionID, ok := shared.GetSessionID(ctx); ok {
		sessionID = strings.TrimSpace(sessionID)
		if sessionID != "" {
			return sessionID
		}
	}

	return ""
}

func extractConversationID(ctx context.Context, request *llm.Request) string {
	if request != nil {
		if conversationID := extractConversationIDFromMetadata(request.Metadata); conversationID != "" {
			return conversationID
		}

		if request.RawRequest != nil {
			if conversationID := extractConversationIDFromHeaders(request.RawRequest.Headers); conversationID != "" {
				return conversationID
			}

			if requestID := strings.TrimSpace(request.RawRequest.RequestID); requestID != "" {
				return requestID
			}
		}
	}

	if sessionID := extractSessionID(ctx); sessionID != "" {
		return sessionID
	}

	return ""
}

func extractConversationIDFromMetadata(metadata map[string]string) string {
	if len(metadata) == 0 {
		return ""
	}

	for _, key := range []string{
		"thread_id",
		"threadId",
		"trace_id",
		"traceId",
		"session_id",
		"sessionId",
		"request_id",
		"requestId",
	} {
		if id := strings.TrimSpace(metadata[key]); id != "" {
			return id
		}
	}

	if userID := strings.TrimSpace(metadata["user_id"]); userID != "" {
		if sessionID := extractSessionFromClaudeUserID(userID); sessionID != "" {
			return sessionID
		}

		return userID
	}

	return ""
}

func extractConversationIDFromHeaders(headers http.Header) string {
	if len(headers) == 0 {
		return ""
	}

	for _, key := range []string{"AH-Thread-Id", "AH-Trace-Id", "Session_id"} {
		if value := strings.TrimSpace(headers.Get(key)); value != "" {
			return value
		}
	}

	return ""
}

func extractSessionFromClaudeUserID(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ""
	}

	lower := strings.ToLower(userID)
	index := strings.LastIndex(lower, "_session_")
	if index == -1 {
		return ""
	}

	sessionID := strings.TrimSpace(userID[index+len("_session_"):])
	if sessionID == "" {
		return ""
	}

	return sessionID
}

type cacheDisguiseStream struct {
	inner                     streams.Stream[*llm.Response]
	middleware                *promptCacheDisguiseMiddleware
	conversationID            string
	mode                      string
	enableSimulation          bool
	mergeCacheTokensIntoInput bool
	seenUsage                 bool
	lastRawPrompt             int64
	lastForged                forgedUsage
}

func (s *cacheDisguiseStream) Next() bool {
	return s.inner.Next()
}

func (s *cacheDisguiseStream) Current() *llm.Response {
	response := s.inner.Current()
	if response == nil || response.Usage == nil {
		return response
	}

	if s.mergeCacheTokensIntoInput {
		mergeCacheTokensIntoInputUsage(response.Usage)
	}

	if !s.enableSimulation {
		return response
	}

	rawPromptTokens := response.Usage.PromptTokens
	if s.seenUsage && rawPromptTokens == s.lastRawPrompt {
		applyForgedUsage(response.Usage, s.mode, s.lastForged)
		return response
	}

	forged, applied := s.middleware.forgeAndApply(s.conversationID, s.mode, response.Usage)
	if !applied {
		return response
	}

	s.seenUsage = true
	s.lastRawPrompt = rawPromptTokens
	s.lastForged = forged

	return response
}

func (s *cacheDisguiseStream) Err() error {
	return s.inner.Err()
}

func (s *cacheDisguiseStream) Close() error {
	return s.inner.Close()
}

func (m *promptCacheDisguiseMiddleware) reconcileMarkerEligibility(ctx context.Context, marker requestMarker) requestMarker {
	// Eligibility is computed from stable marker fields updated by inbound snapshot and orchestrator-selected channel config.
	_ = ctx
	marker.Eligible = marker.Enabled && marker.IsClaudeCode && !marker.IsSubAgent
	return marker
}

func extractLlmRequestFromCtx(ctx context.Context) *llm.Request {
	_ = ctx
	return nil
}

func (m *promptCacheDisguiseMiddleware) UpdateRequestMarker(requestKey string, enabled bool, mode string, mergeCacheTokensIntoInput bool) {
	if requestKey == "" {
		return
	}

	now := time.Now().UTC()
	marker, ok := m.markers.Take(requestKey, now)
	if !ok {
		return
	}

	marker.Enabled = enabled
	marker.Mode = normalizeSimulateCacheMode(mode)
	marker.MergeCacheTokensIntoInput = mergeCacheTokensIntoInput
	marker = m.reconcileMarkerEligibility(context.Background(), marker)

	slog.Info("simulate-cache marker updated",
		slog.Bool("enabled", marker.Enabled),
		slog.String("mode", marker.Mode),
		slog.Bool("merge_cache_tokens_into_input", marker.MergeCacheTokensIntoInput),
		slog.Bool("is_claudecode", marker.IsClaudeCode),
		slog.Bool("is_subagent", marker.IsSubAgent),
		slog.Bool("eligible", marker.Eligible),
		slog.String("conversation_id", marker.ConversationID),
	)

	m.markers.Put(requestKey, marker, now)
}

// looksLikeClaudeCodeRequest is a conservative fallback that does not depend on billing header text.
// It is intentionally strict to minimize false positives.
func looksLikeClaudeCodeRequest(request *llm.Request) bool {
	if request == nil {
		return false
	}
	if len(request.Tools) == 0 || !hasSystemPrompt(request) {
		return false
	}
	lowerModel := strings.ToLower(strings.TrimSpace(request.Model))
	return strings.HasPrefix(lowerModel, "claude-")
}

type llmRequestContextKey struct{}

func isSimulateCacheEnabledFromMetadata(metadata map[string]string) bool {
	if len(metadata) == 0 {
		return false
	}
	enabled := strings.TrimSpace(strings.ToLower(metadata[CacheDisguiseEnabledMetadataKey]))
	return enabled == "true"
}

func isMergeCacheTokensIntoInputEnabledFromMetadata(metadata map[string]string) bool {
	if len(metadata) == 0 {
		return false
	}
	enabled := strings.TrimSpace(strings.ToLower(metadata[CacheDisguiseMergeCacheTokensIntoInputMetadataKey]))
	return enabled == "true"
}

func extractSimulateCacheModeFromMetadata(metadata map[string]string) string {
	if len(metadata) == 0 {
		return ""
	}
	return metadata[CacheDisguiseModeMetadataKey]
}
