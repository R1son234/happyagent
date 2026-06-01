package engine

import (
	"context"
	"fmt"
	"time"

	"happyagent/internal/protocol"
	"happyagent/internal/tools"
)

type HookEvent string

const (
	HookRunStart          HookEvent = "RunStart"
	HookUserPromptSubmit  HookEvent = "UserPromptSubmit"
	HookBeforeModelCall   HookEvent = "BeforeModelCall"
	HookAfterModelCall    HookEvent = "AfterModelCall"
	HookPreToolUse        HookEvent = "PreToolUse"
	HookPostToolUse       HookEvent = "PostToolUse"
	HookBeforeFinalAnswer HookEvent = "BeforeFinalAnswer"
	HookStop              HookEvent = "Stop"
	HookRunError          HookEvent = "RunError"
)

type HookDecisionKind string

const (
	HookDecisionContinue             HookDecisionKind = "continue"
	HookDecisionBlockWithObservation HookDecisionKind = "block_with_observation"
	HookDecisionInjectMessage        HookDecisionKind = "inject_message"
	HookDecisionForceContinue        HookDecisionKind = "force_continue"
	HookDecisionRecordOnly           HookDecisionKind = "record_only"
)

type HookContext struct {
	Event       HookEvent
	RunInput    *RunInput
	State       *LoopState
	Action      *Action
	ToolDef     *tools.Definition
	ToolName    string
	StepIndex   int
	Content     string
	Observation string
	Err         error
	StartedAt   time.Time
}

func (h HookContext) ActionArguments() []byte {
	if h.Action == nil {
		return nil
	}
	return h.Action.Arguments
}

type HookDecision struct {
	Kind        HookDecisionKind
	Observation string
	MessageRole string
	Message     string
	Reason      string
}

type HookHandler interface {
	Name() string
	HandleHook(ctx context.Context, event HookContext) (HookDecision, error)
}

type HookHandlerFunc struct {
	HandlerName string
	Fn          func(ctx context.Context, event HookContext) (HookDecision, error)
}

func (h HookHandlerFunc) Name() string {
	return h.HandlerName
}

func (h HookHandlerFunc) HandleHook(ctx context.Context, event HookContext) (HookDecision, error) {
	return h.Fn(ctx, event)
}

type HookPipeline struct {
	handlers []HookHandler
}

func NewHookPipeline(handlers ...HookHandler) HookPipeline {
	return HookPipeline{handlers: append([]HookHandler(nil), handlers...)}
}

func (p HookPipeline) With(handler HookHandler) HookPipeline {
	p.handlers = append(append([]HookHandler(nil), p.handlers...), handler)
	return p
}

func (p HookPipeline) Emit(ctx context.Context, event HookContext) (HookDecision, error) {
	for _, handler := range p.handlers {
		if handler == nil {
			continue
		}
		decision, err := handler.HandleHook(ctx, event)
		if err != nil {
			return HookDecision{}, fmt.Errorf("hook %s for %s: %w", handler.Name(), event.Event, err)
		}
		if decision.Kind == "" {
			decision.Kind = HookDecisionContinue
		}
		recordHookDecision(event.State, event.Event, handler.Name(), decision)
		switch decision.Kind {
		case HookDecisionContinue, HookDecisionRecordOnly:
			continue
		case HookDecisionInjectMessage:
			if event.State != nil && decision.Message != "" {
				role := decision.MessageRole
				if role == "" {
					role = protocol.RoleUser
				}
				event.State.Messages = append(event.State.Messages, MessageEnvelope{
					Role:    role,
					Content: decision.Message,
				})
			}
			continue
		case HookDecisionBlockWithObservation, HookDecisionForceContinue:
			return decision, nil
		default:
			return HookDecision{}, fmt.Errorf("hook %s returned unsupported decision %q", handler.Name(), decision.Kind)
		}
	}
	return HookDecision{Kind: HookDecisionContinue}, nil
}

func recordHookDecision(state *LoopState, event HookEvent, handler string, decision HookDecision) {
	if state == nil {
		return
	}
	state.HookDecisions = append(state.HookDecisions, HookDecisionRecord{
		Event:   string(event),
		Handler: handler,
		Kind:    string(decision.Kind),
		Reason:  decision.Reason,
	})
}
