package observe

import (
	"context"
	"errors"
	"strings"
)

const (
	ErrorCategoryModel          = "model"
	ErrorCategoryToolValidation = "tool_validation"
	ErrorCategoryToolExecution  = "tool_execution"
	ErrorCategoryTimeout        = "timeout"
	ErrorCategoryPolicyDenial   = "policy_denial"
	ErrorCategoryMCPTransport   = "mcp_transport"
	ErrorCategoryValidation     = "result_validation"
	ErrorCategoryUnknown        = "unknown"
)

func ClassifyError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrorCategoryTimeout
	}

	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "policy denial"), strings.Contains(message, "approval required"):
		return ErrorCategoryPolicyDenial
	case strings.Contains(message, "decode "), strings.Contains(message, "must not be empty"), strings.Contains(message, "required"):
		return ErrorCategoryToolValidation
	case strings.Contains(message, "tool error"), strings.Contains(message, "run command"), strings.Contains(message, "write file"), strings.Contains(message, "read file"):
		return ErrorCategoryToolExecution
	case strings.Contains(message, "chat with model"), strings.Contains(message, "model"):
		return ErrorCategoryModel
	case strings.Contains(message, "mcp"):
		return ErrorCategoryMCPTransport
	case strings.Contains(message, "output validation"):
		return ErrorCategoryValidation
	default:
		return ErrorCategoryUnknown
	}
}
