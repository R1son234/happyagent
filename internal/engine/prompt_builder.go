package engine

import (
	"happyagent/internal/llm"
	"happyagent/internal/protocol"
	"happyagent/internal/tools"
)

func BuildMessages(input RunInput, state LoopState) []llm.Message {
	messages := make([]llm.Message, 0, len(state.Messages)+2)
	messages = append(messages, llm.Message{
		Role:    protocol.RoleSystem,
		Content: input.SystemPrompt,
	})
	messages = append(messages, llm.Message{
		Role:    protocol.RoleUser,
		Content: input.Input,
	})

	for _, message := range state.Messages {
		messages = append(messages, llm.Message{
			Role:             message.Role,
			Content:          message.Content,
			ReasoningContent: message.ReasoningContent,
			ToolCallID:       message.ToolCallID,
			ToolName:         message.ToolName,
			Actions:          append([]protocol.Action(nil), message.Actions...),
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
