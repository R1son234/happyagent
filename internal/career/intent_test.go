package career

import "testing"

func TestClassifyIntent(t *testing.T) {
	tests := []struct {
		input string
		want  CareerIntent
	}{
		{"我把简历和 JD 放进 inbox 了", CareerIntentIngest},
		{"我把简历 JD 和面经放到 inbox 了，帮我记录并分析一下", CareerIntentAnalyze},
		{"我把我的简历和面经放到inbox里了,你帮我记录并分析下", CareerIntentAnalyze},
		{"帮我看看匹配度", CareerIntentAnalyze},
		{"优化简历", CareerIntentResumeReview},
		{"帮我准备一下面试", CareerIntentInterviewBrief},
		{"刚面完，帮我复盘一下", CareerIntentInterviewReview},
		{"rewrite my resume", CareerIntentResumeReview},
		{"看看当前资料状态", CareerIntentStatus},
		{"更新 memory：以后分析岗位时不要自动重新扫描 inbox", CareerIntentMemory},
		{"记住：我偏好远程工作", CareerIntentMemory},
		{"以后按这个规则来", CareerIntentMemory},
		{"不要再自动扫描 inbox 了", CareerIntentMemory},
		{"我的偏好是先看 JD 再看简历", CareerIntentMemory},
		{"更新记忆：面试准备用中文", CareerIntentMemory},
	}
	for _, tc := range tests {
		got := ClassifyIntent(tc.input)
		if got.Intent != tc.want {
			t.Fatalf("ClassifyIntent(%q) = %s, want %s", tc.input, got.Intent, tc.want)
		}
	}
}

func TestClassifyIntentMemoryHasHigherPriorityThanAnalyze(t *testing.T) {
	// "帮我分析" would normally match analyze, but "记住" should take priority.
	got := ClassifyIntent("记住：以后帮我分析时不要自动扫描 inbox")
	if got.Intent != CareerIntentMemory {
		t.Fatalf("expected memory intent when both signals present, got %s", got.Intent)
	}
}
