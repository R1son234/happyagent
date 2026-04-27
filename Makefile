BINARY := bin/happyagent
EVAL_BINARY := bin/happyagent-eval

.PHONY: build build-eval run check test eval-smoke

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
