package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"happyagent/internal/config"
	"happyagent/internal/protocol"
	"happyagent/internal/runlog"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type EinoClient struct {
	model string
	chat  einomodel.ToolCallingChatModel
	round uint64
}

func NewEinoClient(cfg config.LLMConfig) (Client, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("llm.model must not be empty")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("llm.api_key must not be empty")
	}

	chatModel, err := einoopenai.NewChatModel(context.Background(), &einoopenai.ChatModelConfig{
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
		BaseURL: cfg.BaseURL,
		Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("create eino openai chat model: %w", err)
	}

	return &EinoClient{
		model: cfg.Model,
		chat:  chatModel,
	}, nil
}

func (c *EinoClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	round := int(atomic.AddUint64(&c.round, 1))
	logLLMJSON(round, "llm request", req)

	chatModel := c.chat
	if len(req.Tools) > 0 {
		toolInfos, err := toEinoToolInfos(req.Tools)
		if err != nil {
			logLLMError(round, fmt.Errorf("convert tool specs for model %q: %w", c.model, err))
			return ChatResponse{}, fmt.Errorf("convert tool specs for model %q: %w", c.model, err)
		}

		chatModel, err = chatModel.WithTools(toolInfos)
		if err != nil {
			logLLMError(round, fmt.Errorf("bind tools for model %q: %w", c.model, err))
			return ChatResponse{}, fmt.Errorf("bind tools for model %q: %w", c.model, err)
		}
	}

	resp, err := chatModel.Generate(ctx, toEinoMessages(req.Messages))
	if err != nil {
		logLLMError(round, fmt.Errorf("generate response with model %q: %w", c.model, err))
		return ChatResponse{}, fmt.Errorf("generate response with model %q: %w", c.model, err)
	}

	message, actions, err := fromEinoMessage(resp)
	if err != nil {
		logLLMError(round, fmt.Errorf("convert model response for model %q: %w", c.model, err))
		return ChatResponse{}, fmt.Errorf("convert model response for model %q: %w", c.model, err)
	}

	response := ChatResponse{
		Message: message,
		Actions: actions,
		Usage:   fromEinoUsage(resp),
	}
	logLLMJSON(round, "llm response", response)

	return response, nil
}

func toEinoMessages(messages []Message) []*schema.Message {
	out := make([]*schema.Message, 0, len(messages))
	for _, message := range messages {
		if message.Role == protocol.RoleAssistant && len(message.Actions) > 0 {
			toolCalls := make([]schema.ToolCall, 0, len(message.Actions))
			for _, action := range message.Actions {
				if action.Type != protocol.ActionToolCall {
					continue
				}
				toolCalls = append(toolCalls, schema.ToolCall{
					ID:   action.ToolCallID,
					Type: "function",
					Function: schema.FunctionCall{
						Name:      action.ToolName,
						Arguments: string(action.Arguments),
					},
				})
			}
			if len(toolCalls) > 0 {
				out = append(out, &schema.Message{
					Role:             schema.Assistant,
					ReasoningContent: message.ReasoningContent,
					ToolCalls:        toolCalls,
				})
				continue
			}
		}

		out = append(out, &schema.Message{
			Role:             toEinoRole(message.Role),
			Content:          message.Content,
			ReasoningContent: message.ReasoningContent,
			ToolCallID:       message.ToolCallID,
			ToolName:         message.ToolName,
		})
	}
	return out
}

func toEinoRole(role string) schema.RoleType {
	switch role {
	case protocol.RoleSystem:
		return schema.System
	case protocol.RoleAssistant:
		return schema.Assistant
	case protocol.RoleTool:
		return schema.Tool
	default:
		return schema.User
	}
}

func fromEinoMessage(message *schema.Message) (Message, []protocol.Action, error) {
	if message == nil {
		return Message{}, nil, fmt.Errorf("response message is nil")
	}

	if len(message.ToolCalls) > 0 {
		actions := make([]protocol.Action, 0, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			actions = append(actions, protocol.Action{
				Type:       protocol.ActionToolCall,
				ToolCallID: call.ID,
				ToolName:   call.Function.Name,
				Arguments:  []byte(call.Function.Arguments),
			})
		}

		return Message{
			Role:             protocol.RoleAssistant,
			ReasoningContent: message.ReasoningContent,
			Actions:          actions,
		}, actions, nil
	}

	return Message{
		Role:             string(message.Role),
		Content:          message.Content,
		ReasoningContent: message.ReasoningContent,
	}, nil, nil
}

func fromEinoUsage(message *schema.Message) TokenUsage {
	if message == nil || message.ResponseMeta == nil || message.ResponseMeta.Usage == nil {
		return TokenUsage{}
	}

	return TokenUsage{
		PromptTokens:     message.ResponseMeta.Usage.PromptTokens,
		CompletionTokens: message.ResponseMeta.Usage.CompletionTokens,
		TotalTokens:      message.ResponseMeta.Usage.TotalTokens,
	}
}

func logLLMJSON(round int, label string, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		runlog.Section(fmt.Sprintf("LLM Round %d %s", round, label), "marshal error: "+err.Error())
		return
	}

	runlog.CodeBlock(fmt.Sprintf("LLM Round %d %s", round, label), "json", string(data))
}

func logLLMError(round int, err error) {
	runlog.Section(fmt.Sprintf("LLM Round %d error", round), err.Error())
}
