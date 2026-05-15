package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"happyagent/internal/memory"
)

const MemoryDeleteToolName = "memory_delete"

type MemoryDeleteTool struct {
	store *memory.LongTermStore
}

func NewMemoryDeleteTool(store *memory.LongTermStore) *MemoryDeleteTool {
	return &MemoryDeleteTool{store: store}
}

func (t *MemoryDeleteTool) Definition() Definition {
	return Definition{
		Name:        MemoryDeleteToolName,
		Description: "Remove a persistent memory entry by matching a short unique substring. If the substring matches multiple entries, an error is returned asking for a more specific match.",
		InputSchema: `{"type":"object","required":["target","old_text"],"properties":{"target":{"type":"string","enum":["memory","user"],"description":"Which store to delete from."},"old_text":{"type":"string","minLength":1,"description":"Short unique substring identifying the entry to remove."}},"additionalProperties":false}`,
	}
}

func (t *MemoryDeleteTool) Execute(ctx context.Context, call Call) (Result, error) {
	_ = ctx
	var input struct {
		Target  string `json:"target"`
		OldText string `json:"old_text"`
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode memory_delete arguments: %w", err)
	}

	err := t.store.Delete(input.Target, input.OldText)
	if err != nil {
		if mme, ok := err.(*memory.MultipleMatchError); ok {
			out, _ := json.Marshal(map[string]interface{}{
				"success": false,
				"error":   mme.Error(),
				"matches": mme.Matches,
			})
			return Result{Output: string(out)}, nil
		}
		return Result{}, err
	}

	count := t.store.CharCount(input.Target)
	limit := t.store.CharLimit(input.Target)
	pct := 0
	if limit > 0 {
		pct = count * 100 / limit
	}
	out, _ := json.Marshal(map[string]interface{}{
		"success": true,
		"target":  input.Target,
		"usage":   fmt.Sprintf("%d%% — %d/%d chars", pct, count, limit),
	})
	return Result{Output: string(out)}, nil
}
