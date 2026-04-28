BINARY := bin/happyagent
EVAL_BINARY := bin/happyagent-eval

.PHONY: build build-eval run check test eval-smoke eval-profiles

build:
	mkdir -p bin
	go build -o $(BINARY) ./cmd/happyagent

build-eval:
	mkdir -p bin
	go build -o $(EVAL_BINARY) ./cmd/happyagent-eval

run: build
	./$(BINARY)

check:
	go build ./...

test:
	go test ./...

eval-smoke: build-eval
	./$(EVAL_BINARY) -cases eval/smoke_cases.json -output logs/eval/smoke-report.json -trace-dir logs/eval/smoke-traces

eval-profiles: build-eval
	./$(EVAL_BINARY) -cases eval/profile_cases.json -output logs/eval/profile-report.json -trace-dir logs/eval/profile-traces
