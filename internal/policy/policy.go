package policy

import (
	"fmt"
	"sort"
	"strings"

	"happyagent/internal/tools"
)

type Decision string

const (
	DecisionAllow           Decision = "allow"
	DecisionDeny            Decision = "deny"
	DecisionRequireApproval Decision = "require_approval"
)

type Engine struct {
	approved map[string]struct{}
	denied   map[string]struct{}
}

func New(approvedTools []string, deniedTools []string) *Engine {
	return &Engine{
		approved: toSet(approvedTools),
		denied:   toSet(deniedTools),
	}
}

func (e *Engine) Decide(def tools.Definition) (Decision, string) {
	if _, ok := e.denied[def.Name]; ok {
		return DecisionDeny, fmt.Sprintf("policy denial: tool %q is explicitly denied", def.Name)
	}
	if !def.Dangerous {
		return DecisionAllow, ""
	}
	if _, ok := e.approved[def.Name]; ok {
		return DecisionAllow, ""
	}
	return DecisionRequireApproval, fmt.Sprintf("approval required for dangerous tool %q; approved tools: %s", def.Name, strings.Join(sortedKeys(e.approved), ", "))
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
