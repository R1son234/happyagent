package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

const (
	AgentTaskToolName        = "agent_task"
	AgentSpawnToolName       = "agent_spawn"
	AgentSendMessageToolName = "agent_send_message"
	AgentCheckInboxToolName  = "agent_check_inbox"
	AgentShutdownToolName    = "agent_shutdown"
)

type AgentProvider interface {
	AgentTask(ctx context.Context, args json.RawMessage) (string, error)
	AgentSpawn(ctx context.Context, args json.RawMessage) (string, error)
	AgentSendMessage(ctx context.Context, args json.RawMessage) (string, error)
	AgentCheckInbox(ctx context.Context, args json.RawMessage) (string, error)
	AgentShutdown(ctx context.Context, args json.RawMessage) (string, error)
}

type AgentTool struct {
	name        string
	description string
	schema      string
	resolver    func(context.Context) AgentProvider
}

func NewAgentTools(resolver func(context.Context) AgentProvider) []*AgentTool {
	return []*AgentTool{
		{name: AgentTaskToolName, description: "Run a synchronous one-shot child agent with fresh context. Returns only the child summary and stores the child trace.", schema: `{"type":"object","required":["prompt"],"properties":{"name":{"type":"string"},"role":{"type":"string"},"prompt":{"type":"string"},"profile_name":{"type":"string"},"worktree_path":{"type":"string","description":"Optional .happyagent/worktrees path used as the child shell cwd."},"task_id":{"type":"string","description":"Optional durable task id to claim before running and complete on success."}},"additionalProperties":false}`, resolver: resolver},
		{name: AgentSpawnToolName, description: "Spawn an asynchronous teammate. The teammate runs independently and reports to the lead mailbox.", schema: `{"type":"object","required":["prompt"],"properties":{"agent_id":{"type":"string"},"name":{"type":"string"},"role":{"type":"string"},"prompt":{"type":"string"},"profile_name":{"type":"string"},"worktree_path":{"type":"string","description":"Optional .happyagent/worktrees path used as the child shell cwd."},"task_id":{"type":"string","description":"Optional durable task id to claim before running and complete on success."}},"additionalProperties":false}`, resolver: resolver},
		{name: AgentSendMessageToolName, description: "Send a mailbox message to a teammate or the lead.", schema: `{"type":"object","required":["to","content"],"properties":{"to":{"type":"string"},"kind":{"type":"string"},"content":{"type":"string"}},"additionalProperties":false}`, resolver: resolver},
		{name: AgentCheckInboxToolName, description: "List unconsumed messages for the lead inbox and mark returned messages consumed.", schema: `{"type":"object","properties":{"to":{"type":"string"}},"additionalProperties":false}`, resolver: resolver},
		{name: AgentShutdownToolName, description: "Request a teammate shutdown and mark it stopped in the team store.", schema: `{"type":"object","required":["agent_id"],"properties":{"agent_id":{"type":"string"},"reason":{"type":"string"}},"additionalProperties":false}`, resolver: resolver},
	}
}

func (t *AgentTool) Definition() Definition {
	return Definition{Name: t.name, Description: t.description, InputSchema: t.schema, Dangerous: false}
}

func (t *AgentTool) Execute(ctx context.Context, call Call) (Result, error) {
	provider := t.resolver(ctx)
	if provider == nil {
		return Result{}, fmt.Errorf("%s is unavailable outside an active runtime session", t.name)
	}
	var output string
	var err error
	switch t.name {
	case AgentTaskToolName:
		output, err = provider.AgentTask(ctx, call.Arguments)
	case AgentSpawnToolName:
		output, err = provider.AgentSpawn(ctx, call.Arguments)
	case AgentSendMessageToolName:
		output, err = provider.AgentSendMessage(ctx, call.Arguments)
	case AgentCheckInboxToolName:
		output, err = provider.AgentCheckInbox(ctx, call.Arguments)
	case AgentShutdownToolName:
		output, err = provider.AgentShutdown(ctx, call.Arguments)
	default:
		err = fmt.Errorf("unknown agent tool %q", t.name)
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Output: output}, nil
}

type agentProviderContextKey struct{}

func WithAgentProvider(ctx context.Context, provider AgentProvider) context.Context {
	return context.WithValue(ctx, agentProviderContextKey{}, provider)
}

func AgentProviderFromContext(ctx context.Context) AgentProvider {
	if ctx == nil {
		return nil
	}
	provider, _ := ctx.Value(agentProviderContextKey{}).(AgentProvider)
	return provider
}
