package career

import (
	"fmt"
	"strings"
	"time"
)

type JDSplitSection struct {
	Title   string
	Content string
}

func (w *Workspace) EnsureSplitJDMaterials(now time.Time) ([]WorkspaceItem, error) {
	_, index, err := w.Status()
	if err != nil {
		return nil, err
	}
	existingTitles := map[string]bool{}
	for _, item := range index.Items {
		if item.Type == WorkspaceTypeJD {
			existingTitles[item.Title] = true
		}
	}
	var created []WorkspaceItem
	for _, item := range index.Items {
		if item.Type != WorkspaceTypeJD {
			continue
		}
		content := readExcerpt(w, item.Path, 0)
		if shouldSkipJDSplit(content) {
			continue
		}
		sections := SplitJDSections(content)
		if len(sections) <= 1 {
			continue
		}
		for _, section := range sections {
			title := strings.TrimSpace(section.Title)
			if title == "" || existingTitles[title] {
				continue
			}
			createdItem, addErr := w.AddMaterial(WorkspaceTypeJD, section.Content, now)
			if addErr != nil {
				return created, addErr
			}
			existingTitles[createdItem.Title] = true
			created = append(created, createdItem)
		}
	}
	return created, nil
}

func shouldSkipJDSplit(content string) bool {
	return strings.Contains(content, "匹配度分析") || strings.Contains(content, "维度 | 匹配项 | 缺口项")
}

func SplitJDSections(content string) []JDSplitSection {
	lines := strings.Split(content, "\n")
	type candidate struct {
		title string
		start int
	}
	var candidates []candidate
	for i, line := range lines {
		clean := strings.TrimSpace(strings.Trim(line, "# 　\t"))
		if clean == "" || len([]rune(clean)) > 60 {
			continue
		}
		if looksLikeJDSectionTitle(lines, i) {
			candidates = append(candidates, candidate{title: clean, start: i})
		}
	}
	if len(candidates) <= 1 {
		return nil
	}
	var sections []JDSplitSection
	for i, c := range candidates {
		end := len(lines)
		if i+1 < len(candidates) {
			end = candidates[i+1].start
		}
		body := strings.TrimSpace(strings.Join(lines[c.start:end], "\n"))
		if body == "" || !containsJDStructure(body) {
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(body), "#") {
			body = fmt.Sprintf("# %s\n\n%s", c.title, strings.TrimSpace(strings.Join(lines[c.start+1:end], "\n")))
		}
		sections = append(sections, JDSplitSection{Title: c.title, Content: body})
	}
	if len(sections) <= 1 {
		return nil
	}
	return sections
}

func looksLikeJDSectionTitle(lines []string, idx int) bool {
	line := strings.TrimSpace(strings.Trim(lines[idx], "# 　\t"))
	if line == "" || containsJDMarker(line) {
		return false
	}
	lookaheadEnd := idx + 8
	if lookaheadEnd > len(lines) {
		lookaheadEnd = len(lines)
	}
	for _, next := range lines[idx+1 : lookaheadEnd] {
		if containsJDMarker(next) {
			return true
		}
	}
	return false
}

func containsJDStructure(content string) bool {
	count := 0
	for _, line := range strings.Split(content, "\n") {
		if containsJDMarker(line) {
			count++
		}
	}
	return count >= 1
}

func containsJDMarker(line string) bool {
	markers := []string{"职位描述", "岗位描述", "岗位职责", "工作职责", "职位要求", "岗位要求", "任职要求", "任职资格", "基本要求", "加分项"}
	for _, marker := range markers {
		if strings.Contains(line, marker) {
			return true
		}
	}
	return false
}
