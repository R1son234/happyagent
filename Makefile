BINARY := bin/happyagent
EVAL_BINARY := bin/happyagent-eval
GO := GOTOOLCHAIN=go1.25.0 go

.PHONY: build build-eval run check test eval-smoke eval-profiles eval-career

build:
	mkdir -p bin
	$(GO) build -o $(BINARY) ./cmd/happyagent

build-eval:
	mkdir -p bin
	$(GO) build -o $(EVAL_BINARY) ./cmd/happyagent-eval

run: build
	./$(BINARY)

check:
	$(GO) build ./...

test:
	$(GO) test ./...

eval-smoke: build-eval
	./$(EVAL_BINARY) -cases eval/smoke_cases.json -output logs/eval/smoke-report.json -trace-dir logs/eval/smoke-traces -summary logs/eval/smoke-summary.md

eval-profiles: build-eval
	./$(EVAL_BINARY) -cases eval/profile_cases.json -output logs/eval/profile-report.json -trace-dir logs/eval/profile-traces -summary logs/eval/profile-summary.md

eval-career: build-eval
	HAPPYAGENT_LOOP_MAX_STEPS=20 HAPPYAGENT_LLM_TIMEOUT_SECONDS=180 ./$(EVAL_BINARY) -cases eval/career_cases.json -output logs/eval/career-report.json -trace-dir logs/eval/career-traces -summary logs/eval/career-summary.md
