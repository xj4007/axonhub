package claudecode

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/looplj/axonhub/llm"
)

const claudeCodeBillingCCHMetadataKey = "claudecode_billing_cch"

// userIDPattern matches Claude Code format: user_[64-hex]_account__session_[uuid-v4].
var userIDPattern = regexp.MustCompile(`^user_[a-fA-F0-9]{64}_account__session_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// generateFakeUserID generates a fake user ID in Claude Code format.
// Format: user_[64-hex-chars]_account__session_[UUID-v4].
func generateFakeUserID() string {
	hexBytes := make([]byte, 32)
	_, _ = rand.Read(hexBytes)
	hexPart := hex.EncodeToString(hexBytes)
	uuidPart := uuid.New().String()

	return "user_" + hexPart + "_account__session_" + uuidPart
}

// isValidUserID checks if a user ID matches Claude Code format.
func isValidUserID(userID string) bool {
	return userIDPattern.MatchString(userID)
}

// injectFakeUserIDStructured generates and injects a fake user ID into the request metadata.
func injectFakeUserIDStructured(llmReq *llm.Request) *llm.Request {
	if llmReq.Metadata == nil {
		llmReq.Metadata = make(map[string]string)
	}

	existingUserID := llmReq.Metadata["user_id"]
	if existingUserID == "" || !isValidUserID(existingUserID) {
		llmReq.Metadata["user_id"] = generateFakeUserID()
	}

	return llmReq
}

// extractAndRemoveBetas extracts the "betas" array from the body and removes it.
// Returns the extracted betas as a string slice and the modified body.
func extractAndRemoveBetas(body []byte) ([]string, []byte) {
	betasResult := gjson.GetBytes(body, "betas")
	if !betasResult.Exists() {
		return nil, body
	}

	var betas []string

	if betasResult.IsArray() {
		for _, item := range betasResult.Array() {
			if s := strings.TrimSpace(item.String()); s != "" {
				betas = append(betas, s)
			}
		}
	} else if s := strings.TrimSpace(betasResult.String()); s != "" {
		betas = append(betas, s)
	}

	body, _ = sjson.DeleteBytes(body, "betas")

	return betas, body
}

// disableThinkingIfToolChoiceForcedStructured clears ReasoningEffort when tool_choice forces tool use.
// Anthropic API does not allow thinking when tool_choice is "any" or a specific named tool.
// See: https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking#important-considerations
// This operates on the structured llm.Request before it's serialized by the base transformer.
func disableThinkingIfToolChoiceForcedStructured(llmReq *llm.Request) *llm.Request {
	if llmReq.ToolChoice == nil {
		return llmReq
	}

	forcesToolUse := false

	if llmReq.ToolChoice.ToolChoice != nil {
		if *llmReq.ToolChoice.ToolChoice == "any" {
			forcesToolUse = true
		}
	} else if llmReq.ToolChoice.NamedToolChoice != nil {
		if llmReq.ToolChoice.NamedToolChoice.Type == "tool" {
			forcesToolUse = true
		}
	}

	if forcesToolUse && llmReq.ReasoningEffort != "" {
		reqCopy := *llmReq
		reqCopy.ReasoningEffort = ""
		reqCopy.ReasoningBudget = nil

		return &reqCopy
	}

	return llmReq
}

// applyClaudeToolPrefixStructured adds a prefix to all tool names in the request.
func applyClaudeToolPrefixStructured(llmReq *llm.Request, prefix string) *llm.Request {
	if prefix == "" {
		return llmReq
	}

	// Prefix tool names in tools array
	for i := range llmReq.Tools {
		if !strings.HasPrefix(llmReq.Tools[i].Function.Name, prefix) {
			llmReq.Tools[i].Function.Name = prefix + llmReq.Tools[i].Function.Name
		}
	}

	// Prefix tool_choice.name if type is "tool"
	if llmReq.ToolChoice != nil && llmReq.ToolChoice.NamedToolChoice != nil {
		if llmReq.ToolChoice.NamedToolChoice.Type == "tool" {
			name := llmReq.ToolChoice.NamedToolChoice.Function.Name
			if name != "" && !strings.HasPrefix(name, prefix) {
				llmReq.ToolChoice.NamedToolChoice.Function.Name = prefix + name
			}
		}
	}

	return llmReq
}

// stripClaudeToolPrefixFromResponse removes the prefix from tool names in the response.
func stripClaudeToolPrefixFromResponse(body []byte, prefix string) []byte {
	if prefix == "" {
		return body
	}

	content := gjson.GetBytes(body, "content")
	if !content.Exists() || !content.IsArray() {
		return body
	}

	content.ForEach(func(index, part gjson.Result) bool {
		if part.Get("type").String() != "tool_use" {
			return true
		}

		name := part.Get("name").String()
		if !strings.HasPrefix(name, prefix) {
			return true
		}

		path := fmt.Sprintf("content.%d.name", index.Int())
		body, _ = sjson.SetBytes(body, path, strings.TrimPrefix(name, prefix))

		return true
	})

	return body
}

// mergeBetasIntoHeader merges beta features into the Anthropic-Beta header.
func mergeBetasIntoHeader(baseBetas string, extraBetas []string) string {
	var parts []string
	existingSet := make(map[string]bool)

	// Add existing betas if present
	baseBetas = strings.TrimSpace(baseBetas)
	if baseBetas != "" {
		for _, b := range strings.Split(baseBetas, ",") {
			b = strings.TrimSpace(b)
			if b != "" {
				parts = append(parts, b)
				existingSet[b] = true
			}
		}
	}

	// Add extra betas if not already present
	for _, beta := range extraBetas {
		beta = strings.TrimSpace(beta)
		if beta != "" && !existingSet[beta] {
			parts = append(parts, beta)
			existingSet[beta] = true
		}
	}

	return strings.Join(parts, ",")
}

// billingHeaderPrefix is the prefix used to identify billing header system messages.
const billingHeaderPrefix = "x-anthropic-billing-header:"

// removeBillingSystemMessages removes system messages that contain the
// x-anthropic-billing-header pattern. These messages are injected by the
// Claude Code CLI to report billing metadata. For non-official (non-OAuth)
// channels, these messages should be stripped to avoid leaking client info.
func removeBillingSystemMessages(llmReq *llm.Request) *llm.Request {
	if len(llmReq.Messages) == 0 {
		return llmReq
	}

	filtered := make([]llm.Message, 0, len(llmReq.Messages))

	for _, msg := range llmReq.Messages {
		if msg.Role == "system" && msg.Content.Content != nil &&
			strings.HasPrefix(strings.TrimSpace(*msg.Content.Content), billingHeaderPrefix) {
			continue
		}

		filtered = append(filtered, msg)
	}

	llmReq.Messages = filtered

	return llmReq
}

func ensureBillingSystemMessageCCH(llmReq *llm.Request) *llm.Request {
	if llmReq == nil || len(llmReq.Messages) == 0 {
		return llmReq
	}

	cch := ""
	if llmReq.TransformerMetadata != nil {
		if v, ok := llmReq.TransformerMetadata[claudeCodeBillingCCHMetadataKey]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				cch = strings.TrimSpace(s)
			}
		}
	}
	if cch == "" {
		return llmReq
	}

	for i := range llmReq.Messages {
		msg := &llmReq.Messages[i]
		if msg.Role != "system" {
			continue
		}

		if msg.Content.Content != nil {
			updated, changed := ensureBillingHeaderCCHInText(*msg.Content.Content, cch)
			if changed {
				*msg.Content.Content = updated
			}
		}

		if len(msg.Content.MultipleContent) > 0 {
			for j := range msg.Content.MultipleContent {
				part := &msg.Content.MultipleContent[j]
				if part.Type != "text" || part.Text == nil {
					continue
				}

				updated, changed := ensureBillingHeaderCCHInText(*part.Text, cch)
				if changed {
					*part.Text = updated
				}
			}
		}
	}

	return llmReq
}

func ensureBillingHeaderCCHInText(text string, cch string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return text, false
	}

	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, billingHeaderPrefix) {
		return text, false
	}

	rest := strings.TrimSpace(trimmed[len(billingHeaderPrefix):])
	if rest == "" {
		return text, false
	}

	parts := strings.Split(rest, ";")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		if strings.HasPrefix(strings.ToLower(p), "cch=") {
			return text, false
		}
	}

	out := strings.TrimSpace(trimmed)
	if !strings.HasSuffix(out, ";") {
		out += ";"
	}
	out += " cch=" + strings.TrimSpace(cch) + ";"

	return out, true
}

// injectClaudeCodeSystemMessageStructured prepends the Claude Code system message.
// 注意：不在此处设置 cache_control，缓存策略由 ensureCacheControl 统一管理。
func injectClaudeCodeSystemMessageStructured(llmReq *llm.Request) *llm.Request {
	claudeCodeMsg := llm.Message{
		Role: "system",
		Content: llm.MessageContent{
			Content: func() *string { s := claudeCodeSystemMessage; return &s }(),
		},
	}

	if len(llmReq.Messages) > 0 && llmReq.Messages[0].Role == "system" {
		if llmReq.Messages[0].Content.Content != nil &&
			*llmReq.Messages[0].Content.Content == claudeCodeSystemMessage {
			return llmReq
		}
	}

	llmReq.Messages = append([]llm.Message{claudeCodeMsg}, llmReq.Messages...)

	// Ensure array format for system prompts (required for cache_control)
	if llmReq.TransformOptions.ArrayInstructions == nil {
		arrayInstructions := true
		llmReq.TransformOptions.ArrayInstructions = &arrayInstructions
	}

	return llmReq
}

// injectOrReplaceBillingHeader handles the x-anthropic-billing-header in system messages.
// If a billing header already exists in system[0], replace it with the configured value.
// If no billing header exists, inject it as a new system[0].
// If billingHeaderValue is empty, this is a no-op.
func injectOrReplaceBillingHeader(llmReq *llm.Request, billingHeaderValue string) *llm.Request {
	if billingHeaderValue == "" {
		return llmReq
	}

	// Check if system[0] already has a billing header
	if len(llmReq.Messages) > 0 && llmReq.Messages[0].Role == "system" {
		msg := &llmReq.Messages[0]
		if msg.Content.Content != nil &&
			strings.HasPrefix(strings.TrimSpace(strings.ToLower(*msg.Content.Content)), billingHeaderPrefix) {
			// Replace existing billing header
			*msg.Content.Content = billingHeaderValue
			return llmReq
		}
		// Also check MultipleContent[0]
		if len(msg.Content.MultipleContent) > 0 &&
			msg.Content.MultipleContent[0].Type == "text" &&
			msg.Content.MultipleContent[0].Text != nil &&
			strings.HasPrefix(strings.TrimSpace(strings.ToLower(*msg.Content.MultipleContent[0].Text)), billingHeaderPrefix) {
			*msg.Content.MultipleContent[0].Text = billingHeaderValue
			return llmReq
		}
	}

	// No existing billing header found — inject as new system[0]
	billingMsg := llm.Message{
		Role: "system",
		Content: llm.MessageContent{
			Content: func() *string { s := billingHeaderValue; return &s }(),
		},
	}
	llmReq.Messages = append([]llm.Message{billingMsg}, llmReq.Messages...)

	return llmReq
}

// isRealCliRequest performs 3-step detection to determine if the request
// originates from a real Claude CLI client.
func isRealCliRequest(rawUA string, req *llm.Request) bool {
	if !isClaudeCLIUserAgent(rawUA) {
		return false
	}
	if !hasClaudeCodeIdentity(req) {
		return false
	}
	if req.Metadata == nil {
		return false
	}
	userID := req.Metadata["user_id"]
	return userID != "" && isValidUserID(userID)
}

// hasClaudeCodeIdentity checks if the request contains Claude Code identity in the system messages.
func hasClaudeCodeIdentity(req *llm.Request) bool {
	for _, msg := range req.Messages {
		if msg.Role != "system" {
			continue
		}
		if msg.Content.Content != nil && checkClaudeIdentity(*msg.Content.Content) {
			return true
		}
		for _, part := range msg.Content.MultipleContent {
			if part.Type == "text" && part.Text != nil && checkClaudeIdentity(*part.Text) {
				return true
			}
		}
	}
	return false
}

func checkClaudeIdentity(text string) bool {
	return strings.Contains(text, "You are Claude Code, Anthropic's official CLI for Claude") ||
		strings.Contains(text, "You are a Claude agent, built on Anthropic's Claude Agent SDK")
}

// injectOrReplaceUserID replaces user_id with unified client ID or generates a new one.
func injectOrReplaceUserID(llmReq *llm.Request, unifiedClientId string) *llm.Request {
	if llmReq.Metadata == nil {
		llmReq.Metadata = make(map[string]string)
	}

	existingUserID := llmReq.Metadata["user_id"]
	if existingUserID != "" && isValidUserID(existingUserID) {
		idx := strings.Index(existingUserID, "_account__session_")
		if idx != -1 {
			llmReq.Metadata["user_id"] = "user_" + unifiedClientId + existingUserID[idx:]
			return llmReq
		}
	}

	llmReq.Metadata["user_id"] = "user_" + unifiedClientId + "_account__session_" + uuid.New().String()
	return llmReq
}
