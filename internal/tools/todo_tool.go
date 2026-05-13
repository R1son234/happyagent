package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const WriteTodosToolName = "write_todos"

const maxTodoItems = 12

type TodoItem struct {
	Content  string `json:"content"`
	Status   string `json:"status,omitempty"`
	Priority string `json:"priority,omitempty"`
}

type WriteTodosTool struct{}

func NewWriteTodosTool() *WriteTodosTool {
	return &WriteTodosTool{}
}

func (t *WriteTodosTool) Definition() Definition {
	return Definition{
		Name:        WriteTodosToolName,
		Description: "Create or replace the current run's TODO plan for complex multi-step work. Use this to plan, update progress, or remove obsolete TODOs before finishing.",
		InputSchema: `{"type":"object","required":["todos"],"properties":{"todos":{"type":"array","minItems":1,"maxItems":12,"items":{"type":"object","required":["content"],"properties":{"content":{"type":"string","minLength":1},"status":{"type":"string","enum":["pending","in_progress","completed"]},"priority":{"type":"string","enum":["low","medium","high"]}},"additionalProperties":false}}},"additionalProperties":false}`,
	}
}

func (t *WriteTodosTool) Execute(ctx context.Context, call Call) (Result, error) {
	_ = ctx

	todos, err := DecodeWriteTodosArguments(call.Arguments)
	if err != nil {
		return Result{}, err
	}

	counts := map[string]int{
		"pending":     0,
		"in_progress": 0,
		"completed":   0,
	}
	inProgress := make([]string, 0)
	for _, todo := range todos {
		counts[todo.Status]++
		if todo.Status == "in_progress" {
			inProgress = append(inProgress, todo.Content)
		}
	}

	output := fmt.Sprintf("TODO plan updated: %d item(s); pending=%d, in_progress=%d, completed=%d.",
		len(todos), counts["pending"], counts["in_progress"], counts["completed"])
	if len(inProgress) > 0 {
		output += " Current: " + strings.Join(inProgress, "; ")
	}
	return Result{Output: output}, nil
}

func DecodeWriteTodosArguments(arguments json.RawMessage) ([]TodoItem, error) {
	var input struct {
		Todos []TodoItem `json:"todos"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return nil, fmt.Errorf("decode write_todos arguments: %w", err)
	}
	if len(input.Todos) == 0 {
		return nil, fmt.Errorf("write_todos requires at least one todo")
	}
	if len(input.Todos) > maxTodoItems {
		return nil, fmt.Errorf("write_todos accepts at most %d todos", maxTodoItems)
	}

	todos := make([]TodoItem, 0, len(input.Todos))
	for i, todo := range input.Todos {
		content := strings.TrimSpace(todo.Content)
		if content == "" {
			return nil, fmt.Errorf("write_todos todos[%d].content must not be empty", i)
		}
		status := strings.TrimSpace(todo.Status)
		if status == "" {
			status = "pending"
		}
		if status != "pending" && status != "in_progress" && status != "completed" {
			return nil, fmt.Errorf("write_todos todos[%d].status must be pending, in_progress, or completed", i)
		}
		priority := strings.TrimSpace(todo.Priority)
		if priority == "" {
			priority = "medium"
		}
		if priority != "low" && priority != "medium" && priority != "high" {
			return nil, fmt.Errorf("write_todos todos[%d].priority must be low, medium, or high", i)
		}
		todos = append(todos, TodoItem{
			Content:  content,
			Status:   status,
			Priority: priority,
		})
	}
	return todos, nil
}
