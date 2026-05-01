package llm

import (
	"context"

	"happyagent/internal/config"
)

type Client interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

func NewClient(cfg config.LLMConfig) (Client, error) {
	return NewEinoClient(cfg)
}
