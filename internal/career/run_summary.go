package career

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type RunSummaryState struct {
	Version int                `json:"version"`
	Items   []RunSummaryRecord `json:"items"`
}

const runSummaryStateVersion = 1

func (w *Workspace) runSummaryStatePath() string {
	return filepath.Join(w.Root, WorkspaceInternalDir, "run_summaries.json")
}

func (w *Workspace) ReadRunSummaryState() (RunSummaryState, error) {
	var state RunSummaryState
	if err := w.readJSON(w.runSummaryStatePath(), &state); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RunSummaryState{Version: runSummaryStateVersion, Items: []RunSummaryRecord{}}, nil
		}
		return RunSummaryState{}, err
	}
	if state.Version == 0 {
		state.Version = runSummaryStateVersion
	}
	if state.Items == nil {
		state.Items = []RunSummaryRecord{}
	}
	return state, nil
}

func (w *Workspace) WriteRunSummaryState(state RunSummaryState) error {
	if state.Version == 0 {
		state.Version = runSummaryStateVersion
	}
	if state.Items == nil {
		state.Items = []RunSummaryRecord{}
	}
	for i, item := range state.Items {
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("run summary item[%d] id must not be empty", i)
		}
		for j, path := range append(append([]string{}, item.InputPaths...), item.Generated...) {
			path = strings.TrimSpace(path)
			if path == "" {
				continue
			}
			if err := validateWorkspaceRelPath(path); err != nil {
				return fmt.Errorf("run summary item[%d] path[%d]: %w", i, j, err)
			}
		}
		if strings.TrimSpace(item.PrimaryPath) != "" {
			if err := validateWorkspaceRelPath(item.PrimaryPath); err != nil {
				return fmt.Errorf("run summary item[%d] primary_path: %w", i, err)
			}
		}
	}
	return w.writeJSON(w.runSummaryStatePath(), state)
}

func (w *Workspace) WriteRunSummary(record RunSummaryRecord) (string, error) {
	if strings.TrimSpace(record.TaskName) == "" {
		return "", fmt.Errorf("run summary task_name must not be empty")
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}
	if strings.TrimSpace(record.ID) == "" {
		record.ID = fmt.Sprintf("%s-%s", record.CreatedAt.Format("20060102-150405"), slug(record.TaskName))
	}
	rel := filepath.ToSlash(filepath.Join(WorkspaceDirOutputs, "runs", fmt.Sprintf("%s-%s.md", record.CreatedAt.Format("20060102-150405"), slug(record.TaskName))))
	if err := w.writeWorkspaceText(rel, renderRunSummary(record)); err != nil {
		return "", err
	}
	record.Generated = uniqueStrings(append(record.Generated, rel))
	if strings.TrimSpace(record.PrimaryPath) == "" {
		record.PrimaryPath = rel
	}
	state, err := w.ReadRunSummaryState()
	if err != nil {
		return "", err
	}
	replaced := false
	for i, existing := range state.Items {
		if existing.ID == record.ID {
			state.Items[i] = record
			replaced = true
			break
		}
	}
	if !replaced {
		state.Items = append(state.Items, record)
	}
	sort.SliceStable(state.Items, func(i, j int) bool {
		return state.Items[i].CreatedAt.After(state.Items[j].CreatedAt)
	})
	if err := w.WriteRunSummaryState(state); err != nil {
		return "", err
	}
	return rel, nil
}

func renderRunSummary(record RunSummaryRecord) string {
	var b strings.Builder
	b.WriteString("# 运行结果摘要\n\n")
	b.WriteString(fmt.Sprintf("- 任务：%s\n", strings.TrimSpace(record.TaskName)))
	b.WriteString(fmt.Sprintf("- 状态：%s\n", record.Status))
	b.WriteString(fmt.Sprintf("- 时间：%s\n", record.CreatedAt.Format(time.RFC3339)))
	if strings.TrimSpace(record.LogPath) != "" {
		b.WriteString(fmt.Sprintf("- 日志：%s\n", record.LogPath))
	}
	if len(record.InputPaths) > 0 {
		b.WriteString("\n## 已读取资料\n\n")
		for _, path := range record.InputPaths {
			if strings.TrimSpace(path) != "" {
				b.WriteString("- " + path + "\n")
			}
		}
	}
	if len(record.Generated) > 0 {
		b.WriteString("\n## 生成文件\n\n")
		for _, path := range record.Generated {
			if strings.TrimSpace(path) != "" {
				b.WriteString("- " + path + "\n")
			}
		}
	}
	if len(record.Warnings) > 0 {
		b.WriteString("\n## 风险与待确认\n\n")
		for _, warning := range record.Warnings {
			if strings.TrimSpace(warning) != "" {
				b.WriteString("- " + warning + "\n")
			}
		}
	}
	if len(record.Errors) > 0 {
		b.WriteString("\n## 失败原因\n\n")
		for _, failure := range record.Errors {
			if strings.TrimSpace(failure) != "" {
				b.WriteString("- " + failure + "\n")
			}
		}
	}
	if len(record.NextActions) > 0 {
		b.WriteString("\n## 下一步动作\n\n")
		for _, action := range record.NextActions {
			if strings.TrimSpace(action) != "" {
				b.WriteString("- " + action + "\n")
			}
		}
	}
	return b.String()
}
