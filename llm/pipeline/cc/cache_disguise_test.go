package cc

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/streams"
)

func claudeCodeSystemMessage() llm.Message {
	billing := "x-anthropic-billing-header: cc_version=2.1.50.f15; cc_entrypoint=cli;"
	return llm.Message{Role: "system", Content: llm.MessageContent{Content: &billing}}
}

func makeEligibleRequest(conversationID string, enable bool, mode string) *llm.Request {
	return &llm.Request{
		Model: "claude-sonnet-4-5",
		Messages: []llm.Message{
			claudeCodeSystemMessage(),
			{Role: "user", Content: llm.MessageContent{Content: ptrString("hello")}},
		},
		Metadata: map[string]string{
			"thread_id":                     conversationID,
			CacheDisguiseEnabledMetadataKey: boolToString(enable),
			CacheDisguiseModeMetadataKey:    mode,
		},
	}
}

func ptrString(v string) *string { return &v }

func boolToString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func newMarkerCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return ctx
}

func TestPromptCacheDisguise_DisabledByChannelSetting(t *testing.T) {
	mw := PromptCacheDisguise()
	ctx := newMarkerCtx(t)
	request := makeEligibleRequest("conv-disabled", false, simulateCacheModeEphemeral5M)

	_, err := mw.OnInboundLlmRequest(ctx, request)
	require.NoError(t, err)

	response := &llm.Response{Usage: &llm.Usage{PromptTokens: 300, CompletionTokens: 20, TotalTokens: 320}}
	_, err = mw.OnOutboundLlmResponse(ctx, response)
	require.NoError(t, err)

	// In the new flow, eligibility is decided in OnOutboundRawRequest based on per-attempt metadata,
	// so OnOutboundLlmResponse alone should not forge usage.
	require.Nil(t, response.Usage.PromptTokensDetails)
	require.Equal(t, int64(300), response.Usage.PromptTokens)
	require.Equal(t, int64(320), response.Usage.TotalTokens)
}

func TestPromptCacheDisguise_MergeCacheTokensIntoInputWithoutSimulation(t *testing.T) {
	mw := PromptCacheDisguise().(*PromptCacheDisguiseMiddleware)
	ctx := newMarkerCtx(t)
	request := makeEligibleRequest("conv-merge-no-sim", false, simulateCacheModeEphemeral5M)

	_, err := mw.OnInboundLlmRequest(ctx, request)
	require.NoError(t, err)

	_, err = mw.OnOutboundRawRequest(ctx, &httpclient.Request{Metadata: map[string]string{
		CacheDisguiseEnabledMetadataKey:                   "false",
		CacheDisguiseModeMetadataKey:                      simulateCacheModeEphemeral5M,
		CacheDisguiseMergeCacheTokensIntoInputMetadataKey: "true",
	}})
	require.NoError(t, err)

	response := &llm.Response{Usage: &llm.Usage{
		PromptTokens:     300,
		CompletionTokens: 20,
		TotalTokens:      320,
		PromptTokensDetails: &llm.PromptTokensDetails{
			CachedTokens:           180,
			WriteCachedTokens:      90,
			WriteCached5MinTokens:  90,
			WriteCached1HourTokens: 0,
		},
	}}

	_, err = mw.OnOutboundLlmResponse(ctx, response)
	require.NoError(t, err)

	require.NotNil(t, response.Usage.PromptTokensDetails)
	require.Equal(t, int64(300), response.Usage.PromptTokens)
	require.Equal(t, int64(320), response.Usage.TotalTokens)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.WriteCachedTokens)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.WriteCached5MinTokens)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.WriteCached1HourTokens)
}

