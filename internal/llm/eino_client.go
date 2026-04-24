package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"happyagent/internal/config"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type EinoClient struct {
	model string
	chat  einomodel.ToolCallingChatModel
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
	logLLMJSON("llm request", req)

	chatModel := c.chat
	if len(req.Tools) > 0 {
		toolInfos, err := toEinoToolInfos(req.Tools)
		if err != nil {
			logLLMError(fmt.Errorf("convert tool specs for model %q: %w", c.model, err))
			return ChatResponse{}, fmt.Errorf("convert tool specs for model %q: %w", c.model, err)
		}

		chatModel, err = chatModel.WithTools(toolInfos)
		if err != nil {
			logLLMError(fmt.Errorf("bind tools for model %q: %w", c.model, err))
			return ChatResponse{}, fmt.Errorf("bind tools for model %q: %w", c.model, err)
		}
	}

	resp, err := chatModel.Generate(ctx, toEinoMessages(req.Messages))
	if err != nil {
		logLLMError(fmt.Errorf("generate response with model %q: %w", c.model, err))
		return ChatResponse{}, fmt.Errorf("generate response with model %q: %w", c.model, err)
	}

	message, err := fromEinoMessage(resp)
	if err != nil {
		logLLMError(fmt.Errorf("convert model response for model %q: %w", c.model, err))
		return ChatResponse{}, fmt.Errorf("convert model response for model %q: %w", c.model, err)
	}

	response := ChatResponse{
		Message: message,
		Usage:   fromEinoUsage(resp),
	}
	logLLMJSON("llm response", response)

	return response, nil
}

func toEinoMessages(messages []Message) []*schema.Message {
	out := make([]*schema.Message, 0, len(messages))
	for _, message := range messages {
		out = append(out, &schema.Message{
			Role:    toEinoRole(message.Role),
			Content: message.Content,
		})
	}
	return out
}

func toEinoRole(role string) schema.RoleType {
	switch role {
	case "system":
		return schema.System
	case "assistant":
		return schema.Assistant
	case "tool":
		return schema.Tool
	default:
		return schema.User
	}
}

func fromEinoMessage(message *schema.Message) (Message, error) {
	if message == nil {
		return Message{}, fmt.Errorf("response message is nil")
	}

	if len(message.ToolCalls) > 1 {
		return Message{}, fmt.Errorf("multiple tool calls are not supported yet")
	}

	if len(message.ToolCalls) == 1 {
		call := message.ToolCalls[0]
		action, err := json.Marshal(struct {
			Type      string          `json:"type"`
			ToolName  string          `json:"tool_name"`
			Arguments json.RawMessage `json:"arguments"`
		}{
			Type:      "tool_call",
			ToolName:  call.Function.Name,
			Arguments: json.RawMessage(call.Function.Arguments),
		})
		if err != nil {
			return Message{}, fmt.Errorf("marshal tool call action: %w", err)
		}

		return Message{
			Role:    "assistant",
			Content: string(action),
		}, nil
	}

	return Message{
		Role:    string(message.Role),
		Content: message.Content,
	}, nil
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

func logLLMJSON(label string, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: marshal error: %v\n", label, err)
		return
	}

	fmt.Fprintf(os.Stderr, "%s:\n%s\n", label, data)
}

func logLLMError(err error) {
	fmt.Fprintf(os.Stderr, "llm error: %v\n", err)
}
