package flow

import (
	"context"

	"github.com/package-register/trpc-agent-go-extensions/pipeline"

	"trpc.group/trpc-go/trpc-agent-go/graph"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// MiddlewareChain combines multiple Middleware into one.
// Pre callbacks execute in order; Post callbacks execute in order.
type MiddlewareChain struct {
	items []pipeline.Middleware
}

// NewMiddlewareChain creates a chain from the given middlewares.
func NewMiddlewareChain(items ...pipeline.Middleware) *MiddlewareChain {
	return &MiddlewareChain{items: items}
}

// WrapPreNode returns a single BeforeNodeCallback that runs all middleware pre-callbacks in order.
func (c *MiddlewareChain) WrapPreNode(stepID string, step *pipeline.StepDefinition) graph.BeforeNodeCallback {
	var callbacks []graph.BeforeNodeCallback
	for _, mw := range c.items {
		cb := mw.WrapPreNode(stepID, step)
		if cb != nil {
			callbacks = append(callbacks, cb)
		}
	}
	if len(callbacks) == 0 {
		return nil
	}
	return func(ctx context.Context, cbCtx *graph.NodeCallbackContext, state graph.State) (any, error) {
		var merged graph.State
		for _, cb := range callbacks {
			result, err := cb(ctx, cbCtx, state)
			if err != nil {
				return nil, err
			}
			if st, ok := result.(graph.State); ok {
				if merged == nil {
					merged = st
				} else {
					for k, v := range st {
						merged[k] = v
					}
				}
				// Propagate message changes to state for subsequent callbacks
				if msgs, ok := st[graph.StateKeyMessages]; ok {
					state[graph.StateKeyMessages] = msgs
				}
			}
		}
		return merged, nil
	}
}

// WrapPostNode returns a single AfterNodeCallback that runs all middleware post-callbacks in order.
func (c *MiddlewareChain) WrapPostNode(stepID string, step *pipeline.StepDefinition) graph.AfterNodeCallback {
	var callbacks []graph.AfterNodeCallback
	for _, mw := range c.items {
		cb := mw.WrapPostNode(stepID, step)
		if cb != nil {
			callbacks = append(callbacks, cb)
		}
	}
	if len(callbacks) == 0 {
		return nil
	}
	return func(ctx context.Context, cbCtx *graph.NodeCallbackContext, state graph.State, result any, nodeErr error) (any, error) {
		for _, cb := range callbacks {
			r, err := cb(ctx, cbCtx, state, result, nodeErr)
			if err != nil {
				return nil, err
			}
			if r != nil {
				result = r
			}
		}
		return result, nil
	}
}

// CompressionMiddleware implements pipeline.Middleware.
// It checks token usage before each LLM node and triggers compression when needed.
type CompressionMiddleware struct {
	compressor pipeline.Compressor
	counter    pipeline.TokenCounter
	observer   pipeline.TokenObserver // optional, may be nil
}

// NewCompressionMiddleware creates a middleware that compresses context when needed.
func NewCompressionMiddleware(
	compressor pipeline.Compressor,
	counter pipeline.TokenCounter,
	observer pipeline.TokenObserver,
) *CompressionMiddleware {
	return &CompressionMiddleware{
		compressor: compressor,
		counter:    counter,
		observer:   observer,
	}
}

// WrapPreNode returns a callback that checks and compresses messages.
func (m *CompressionMiddleware) WrapPreNode(_ string, _ *pipeline.StepDefinition) graph.BeforeNodeCallback {
	return func(ctx context.Context, _ *graph.NodeCallbackContext, state graph.State) (any, error) {
		msgs, ok := state[graph.StateKeyMessages].([]model.Message)
		if !ok || len(msgs) <= 1 {
			return nil, nil
		}

		estimatedTokens := m.counter.Count(ctx, msgs)

		compressed, didCompress, err := m.compressor.CompressIfNeeded(ctx, msgs, estimatedTokens)
		if err != nil || !didCompress {
			return nil, nil
		}

		if m.observer != nil {
			afterTokens := m.counter.Count(ctx, compressed)
			m.observer.OnCompression(estimatedTokens, afterTokens)
		}

		return graph.State{
			graph.StateKeyMessages: []graph.MessageOp{
				graph.RemoveAllMessages{},
				graph.AppendMessages{Items: compressed},
			},
		}, nil
	}
}

// WrapPostNode returns nil (compression has no post-node behavior).
func (m *CompressionMiddleware) WrapPostNode(_ string, _ *pipeline.StepDefinition) graph.AfterNodeCallback {
	return nil
}

// PromptInjectionMiddleware implements pipeline.Middleware.
// It rebuilds the Layer 1+2 system message at runtime with dynamic context.
type PromptInjectionMiddleware struct {
	assembler pipeline.PromptAssembler
	baseVars  map[string]string
}

// NewPromptInjectionMiddleware creates a middleware that injects dynamic prompts.
func NewPromptInjectionMiddleware(assembler pipeline.PromptAssembler, baseVars map[string]string) *PromptInjectionMiddleware {
	return &PromptInjectionMiddleware{
		assembler: assembler,
		baseVars:  baseVars,
	}
}

// WrapPreNode returns a callback that rebuilds the system message with dynamic content.
func (m *PromptInjectionMiddleware) WrapPreNode(_ string, step *pipeline.StepDefinition) graph.BeforeNodeCallback {
	if !m.assembler.HasDynamicContent() {
		return nil
	}
	return func(ctx context.Context, _ *graph.NodeCallbackContext, state graph.State) (any, error) {
		msgs, ok := state[graph.StateKeyMessages].([]model.Message)
		if !ok || len(msgs) == 0 {
			return nil, nil
		}
		if msgs[0].Role != model.RoleSystem {
			return nil, nil
		}

		fullInstruction, err := m.assembler.BuildDynamic(ctx, step, m.baseVars)
		if err != nil {
			return nil, nil
		}

		newMsgs := make([]model.Message, len(msgs))
		copy(newMsgs, msgs)
		newMsgs[0] = model.NewSystemMessage(fullInstruction)

		return graph.State{
			graph.StateKeyMessages: newMsgs,
		}, nil
	}
}

// WrapPostNode returns nil (prompt injection has no post-node behavior).
func (m *PromptInjectionMiddleware) WrapPostNode(_ string, _ *pipeline.StepDefinition) graph.AfterNodeCallback {
	return nil
}

// ArtifactRecordMiddleware implements pipeline.Middleware.
// It records step output artifacts after the confirm node completes.
type ArtifactRecordMiddleware struct {
	tracker pipeline.ArtifactTracker
}

// NewArtifactRecordMiddleware creates a middleware that records artifacts.
func NewArtifactRecordMiddleware(tracker pipeline.ArtifactTracker) *ArtifactRecordMiddleware {
	return &ArtifactRecordMiddleware{tracker: tracker}
}

// WrapPreNode returns nil (artifact recording has no pre-node behavior).
func (m *ArtifactRecordMiddleware) WrapPreNode(_ string, _ *pipeline.StepDefinition) graph.BeforeNodeCallback {
	return nil
}

// WrapPostNode returns a callback that records the artifact when the node completes.
func (m *ArtifactRecordMiddleware) WrapPostNode(stepID string, step *pipeline.StepDefinition) graph.AfterNodeCallback {
	if len(step.Frontmatter.Output) == 0 {
		return nil
	}
	return func(_ context.Context, _ *graph.NodeCallbackContext, _ graph.State, _ any, nodeErr error) (any, error) {
		if nodeErr != nil {
			return nil, nodeErr
		}
		for _, output := range step.Frontmatter.Output {
			m.tracker.RecordCompleted(stepID, step.Frontmatter.Title, output)
		}
		return nil, nil
	}
}

// Verify interface compliance at compile time.
var (
	_ pipeline.Middleware = (*MiddlewareChain)(nil)
	_ pipeline.Middleware = (*CompressionMiddleware)(nil)
	_ pipeline.Middleware = (*PromptInjectionMiddleware)(nil)
	_ pipeline.Middleware = (*ArtifactRecordMiddleware)(nil)
)
