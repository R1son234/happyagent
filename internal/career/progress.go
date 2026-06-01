package career

import "context"

type ProgressReporter interface {
	Status(message string)
	StepStart(stepIndex int)
	ToolStart(toolName string, arguments []byte)
	ToolEnd(toolName string, arguments []byte, succeeded bool)
}

type progressReporterContextKey struct{}

func WithProgressReporter(ctx context.Context, reporter ProgressReporter) context.Context {
	if reporter == nil {
		return ctx
	}
	return context.WithValue(ctx, progressReporterContextKey{}, reporter)
}

func ProgressReporterFromContext(ctx context.Context) ProgressReporter {
	if ctx == nil {
		return nil
	}
	reporter, _ := ctx.Value(progressReporterContextKey{}).(ProgressReporter)
	return reporter
}

func reportProgressStatus(ctx context.Context, message string) {
	if reporter := ProgressReporterFromContext(ctx); reporter != nil {
		reporter.Status(message)
	}
}
