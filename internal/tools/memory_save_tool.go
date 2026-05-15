package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"happyagent/internal/memory"
)

const MemorySaveToolName = "memory_save"

type MemorySaveTool struct {
	store *memory.LongTermStore
}

func NewMemorySaveTool(store *memory.LongTermStore) *MemorySaveTool {
	return &MemorySaveTool{store: store}
}

func (t *MemorySaveTool) Definition() Definition {
	return Definition{
		Name:        MemorySaveToolName,
		Description: "Save durable information to persistent memory that survives across sessions. Memory is injected into future sessions as a frozen snapshot at session start.\n\nWHEN TO SAVE (do this proactively, don't wait to be asked):\n- User corrects you or says 'remember this' / 'don't do that again'\n- User shares a preference, habit, or personal detail (name, role, coding style)\n- You discover something about the environment (OS, installed tools, project structure)\n- You learn a convention, API quirk, or workflow specific to this user's setup\n- You identify a stable fact that will be useful again in future sessions\n\nPRIORITY: User preferences and corrections > environment facts > procedural knowledge.\n\nDo NOT save task progress, session outcomes, completed-work logs, or temporary TODO state.\n\nTWO TARGETS:\n- 'user': who the user is -- name, role, preferences, communication style, pet peeves\n- 'memory': your notes -- environment facts, project conventions, tool quirks, lessons learned\n\nWhen memory is full, use memory_delete to remove outdated entries first.",
		InputSchema: `{"type":"object","required":["target","content"],"properties":{"target":{"type":"string","enum":["memory","user"],"description":"Which store: 'memory' for agent notes, 'user' for user profile."},"content":{"type":"string","minLength":1,"description":"The entry content. Keep it compact and information-dense."}},"additionalProperties":false}`,
	}
}

func (t *MemorySaveTool) Execute(ctx context.Context, call Call) (Result, error) {
	_ = ctx
	var input struct {
		Target  string `json:"target"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode memory_save arguments: %w", err)
	}

	err := t.store.Save(input.Target, input.Content, "")
	if err != nil {
		if ce, ok := err.(*memory.CapacityError); ok {
			entries := ce.Entries
			entryList := make([]string, len(entries))
			for i, e := range entries {
				c := e.Content
				if len(c) > 100 {
					c = c[:100] + "..."
				}
				entryList[i] = c
			}
			out, _ := json.Marshal(map[string]interface{}{
				"success":    false,
				"error":      ce.Error(),
				"usage":      fmt.Sprintf("%d/%d chars", ce.Current, ce.Limit),
				"entries":    entryList,
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
