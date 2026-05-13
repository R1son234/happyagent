package tools

import (
	"context"
	"strings"
	"testing"
)

func TestWriteTodosToolDefaultsAndSummarizesTodos(t *testing.T) {
	tool := NewWriteTodosTool()

	result, err := tool.Execute(context.Background(), Call{
		Name:      WriteTodosToolName,
		Arguments: []byte(`{"todos":[{"content":" inspect repo ","status":"in_progress","priority":"high"},{"content":"write summary"}]}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "pending=1, in_progress=1, completed=0") {
		t.Fatalf("unexpected output: %q", result.Output)
	}

	todos, err := DecodeWriteTodosArguments([]byte(`{"todos":[{"content":" inspect repo ","status":"in_progress","priority":"high"},{"content":"write summary"}]}`))
	if err != nil {
		t.Fatalf("DecodeWriteTodosArguments() error = %v", err)
	}
	if todos[0].Content != "inspect repo" || todos[1].Status != "pending" || todos[1].Priority != "medium" {
		t.Fatalf("unexpected todos: %+v", todos)
	}
}

func TestWriteTodosToolRejectsInvalidTodos(t *testing.T) {
	tool := NewWriteTodosTool()

	_, err := tool.Execute(context.Background(), Call{
		Name:      WriteTodosToolName,
		Arguments: []byte(`{"todos":[{"content":"done","status":"stuck"}]}`),
	})
	if err == nil || !strings.Contains(err.Error(), "status must be pending") {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = tool.Execute(context.Background(), Call{
		Name:      WriteTodosToolName,
		Arguments: []byte(`{"todos":[{"content":"   "}]}`),
	})
	if err == nil || !strings.Contains(err.Error(), "content must not be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}