func TestPromptCacheDisguise_MergeCacheTokensIntoInputWithoutConversationID(t *testing.T) {
	mw := PromptCacheDisguise().(*PromptCacheDisguiseMiddleware)
	ctx := newMarkerCtx(t)
	request := &llm.Request{
		Model: "claude-sonnet-4-5",
		Messages: []llm.Message{
			claudeCodeSystemMessage(),
			{Role: "user", Content: llm.MessageContent{Content: ptrString("hello")}},
		},
		Metadata: map[string]string{
			CacheDisguiseEnabledMetadataKey:                   "false",
			CacheDisguiseModeMetadataKey:                      simulateCacheModeEphemeral5M,
			CacheDisguiseMergeCacheTokensIntoInputMetadataKey: "true",
		},
	}

	_, err := mw.OnInboundLlmRequest(ctx, request)
	require.NoError(t, err)

	_, err = mw.OnOutboundRawRequest(ctx, &httpclient.Request{Metadata: map[string]string{
		CacheDisguiseEnabledMetadataKey:                   "false",
		CacheDisguiseModeMetadataKey:                      simulateCacheModeEphemeral5M,
		CacheDisguiseMergeCacheTokensIntoInputMetadataKey: "true",
	}})
	require.NoError(t, err)

	response := &llm.Response{Usage: &llm.Usage{
		PromptTokens:     300,
		CompletionTokens: 20,
		TotalTokens:      320,
		PromptTokensDetails: &llm.PromptTokensDetails{
			CachedTokens:           180,
			WriteCachedTokens:      90,
			WriteCached5MinTokens:  90,
			WriteCached1HourTokens: 0,
		},
	}}

	_, err = mw.OnOutboundLlmResponse(ctx, response)
	require.NoError(t, err)

	require.NotNil(t, response.Usage.PromptTokensDetails)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.WriteCachedTokens)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.WriteCached5MinTokens)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.WriteCached1HourTokens)
}

func TestPromptCacheDisguise_UpdateRequestMarkerControlsEligibility(t *testing.T) {
	mw := PromptCacheDisguise().(*PromptCacheDisguiseMiddleware)
	ctx := newMarkerCtx(t)
	request := makeEligibleRequest("conv-update", false, simulateCacheModeEphemeral5M)

	_, err := mw.OnInboundLlmRequest(ctx, request)
	require.NoError(t, err)

	_, err = mw.OnOutboundRawRequest(ctx, &httpclient.Request{Metadata: map[string]string{
		CacheDisguiseEnabledMetadataKey: "true",
		CacheDisguiseModeMetadataKey:    simulateCacheModeEphemeral1H,
	}})
	require.NoError(t, err)

	response := &llm.Response{Usage: &llm.Usage{PromptTokens: 300, CompletionTokens: 20, TotalTokens: 320}}
	_, err = mw.OnOutboundLlmResponse(ctx, response)
	require.NoError(t, err)

	require.NotNil(t, response.Usage.PromptTokensDetails)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.WriteCached5MinTokens)
	require.Greater(t, response.Usage.PromptTokensDetails.WriteCached1HourTokens, int64(0))
}
func TestPromptCacheDisguise_ModeFieldExclusivity(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mode       string
		expect5Min bool
	}{
		{name: "5m mode", mode: simulateCacheModeEphemeral5M, expect5Min: true},
		{name: "1h mode", mode: simulateCacheModeEphemeral1H, expect5Min: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mw := PromptCacheDisguise().(*PromptCacheDisguiseMiddleware)
			ctx := newMarkerCtx(t)
			conversationID := "conv-mode"

			// first turn seeds state and eligibility marker via outbound raw request phase
			_, err := mw.OnInboundLlmRequest(ctx, makeEligibleRequest(conversationID, true, tc.mode))
			require.NoError(t, err)
			_, err = mw.OnOutboundRawRequest(ctx, &httpclient.Request{Metadata: map[string]string{
				CacheDisguiseEnabledMetadataKey: "true",
				CacheDisguiseModeMetadataKey:    tc.mode,
			}})
			_, err = mw.OnOutboundLlmResponse(ctx, &llm.Response{Usage: &llm.Usage{PromptTokens: 200, CompletionTokens: 10}})
			require.NoError(t, err)

			// second turn uses delta>=110 path
			_, err = mw.OnInboundLlmRequest(ctx, makeEligibleRequest(conversationID, true, tc.mode))
			require.NoError(t, err)
			_, err = mw.OnOutboundRawRequest(ctx, &httpclient.Request{Metadata: map[string]string{
				CacheDisguiseEnabledMetadataKey: "true",
				CacheDisguiseModeMetadataKey:    tc.mode,
			}})
			require.NoError(t, err)

			resp := &llm.Response{Usage: &llm.Usage{PromptTokens: 360, CompletionTokens: 10}}
			_, err = mw.OnOutboundLlmResponse(ctx, resp)
			require.NoError(t, err)

			require.NotNil(t, resp.Usage.PromptTokensDetails)
			if tc.expect5Min {
				require.GreaterOrEqual(t, resp.Usage.PromptTokensDetails.WriteCached5MinTokens, int64(0))
				require.Equal(t, int64(0), resp.Usage.PromptTokensDetails.WriteCached1HourTokens)
			} else {
				require.GreaterOrEqual(t, resp.Usage.PromptTokensDetails.WriteCached1HourTokens, int64(0))
				require.Equal(t, int64(0), resp.Usage.PromptTokensDetails.WriteCached5MinTokens)
			}
		})
	}
}

