package career

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRenderAndWriteWorkspaceArtifact(t *testing.T) {
	ws, err := OpenWorkspace(filepath.Join(t.TempDir(), "career"), time.Now())
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	if _, err := ws.AddMaterial(WorkspaceTypeJD, "Sample Role\n岗位职责：负责项目规划和跨部门协作。", time.Now()); err != nil {
		t.Fatalf("AddMaterial jd error = %v", err)
	}
	if _, err := ws.AddMaterial(WorkspaceTypeResume, "简历：负责项目推进、协作沟通和结果复盘。", time.Now()); err != nil {
		t.Fatalf("AddMaterial resume error = %v", err)
	}
	title, content, err := RenderWorkspaceArtifact(ws, "jd-match")
	if err != nil {
		t.Fatalf("RenderWorkspaceArtifact() error = %v", err)
	}
	if title != "JD Match Report" || !strings.Contains(content, "Active JD Signals") || !strings.Contains(content, "Resume Evidence") {
		t.Fatalf("unexpected artifact:\n%s", content)
	}
	path, err := ws.WriteArtifact("jd-match", title, content, time.Now())
	if err != nil {
		t.Fatalf("WriteArtifact() error = %v", err)
	}
	if !strings.HasPrefix(path, "reports/") {
		t.Fatalf("expected report path, got %s", path)
	}
}
