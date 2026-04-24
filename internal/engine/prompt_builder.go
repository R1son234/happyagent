package engine

import (
	"fmt"
	"strings"

	"happyagent/internal/llm"
	"happyagent/internal/tools"
)

func BuildMessages(input RunInput, state LoopState) []llm.Message {
	messages := make([]llm.Message, 0, len(state.Messages)+2)
	messages = append(messages, llm.Message{
		Role:    "system",
		Content: buildSystemPrompt(input.SystemPrompt, input.ToolDefs),
	})
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: input.Input,
	})

	for _, message := range state.Messages {
		messages = append(messages, llm.Message{
			Role:    message.Role,
			Content: message.Content,
		})
	}

	return messages
}

func BuildToolSpecs(defs []tools.Definition) []llm.ToolSpec {
	specs := make([]llm.ToolSpec, 0, len(defs))
	for _, def := range defs {
		specs = append(specs, llm.ToolSpec{
			Name:        def.Name,
			Description: def.Description,
			InputSchema: def.InputSchema,
		})
	}
	return specs
}

func buildSystemPrompt(base string, defs []tools.Definition) string {
	if len(defs) == 0 {
		return base
	}

	var builder strings.Builder
	builder.WriteString(base)
	builder.WriteString("\n\nAvailable tools:\n")
	for _, def := range defs {
		builder.WriteString(fmt.Sprintf("- %s: %s\n", def.Name, def.Description))
	}
	return builder.String()
}