func TestPromptCacheDisguise_DeltaAtLeast110_InputInRange(t *testing.T) {
	mw := PromptCacheDisguise().(*PromptCacheDisguiseMiddleware)
	ctx := newMarkerCtx(t)

	_, err := mw.OnInboundLlmRequest(ctx, makeEligibleRequest("conv-delta", true, simulateCacheModeEphemeral5M))
	require.NoError(t, err)
	_, err = mw.OnOutboundRawRequest(ctx, &httpclient.Request{Metadata: map[string]string{
		CacheDisguiseEnabledMetadataKey: "true",
		CacheDisguiseModeMetadataKey:    simulateCacheModeEphemeral5M,
	}})
	require.NoError(t, err)
	_, err = mw.OnOutboundLlmResponse(ctx, &llm.Response{Usage: &llm.Usage{PromptTokens: 200, CompletionTokens: 10}})
	require.NoError(t, err)

	_, err = mw.OnInboundLlmRequest(ctx, makeEligibleRequest("conv-delta", true, simulateCacheModeEphemeral5M))
	require.NoError(t, err)
	_, err = mw.OnOutboundRawRequest(ctx, &httpclient.Request{Metadata: map[string]string{
		CacheDisguiseEnabledMetadataKey: "true",
		CacheDisguiseModeMetadataKey:    simulateCacheModeEphemeral5M,
	}})
	require.NoError(t, err)
	resp := &llm.Response{Usage: &llm.Usage{PromptTokens: 360, CompletionTokens: 10}}
	_, err = mw.OnOutboundLlmResponse(ctx, resp)
	require.NoError(t, err)

	details := resp.Usage.PromptTokensDetails
	require.NotNil(t, details)
	input := resp.Usage.PromptTokens - details.CachedTokens - details.WriteCachedTokens
	require.GreaterOrEqual(t, input, int64(0))
	require.LessOrEqual(t, input, int64(5))
}

