package tools

import (
	"context"
	"encoding/json"
)

type Definition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema string `json:"input_schema"`
	Dangerous   bool   `json:"dangerous"`
}

type Call struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type Result struct {
	Output string `json:"output"`
}

type Tool interface {
	Definition() Definition
	Execute(ctx context.Context, call Call) (Result, error)
}
