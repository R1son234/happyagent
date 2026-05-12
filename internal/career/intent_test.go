package career

import "testing"

func TestClassifyIntent(t *testing.T) {
	tests := []struct {
		input string
		want  CareerIntent
	}{
		{"我把简历和 JD 放进 inbox 了", CareerIntentIngest},
		{"我把简历 JD 和面经放到 inbox 了，帮我记录并分析一下", CareerIntentAnalyze},
		{"帮我看看匹配度", CareerIntentAnalyze},
		{"优化简历", CareerIntentResumeReview},
		{"帮我准备一下面试", CareerIntentInterviewBrief},
		{"刚面完，帮我复盘一下", CareerIntentInterviewReview},
		{"rewrite my resume", CareerIntentResumeReview},
		{"看看当前资料状态", CareerIntentStatus},
	}
	for _, tc := range tests {
		got := ClassifyIntent(tc.input)
		if got.Intent != tc.want {
			t.Fatalf("ClassifyIntent(%q) = %s, want %s", tc.input, got.Intent, tc.want)
		}
	}
}