func TestPromptCacheDisguise_DeltaBelow110KeepsSmallIncrementSemantics(t *testing.T) {
	mw := PromptCacheDisguise().(*PromptCacheDisguiseMiddleware)
	ctx := newMarkerCtx(t)

	_, err := mw.OnInboundLlmRequest(ctx, makeEligibleRequest("conv-small", true, simulateCacheModeEphemeral5M))
	require.NoError(t, err)
	_, err = mw.OnOutboundRawRequest(ctx, &httpclient.Request{Metadata: map[string]string{
		CacheDisguiseEnabledMetadataKey: "true",
		CacheDisguiseModeMetadataKey:    simulateCacheModeEphemeral5M,
	}})
	require.NoError(t, err)
	_, err = mw.OnOutboundLlmResponse(ctx, &llm.Response{Usage: &llm.Usage{PromptTokens: 300, CompletionTokens: 10}})
	require.NoError(t, err)

	_, err = mw.OnInboundLlmRequest(ctx, makeEligibleRequest("conv-small", true, simulateCacheModeEphemeral5M))
	require.NoError(t, err)
	_, err = mw.OnOutboundRawRequest(ctx, &httpclient.Request{Metadata: map[string]string{
		CacheDisguiseEnabledMetadataKey: "true",
		CacheDisguiseModeMetadataKey:    simulateCacheModeEphemeral5M,
	}})
	require.NoError(t, err)
	resp := &llm.Response{Usage: &llm.Usage{PromptTokens: 350, CompletionTokens: 10}}
	_, err = mw.OnOutboundLlmResponse(ctx, resp)
	require.NoError(t, err)

	details := resp.Usage.PromptTokensDetails
	require.NotNil(t, details)
	require.Equal(t, int64(300), details.CachedTokens)
	require.Equal(t, int64(0), details.WriteCachedTokens)
	require.Equal(t, int64(50), resp.Usage.PromptTokens-details.CachedTokens-details.WriteCachedTokens)
}

func TestPromptCacheDisguise_StreamReusedUsageChunkUsesLastForged(t *testing.T) {
	mw := PromptCacheDisguise().(*PromptCacheDisguiseMiddleware)
	ctx := newMarkerCtx(t)

	_, err := mw.OnInboundLlmRequest(ctx, makeEligibleRequest("conv-stream", true, simulateCacheModeEphemeral5M))
	require.NoError(t, err)
	_, err = mw.OnOutboundRawRequest(ctx, &httpclient.Request{Metadata: map[string]string{
		CacheDisguiseEnabledMetadataKey: "true",
		CacheDisguiseModeMetadataKey:    simulateCacheModeEphemeral5M,
	}})
	require.NoError(t, err)

	baseUsage := &llm.Usage{PromptTokens: 320, CompletionTokens: 12}
	stream := streams.SliceStream([]*llm.Response{
		{Usage: &llm.Usage{PromptTokens: baseUsage.PromptTokens, CompletionTokens: baseUsage.CompletionTokens}},
		{Usage: &llm.Usage{PromptTokens: baseUsage.PromptTokens, CompletionTokens: baseUsage.CompletionTokens}},
	})
	wrapped, err := mw.OnOutboundLlmStream(ctx, stream)
	require.NoError(t, err)

	require.True(t, wrapped.Next())
	first := wrapped.Current()
	require.NotNil(t, first.Usage)
	require.NotNil(t, first.Usage.PromptTokensDetails)

	firstPrompt := first.Usage.PromptTokens
	firstWrite := first.Usage.PromptTokensDetails.WriteCachedTokens
	firstRead := first.Usage.PromptTokensDetails.CachedTokens

	require.True(t, wrapped.Next())
	second := wrapped.Current()
	require.NotNil(t, second.Usage)
	require.NotNil(t, second.Usage.PromptTokensDetails)

	require.Equal(t, firstPrompt, second.Usage.PromptTokens)
	require.Equal(t, firstWrite, second.Usage.PromptTokensDetails.WriteCachedTokens)
	require.Equal(t, firstRead, second.Usage.PromptTokensDetails.CachedTokens)
}

