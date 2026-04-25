package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

const FinalAnswerToolName = "final_answer"

type FinalAnswerTool struct{}

func NewFinalAnswerTool() *FinalAnswerTool {
	return &FinalAnswerTool{}
}

func (t *FinalAnswerTool) Definition() Definition {
	return Definition{
		Name:        FinalAnswerToolName,
		Description: "Finish the run and return the final answer to the user.",
		InputSchema: `{"type":"object","properties":{"content":{"type":"string","description":"Final answer to return to the user."}},"required":["content"]}`,
	}
}

func (t *FinalAnswerTool) Execute(ctx context.Context, call Call) (Result, error) {
	_ = ctx

	var input struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode final_answer arguments: %w", err)
	}
	if input.Content == "" {
		return Result{}, fmt.Errorf("final_answer content must not be empty")
	}
	return Result{Output: input.Content}, nil
}
