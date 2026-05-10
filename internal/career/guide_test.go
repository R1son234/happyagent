package career

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenWorkspaceCreatesDefaultGuide(t *testing.T) {
	root := filepath.Join(t.TempDir(), "career")
	ws, err := OpenWorkspace(root, time.Date(2026, 5, 10, 20, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, WorkspaceGuideFileName)); err != nil {
		t.Fatalf("expected default guide file: %v", err)
	}
	guide, err := ws.LoadGuide()
	if err != nil {
		t.Fatalf("LoadGuide() error = %v", err)
	}
	if guide.DemoTopic != "市场营销" {
		t.Fatalf("unexpected demo topic: %+v", guide)
	}
	if _, ok := guide.Directory(WorkspaceTypeJD); !ok {
		t.Fatalf("default guide missing JD rule")
	}
}

func TestWorkspaceGuideRejectsUnsafePaths(t *testing.T) {
	guide := DefaultWorkspaceGuide()
	guide.Directories[0].Path = "../outside"
	if err := guide.Validate(); err == nil {
		t.Fatalf("expected unsafe path to be rejected")
	}
}

func TestClassifyInputWithGuideUsesCustomSignals(t *testing.T) {
	guide := DefaultWorkspaceGuide()
	for i := range guide.Directories {
		if guide.Directories[i].Type == WorkspaceTypeRecord {
			guide.Directories[i].Signals = append(guide.Directories[i].Signals, "闪卡")
		}
	}
	got := ClassifyInputWithGuide("闪卡：今天复习岗位关键词和追问答案。", guide)
	if got.Type != WorkspaceTypeRecord {
		t.Fatalf("expected custom record signal, got %+v", got)
	}
	if got.Reason == "" || got.RulePath == "" {
		t.Fatalf("expected explainable classification, got %+v", got)
	}
}

func TestWorkspaceGuidePromptSummaryIncludesDirectoryResponsibilities(t *testing.T) {
	summary := DefaultWorkspaceGuide().PromptSummary()
	for _, expected := range []string{"Workspace directory guide", "jd", "experiences", "Required sections", "市场营销", "Sync rules"} {
		if !strings.Contains(summary, expected) {
			t.Fatalf("summary missing %q:\n%s", expected, summary)
		}
	}
}