func TestPromptCacheDisguise_StreamMergeCacheTokensIntoInputWithoutSimulation(t *testing.T) {
	mw := PromptCacheDisguise().(*PromptCacheDisguiseMiddleware)
	ctx := newMarkerCtx(t)
	request := makeEligibleRequest("conv-stream-merge-no-sim", false, simulateCacheModeEphemeral5M)

	_, err := mw.OnInboundLlmRequest(ctx, request)
	require.NoError(t, err)

	_, err = mw.OnOutboundRawRequest(ctx, &httpclient.Request{Metadata: map[string]string{
		CacheDisguiseEnabledMetadataKey:                   "false",
		CacheDisguiseModeMetadataKey:                      simulateCacheModeEphemeral5M,
		CacheDisguiseMergeCacheTokensIntoInputMetadataKey: "true",
	}})
	require.NoError(t, err)

	stream := streams.SliceStream([]*llm.Response{
		{Usage: &llm.Usage{
			PromptTokens:     400,
			CompletionTokens: 30,
			TotalTokens:      430,
			PromptTokensDetails: &llm.PromptTokensDetails{
				CachedTokens:           250,
				WriteCachedTokens:      120,
				WriteCached5MinTokens:  120,
				WriteCached1HourTokens: 0,
			},
		}},
	})

	wrapped, err := mw.OnOutboundLlmStream(ctx, stream)
	require.NoError(t, err)
	require.True(t, wrapped.Next())

	response := wrapped.Current()
	require.NotNil(t, response)
	require.NotNil(t, response.Usage)
	require.NotNil(t, response.Usage.PromptTokensDetails)
	require.Equal(t, int64(400), response.Usage.PromptTokens)
	require.Equal(t, int64(430), response.Usage.TotalTokens)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.WriteCachedTokens)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.WriteCached5MinTokens)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.WriteCached1HourTokens)
}

func TestPromptCacheDisguise_StreamMergeCacheTokensIntoInputWithoutConversationID(t *testing.T) {
	mw := PromptCacheDisguise().(*PromptCacheDisguiseMiddleware)
	ctx := newMarkerCtx(t)
	request := &llm.Request{
		Model: "claude-sonnet-4-5",
		Messages: []llm.Message{
			claudeCodeSystemMessage(),
			{Role: "user", Content: llm.MessageContent{Content: ptrString("hello")}},
		},
		Metadata: map[string]string{
			CacheDisguiseEnabledMetadataKey:                   "false",
			CacheDisguiseModeMetadataKey:                      simulateCacheModeEphemeral5M,
			CacheDisguiseMergeCacheTokensIntoInputMetadataKey: "true",
		},
	}

	_, err := mw.OnInboundLlmRequest(ctx, request)
	require.NoError(t, err)

	_, err = mw.OnOutboundRawRequest(ctx, &httpclient.Request{Metadata: map[string]string{
		CacheDisguiseEnabledMetadataKey:                   "false",
		CacheDisguiseModeMetadataKey:                      simulateCacheModeEphemeral5M,
		CacheDisguiseMergeCacheTokensIntoInputMetadataKey: "true",
	}})
	require.NoError(t, err)

	stream := streams.SliceStream([]*llm.Response{
		{Usage: &llm.Usage{
			PromptTokens:     400,
			CompletionTokens: 30,
			TotalTokens:      430,
			PromptTokensDetails: &llm.PromptTokensDetails{
				CachedTokens:           250,
				WriteCachedTokens:      120,
				WriteCached5MinTokens:  120,
				WriteCached1HourTokens: 0,
			},
		}},
	})

	wrapped, err := mw.OnOutboundLlmStream(ctx, stream)
	require.NoError(t, err)
	require.True(t, wrapped.Next())

	response := wrapped.Current()
	require.NotNil(t, response)
	require.NotNil(t, response.Usage)
	require.NotNil(t, response.Usage.PromptTokensDetails)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.WriteCachedTokens)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.WriteCached5MinTokens)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.WriteCached1HourTokens)
}

