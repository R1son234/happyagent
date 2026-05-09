package career

import "strings"

type CareerIntent string

const (
	CareerIntentChat            CareerIntent = "chat"
	CareerIntentIngest          CareerIntent = "ingest"
	CareerIntentAnalyze         CareerIntent = "analyze"
	CareerIntentResumeReview    CareerIntent = "resume_review"
	CareerIntentInterviewBrief  CareerIntent = "interview_brief"
	CareerIntentGapPlan         CareerIntent = "gap_plan"
	CareerIntentInterviewReview CareerIntent = "interview_review"
	CareerIntentStatus          CareerIntent = "status"
)

type IntentClassification struct {
	Intent     CareerIntent `json:"intent"`
	Confidence float64      `json:"confidence"`
	Signals    []string     `json:"signals,omitempty"`
}

func ClassifyIntent(input string) IntentClassification {
	normalized := strings.ToLower(strings.TrimSpace(input))
	if normalized == "" {
		return IntentClassification{Intent: CareerIntentChat, Confidence: 0.2}
	}
	intentSignals := []struct {
		intent  CareerIntent
		signals []string
	}{
		{CareerIntentInterviewReview, []string{"刚面完", "面试官问我", "我回答", "interviewer asked", "asked me", "面试复盘"}},
		{CareerIntentResumeReview, []string{"优化简历", "改简历", "简历建议", "rewrite my resume", "rewrite resume", "resume review", "给我建议", "看看内容"}},
		{CareerIntentInterviewBrief, []string{"面试准备", "准备一面", "准备二面", "interview brief", "mock interview", "准备面试", "准备一下面试"}},
		{CareerIntentGapPlan, []string{"差距", "补什么", "gap", "提升计划", "gap plan"}},
		{CareerIntentAnalyze, []string{"匹配度", "适合吗", "帮我分析", "分析一下", "match", "analy", "评估"}},
		{CareerIntentIngest, []string{"放进 inbox", "放进来了", "看看资料", "整理文件", "扫描", "scan", "我放好了", "记录下来"}},
		{CareerIntentStatus, []string{"当前资料", "现在有什么资料", "状态", "status"}},
	}
	best := IntentClassification{Intent: CareerIntentChat, Confidence: 0.2}
	for _, candidate := range intentSignals {
		var matched []string
		for _, signal := range candidate.signals {
			if strings.Contains(normalized, strings.ToLower(signal)) {
				matched = append(matched, signal)
			}
		}
		if len(matched) == 0 {
			continue
		}
		confidence := 0.55 + float64(len(matched))*0.1
		if confidence > 0.95 {
			confidence = 0.95
		}
		if confidence > best.Confidence {
			best = IntentClassification{
				Intent:     candidate.intent,
				Confidence: confidence,
				Signals:    matched,
			}
		}
	}
	return best
}
