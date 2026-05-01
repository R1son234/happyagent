package career

import "testing"

func TestClassifyInputDetectsMaterialTypes(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{
			name: "jd",
			text: "岗位职责：负责 Agent runtime 和 RAG 服务。任职要求：熟悉 Go、MCP、LLM tool calling。",
			want: WorkspaceTypeJD,
		},
		{
			name: "resume",
			text: "我的简历：工作经历包括后端开发，教育经历是计算机，专业技能包括 Go 和 Python。",
			want: WorkspaceTypeResume,
		},
		{
			name: "project",
			text: "项目名称 happyagent，技术栈 Go，架构包括 runtime、tools、store 和 trace。",
			want: WorkspaceTypeProject,
		},
		{
			name: "interview record",
			text: "刚才面试记录：面试官问了 MCP tool calling，我回答了 runtime 里的工具注册流程。",
			want: WorkspaceTypeInterviewRecord,
		},
		{
			name: "review note",
			text: "复习笔记：RAG 需要补一下 hybrid retrieval、rerank 和 citation 的知识点。",
			want: WorkspaceTypeReviewNote,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyInput(tt.text)
			if got.Type != tt.want {
				t.Fatalf("ClassifyInput() type = %q, want %q (%+v)", got.Type, tt.want, got)
			}
			if got.Confidence <= 0 {
				t.Fatalf("expected confidence, got %+v", got)
			}
		})
	}
}

func TestClassifyInputLeavesShortRequestsAsGeneral(t *testing.T) {
	got := ClassifyInput("帮我优化简历")
	if got.Type != WorkspaceTypeResume {
		t.Fatalf("expected resume signal, got %+v", got)
	}
	if got.ShouldSave {
		t.Fatalf("short request should not be auto-saved: %+v", got)
	}
}
