package policy

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"happyagent/internal/tools"
)

type Decision string

const (
	DecisionAllow       Decision = "allow"
	DecisionDeny        Decision = "deny"
	DecisionAsk         Decision = "ask"
	DecisionPassthrough Decision = "passthrough"
)

type Rule struct {
	Tool     string   `json:"tool"`
	Decision Decision `json:"decision"`
	Contains []string `json:"contains,omitempty"`
}

type Request struct {
	Tool     tools.Definition
	Args     json.RawMessage
	Profile  string
	ChildRun bool
}

type Engine struct {
	approved map[string]struct{}
	denied   map[string]struct{}
	rules    []Rule
}

func New(approvedTools []string, deniedTools []string, rules ...Rule) *Engine {
	return &Engine{
		approved: toSet(approvedTools),
		denied:   toSet(deniedTools),
		rules:    append([]Rule(nil), rules...),
	}
}

func (e *Engine) Decide(def tools.Definition) (Decision, string) {
	return e.DecideRequest(Request{Tool: def})
}

func (e *Engine) DecideRequest(req Request) (Decision, string) {
	def := req.Tool
	if _, ok := e.denied[def.Name]; ok {
		return DecisionDeny, fmt.Sprintf("policy denial: tool %q is explicitly denied", def.Name)
	}
	for _, rule := range e.rules {
		if rule.Tool != "" && rule.Tool != def.Name {
			continue
		}
		if len(rule.Contains) > 0 && !argumentsContain(req.Args, rule.Contains) {
			continue
		}
		switch rule.Decision {
		case DecisionDeny:
			return DecisionDeny, fmt.Sprintf("policy denial: rule denied tool %q", def.Name)
		case DecisionAsk:
			return DecisionAsk, fmt.Sprintf("approval required by policy rule for tool %q", def.Name)
		case DecisionAllow:
			return DecisionAllow, ""
		}
	}
	if deny, reason := dangerousArgumentDeny(def.Name, req.Args); deny {
		return DecisionDeny, reason
	}
	if !def.Dangerous {
		return DecisionAllow, ""
	}
	if _, ok := e.approved[def.Name]; ok {
		return DecisionAllow, ""
	}
	return DecisionAsk, fmt.Sprintf("approval required for dangerous tool %q; approved tools: %s", def.Name, strings.Join(sortedKeys(e.approved), ", "))
}

func dangerousArgumentDeny(toolName string, args json.RawMessage) (bool, string) {
	switch toolName {
	case "shell":
		var input struct {
			Command string   `json:"command"`
			Argv    []string `json:"argv"`
		}
		_ = json.Unmarshal(args, &input)
		values := append([]string{}, input.Argv...)
		if input.Command != "" {
			values = append(values, input.Command)
		}
		joined := strings.ToLower(strings.Join(values, " "))
		for _, denied := range []string{"rm -rf /", "sudo", "mkfs", "dd if=", "shutdown", "reboot"} {
			if strings.Contains(joined, denied) {
				return true, fmt.Sprintf("policy denial: shell arguments contain denied pattern %q", denied)
			}
		}
	case "file_write", "file_patch", "file_delete":
		var input struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(args, &input)
		clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(input.Path)))
		if clean == ".." || strings.HasPrefix(clean, "../") {
			return true, "policy denial: file path escapes workspace"
		}
	}
	return false, ""
}

func argumentsContain(args json.RawMessage, needles []string) bool {
	value := strings.ToLower(string(args))
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func toSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	return set
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