func TestPromptCacheDisguise_MergeRunsBeforeSimulateCache(t *testing.T) {
	mw := PromptCacheDisguise().(*PromptCacheDisguiseMiddleware)
	ctx := newMarkerCtx(t)
	request := makeEligibleRequest("conv-merge-before-sim", true, simulateCacheModeEphemeral5M)
	request.Metadata[CacheDisguiseMergeCacheTokensIntoInputMetadataKey] = "true"

	_, err := mw.OnInboundLlmRequest(ctx, request)
	require.NoError(t, err)

	_, err = mw.OnOutboundRawRequest(ctx, &httpclient.Request{Metadata: map[string]string{
		CacheDisguiseEnabledMetadataKey:                   "true",
		CacheDisguiseModeMetadataKey:                      simulateCacheModeEphemeral5M,
		CacheDisguiseMergeCacheTokensIntoInputMetadataKey: "true",
	}})
	require.NoError(t, err)

	response := &llm.Response{Usage: &llm.Usage{
		PromptTokens:     300,
		CompletionTokens: 20,
		TotalTokens:      320,
		PromptTokensDetails: &llm.PromptTokensDetails{
			CachedTokens:           180,
			WriteCachedTokens:      90,
			WriteCached5MinTokens:  90,
			WriteCached1HourTokens: 0,
		},
	}}

	_, err = mw.OnOutboundLlmResponse(ctx, response)
	require.NoError(t, err)

	require.NotNil(t, response.Usage.PromptTokensDetails)
	require.Equal(t, int64(270), response.Usage.PromptTokensDetails.WriteCachedTokens)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, int64(270), response.Usage.PromptTokensDetails.WriteCached5MinTokens)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.WriteCached1HourTokens)
}

func TestPromptCacheDisguise_StreamMergeRunsBeforeSimulateCache(t *testing.T) {
	mw := PromptCacheDisguise().(*PromptCacheDisguiseMiddleware)
	ctx := newMarkerCtx(t)
	request := makeEligibleRequest("conv-stream-merge-before-sim", true, simulateCacheModeEphemeral5M)
	request.Metadata[CacheDisguiseMergeCacheTokensIntoInputMetadataKey] = "true"

	_, err := mw.OnInboundLlmRequest(ctx, request)
	require.NoError(t, err)

	_, err = mw.OnOutboundRawRequest(ctx, &httpclient.Request{Metadata: map[string]string{
		CacheDisguiseEnabledMetadataKey:                   "true",
		CacheDisguiseModeMetadataKey:                      simulateCacheModeEphemeral5M,
		CacheDisguiseMergeCacheTokensIntoInputMetadataKey: "true",
	}})
	require.NoError(t, err)

	stream := streams.SliceStream([]*llm.Response{
		{Usage: &llm.Usage{
			PromptTokens:     300,
			CompletionTokens: 20,
			TotalTokens:      320,
			PromptTokensDetails: &llm.PromptTokensDetails{
				CachedTokens:           180,
				WriteCachedTokens:      90,
				WriteCached5MinTokens:  90,
				WriteCached1HourTokens: 0,
			},
		}},
	})

	wrapped, err := mw.OnOutboundLlmStream(ctx, stream)
	require.NoError(t, err)
	require.True(t, wrapped.Next())

	response := wrapped.Current()
	require.NotNil(t, response)
	require.NotNil(t, response.Usage)
	require.NotNil(t, response.Usage.PromptTokensDetails)
	require.Equal(t, int64(270), response.Usage.PromptTokensDetails.WriteCachedTokens)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, int64(270), response.Usage.PromptTokensDetails.WriteCached5MinTokens)
	require.Equal(t, int64(0), response.Usage.PromptTokensDetails.WriteCached1HourTokens)
}

func TestStateManager_ForgeAndAdvance_ConcurrentNoRegression(t *testing.T) {
	sm := NewStateManager(0, 0, 0)
	conversationID := "conv-concurrent"

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(offset int64) {
			defer wg.Done()
			_, _ = sm.ForgeAndAdvance(conversationID, 1000+offset, nowUTC())
		}(int64(i))
	}
	wg.Wait()

	state, ok := sm.Load(conversationID, nowUTC())
	require.True(t, ok)
	require.GreaterOrEqual(t, state.LastPromptTokens, int64(1000))
	require.Equal(t, uint64(32), state.Seq)
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
