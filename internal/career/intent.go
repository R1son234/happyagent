package career

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
	CareerIntentMemory          CareerIntent = "memory"
)
