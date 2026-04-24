package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"happyagent/internal/config"
	"happyagent/internal/protocol"

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
		Timeout: 60 * time.Second,
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

	message, action, err := fromEinoMessage(resp)
	if err != nil {
		logLLMError(round, fmt.Errorf("convert model response for model %q: %w", c.model, err))
		return ChatResponse{}, fmt.Errorf("convert model response for model %q: %w", c.model, err)
	}

	response := ChatResponse{
		Message: message,
		Action:  action,
		Usage:   fromEinoUsage(resp),
	}
	logLLMJSON(round, "llm response", response)

	return response, nil
}

func toEinoMessages(messages []Message) []*schema.Message {
	out := make([]*schema.Message, 0, len(messages))
	for _, message := range messages {
		if message.Role == protocol.RoleAssistant && message.Action != nil && message.Action.Type == protocol.ActionToolCall {
			out = append(out, &schema.Message{
				Role:             schema.Assistant,
				ReasoningContent: message.ReasoningContent,
				ToolCalls: []schema.ToolCall{
					{
						ID:   message.Action.ToolCallID,
						Type: "function",
						Function: schema.FunctionCall{
							Name:      message.Action.ToolName,
							Arguments: string(message.Action.Arguments),
						},
					},
				},
			})
			continue
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

func fromEinoMessage(message *schema.Message) (Message, *protocol.Action, error) {
	if message == nil {
		return Message{}, nil, fmt.Errorf("response message is nil")
	}

	if len(message.ToolCalls) > 1 {
		return Message{}, nil, fmt.Errorf("multiple tool calls are not supported yet")
	}

	if len(message.ToolCalls) == 1 {
		call := message.ToolCalls[0]
		action := &protocol.Action{
			Type:       protocol.ActionToolCall,
			ToolCallID: call.ID,
			ToolName:   call.Function.Name,
			Arguments:  []byte(call.Function.Arguments),
		}

		return Message{
			Role:             protocol.RoleAssistant,
			ReasoningContent: message.ReasoningContent,
			Action:           action,
		}, action, nil
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
		fmt.Fprintf(os.Stderr, "===== LLM Round %d =====\n%s: marshal error: %v\n", round, label, err)
		return
	}

	fmt.Fprintf(os.Stderr, "===== LLM Round %d =====\n%s:\n%s\n", round, label, data)
}

func logLLMError(round int, err error) {
	fmt.Fprintf(os.Stderr, "===== LLM Round %d =====\nllm error: %v\n", round, err)
}
