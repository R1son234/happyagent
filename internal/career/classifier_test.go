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
			text: "岗位职责：负责业务规划、跨部门协作、资料整理和结果复盘。任职要求：熟悉沟通协调、执行跟踪、文档沉淀和问题分析。",
			want: WorkspaceTypeJD,
		},
		{
			name: "resume",
			text: "我的简历：工作经历包括后端开发，教育经历是计算机，专业技能包括 Go 和 Python。",
			want: WorkspaceTypeResume,
		},
		{
			name: "project",
			text: "项目名称 happyagent，项目追问包括技术方案、证据口径和架构取舍。",
			want: WorkspaceTypePrepare,
		},
		{
			name: "public interview experience",
			text: "市场营销公开面经：一面问了用户增长、内容策略和复盘方法，二面追问高频题。",
			want: WorkspaceTypeExperiences,
		},
		{
			name: "interview record",
			text: "刚面完市场营销岗位，面试官问我项目复盘和协作方式，我回答了推进流程，现场表现一般。",
			want: WorkspaceTypeMyInterviews,
		},
		{
			name: "review note",
			text: "复习笔记：项目复盘需要补一下目标拆解、过程跟踪和结果量化的知识点。",
			want: WorkspaceTypeRecord,
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
