package career

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

const DefaultReportTemplatePath = "docs/templates/career-report.md.tmpl"

func RenderMarkdown(report Report, templatePath string) (string, error) {
	if strings.TrimSpace(templatePath) == "" {
		templatePath = DefaultReportTemplatePath
	}
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return "", fmt.Errorf("parse career report template %q: %w", templatePath, err)
	}
	var buffer bytes.Buffer
	if err := tmpl.Execute(&buffer, report); err != nil {
		return "", fmt.Errorf("render career report markdown: %w", err)
	}
	return buffer.String(), nil
}

func RenderResumeBullets(report Report) string {
	var builder strings.Builder
	builder.WriteString("# Resume Bullets\n\n")
	for _, bullet := range report.ResumeRewrite.Bullets {
		builder.WriteString("- ")
		builder.WriteString(bullet.Recommended)
		if strings.TrimSpace(bullet.Why) != "" {
			builder.WriteString("\n  - Why: ")
			builder.WriteString(bullet.Why)
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func RenderInterviewBrief(report Report) string {
	var builder strings.Builder
	builder.WriteString("# Interview Brief\n\n")
	builder.WriteString("## Project Pitch\n\n")
	builder.WriteString(report.InterviewBrief.ProjectPitch)
	builder.WriteString("\n\n## Architecture Talk Track\n\n")
	builder.WriteString(report.InterviewBrief.ArchitectureTalkTrack)
	builder.WriteString("\n\n## Tradeoffs\n\n")
	for _, item := range report.InterviewBrief.Tradeoffs {
		builder.WriteString("- ")
		builder.WriteString(item)
		builder.WriteString("\n")
	}
	builder.WriteString("\n## Questions To Expect\n\n")
	for _, item := range report.InterviewBrief.QuestionsToExpect {
		builder.WriteString("- ")
		builder.WriteString(item)
		builder.WriteString("\n")
	}
	return builder.String()
}

func RenderGapPlan(report Report) string {
	var builder strings.Builder
	builder.WriteString("# Project Gap Plan\n\n")
	for _, gap := range report.GapPlan {
		builder.WriteString("## ")
		builder.WriteString(gap.Priority)
		builder.WriteString(" - ")
		builder.WriteString(gap.Item)
		builder.WriteString("\n\n")
		if strings.TrimSpace(gap.WhyItMatters) != "" {
			builder.WriteString("Why it matters: ")
			builder.WriteString(gap.WhyItMatters)
			builder.WriteString("\n\n")
		}
		builder.WriteString("Acceptance: ")
		builder.WriteString(gap.Acceptance)
		builder.WriteString("\n\n")
	}
	return builder.String()
}
