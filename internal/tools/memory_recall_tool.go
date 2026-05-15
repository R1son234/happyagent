package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"happyagent/internal/memory"
)

const MemoryRecallToolName = "memory_recall"

type MemoryRecallTool struct {
	store *memory.LongTermStore
}

func NewMemoryRecallTool(store *memory.LongTermStore) *MemoryRecallTool {
	return &MemoryRecallTool{store: store}
}

func (t *MemoryRecallTool) Definition() Definition {
	return Definition{
		Name:        MemoryRecallToolName,
		Description: "Search persistent long-term memory for relevant information. Use when you need to recall specific facts, preferences, or decisions from past sessions. Note: all memory is already loaded at session start as a frozen snapshot in the context; this tool is for more targeted searches.",
		InputSchema: `{"type":"object","required":["query"],"properties":{"query":{"type":"string","minLength":1,"description":"Search keywords or description of what to recall."},"limit":{"type":"integer","minimum":1,"maximum":20,"default":5,"description":"Maximum number of results to return."}},"additionalProperties":false}`,
	}
}

func (t *MemoryRecallTool) Execute(ctx context.Context, call Call) (Result, error) {
	_ = ctx
	var input struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode memory_recall arguments: %w", err)
	}
	if input.Limit <= 0 {
		input.Limit = 5
	}

	results := t.store.Recall(input.Query, input.Limit)
	items := make([]map[string]interface{}, len(results))
	for i, e := range results {
		items[i] = map[string]interface{}{
			"content": e.Content,
			"source":  e.Source,
		}
	}

	out, _ := json.Marshal(map[string]interface{}{
		"results": items,
		"count":   len(items),
	})
	return Result{Output: string(out)}, nil
}
