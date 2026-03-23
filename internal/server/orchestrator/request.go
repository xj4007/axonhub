package orchestrator

import (
	"context"
	"time"

	"github.com/looplj/axonhub/internal/ent/request"
	"github.com/looplj/axonhub/internal/log"
	"github.com/looplj/axonhub/internal/pkg/xcontext"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/pipeline"
	"github.com/looplj/axonhub/llm/streams"
)

type persistRequestMiddleware struct {
	pipeline.DummyMiddleware

	inbound     *PersistentInboundTransformer
	llmResponse *llm.Response
}

func persistRequest(inbound *PersistentInboundTransformer) pipeline.Middleware {
	return &persistRequestMiddleware{
		inbound: inbound,
	}
}

type finalStreamUsageCaptureMiddleware struct {
	pipeline.DummyMiddleware

	state *PersistenceState
}

func captureFinalStreamUsage(state *PersistenceState) pipeline.Middleware {
	return &finalStreamUsageCaptureMiddleware{state: state}
}

func (m *persistRequestMiddleware) Name() string {
	return "persist-request"
}

func (m *finalStreamUsageCaptureMiddleware) Name() string {
	return "capture-final-stream-usage"
}

func (m *persistRequestMiddleware) OnInboundLlmRequest(ctx context.Context, llmRequest *llm.Request) (*llm.Request, error) {
	if m.inbound.state.Request != nil {
		return llmRequest, nil
	}

	request, err := m.inbound.state.RequestService.CreateRequest(
		ctx,
		llmRequest,
		m.inbound.state.RawRequest,
		m.inbound.APIFormat(),
	)
	if err != nil {
		return nil, err
	}

	m.inbound.state.Request = request

	return llmRequest, nil
}

func (m *persistRequestMiddleware) OnOutboundLlmResponse(ctx context.Context, llmResp *llm.Response) (*llm.Response, error) {
	state := m.inbound.state
	if state.Request == nil || llmResp == nil {
		return llmResp, nil
	}

	// Store LLM response locally for use in OnInboundRawResponse
	m.llmResponse = llmResp

	return llmResp, nil
}

func (m *persistRequestMiddleware) OnOutboundLlmStream(ctx context.Context, stream streams.Stream[*llm.Response]) (streams.Stream[*llm.Response], error) {
	return &persistRequestUsageStream{
		stream: stream,
		state:  m.inbound.state,
	}, nil
}

func (m *finalStreamUsageCaptureMiddleware) OnOutboundLlmStream(ctx context.Context, stream streams.Stream[*llm.Response]) (streams.Stream[*llm.Response], error) {
	return &persistRequestUsageStream{
		stream: stream,
		state:  m.state,
	}, nil
}

func (m *persistRequestMiddleware) OnInboundRawResponse(ctx context.Context, httpResp *httpclient.Response) (*httpclient.Response, error) {
	state := m.inbound.state
	if state.Request == nil || httpResp == nil {
		return httpResp, nil
	}

	llmResp := m.llmResponse
	if llmResp == nil {
		log.Warn(ctx, "LLM response not found in middleware, cannot update request completed status")
		return httpResp, nil
	}

	// Use context without cancellation to ensure persistence even if client canceled
	persistCtx, cancel := xcontext.DetachWithTimeout(ctx, time.Second*10)
	defer cancel()

	if _, err := state.UsageLogService.CreateUsageLogFromRequest(persistCtx, state.Request, state.RequestExec, llmResp.Usage); err != nil {
		log.Warn(persistCtx, "Failed to create usage log from request", log.Cause(err))
	}

	// Build latency metrics from performance record
	var metrics *biz.LatencyMetrics

	if state.Perf != nil {
		firstTokenLatencyMs, requestLatencyMs, _ := state.Perf.Calculate()

		metrics = &biz.LatencyMetrics{
			LatencyMs: &requestLatencyMs,
		}
		if state.Perf.Stream && state.Perf.FirstTokenTime != nil {
			metrics.FirstTokenLatencyMs = &firstTokenLatencyMs
		}
	}

	// Video generation is async: initial response contains provider task id, but task may not be completed.
	// Keep request in processing status and store provider task id in external_id.
	if llmResp.RequestType == llm.RequestTypeVideo {
		err := state.RequestService.UpdateRequestStatusExternalIDAndResponseBody(
			persistCtx,
			state.Request.ID,
			request.StatusProcessing,
			llmResp.ID,
			httpResp.Body,
			metrics,
		)
		if err != nil {
			log.Warn(persistCtx, "Failed to update video request status to processing", log.Cause(err))
		}

		return httpResp, nil
	}

	err := state.RequestService.UpdateRequestCompleted(persistCtx, state.Request.ID, llmResp.ID, httpResp.Body, metrics)
	if err != nil {
		log.Warn(persistCtx, "Failed to update request status to completed", log.Cause(err))
	}

	return httpResp, nil
}

type persistRequestUsageStream struct {
	stream streams.Stream[*llm.Response]
	state  *PersistenceState
}

func (s *persistRequestUsageStream) Next() bool {
	return s.stream.Next()
}

func (s *persistRequestUsageStream) Current() *llm.Response {
	response := s.stream.Current()
	if response != nil && response.Usage != nil && s.state != nil {
		s.state.FinalStreamUsage = cloneUsage(response.Usage)
	}
	return response
}

func (s *persistRequestUsageStream) Err() error {
	return s.stream.Err()
}

func (s *persistRequestUsageStream) Close() error {
	return s.stream.Close()
}

func cloneUsage(usage *llm.Usage) *llm.Usage {
	if usage == nil {
		return nil
	}

	cloned := *usage
	if usage.PromptTokensDetails != nil {
		details := *usage.PromptTokensDetails
		cloned.PromptTokensDetails = &details
	}
	if usage.CompletionTokensDetails != nil {
		details := *usage.CompletionTokensDetails
		cloned.CompletionTokensDetails = &details
	}
	return &cloned
}
