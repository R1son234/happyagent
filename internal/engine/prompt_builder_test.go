package engine

import (
	"strings"
	"testing"

	"happyagent/internal/protocol"
)

func TestBuildMessagesPlacesRuntimeContextOutsideSystemPrompt(t *testing.T) {
	messages := BuildMessages(RunInput{
		Input:          "hello",
		SystemPrompt:   "stable prompt",
		RuntimeContext: "Recent session memory:\n- user: earlier",
	}, LoopState{})

	if len(messages) != 3 {
		t.Fatalf("unexpected message count: %d", len(messages))
	}
	if messages[0].Role != protocol.RoleSystem || messages[0].Content != "stable prompt" {
		t.Fatalf("unexpected system message: %+v", messages[0])
	}
	if messages[1].Role != protocol.RoleUser || !strings.Contains(messages[1].Content, "Recent session memory") {
		t.Fatalf("unexpected runtime context message: %+v", messages[1])
	}
	if messages[2].Role != protocol.RoleUser || messages[2].Content != "hello" {
		t.Fatalf("unexpected user message: %+v", messages[2])
	}
}
