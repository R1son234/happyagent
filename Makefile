BINARY := bin/happyagent
EVAL_BINARY := bin/happyagent-eval
GOCACHE_DIR := $(CURDIR)/.gocache
GOMODCACHE_DIR := $(CURDIR)/.gomodcache

.PHONY: build build-eval run check test eval-smoke

build:
	mkdir -p bin
	mkdir -p $(GOCACHE_DIR)
	mkdir -p $(GOMODCACHE_DIR)
	GOCACHE=$(GOCACHE_DIR) GOMODCACHE=$(GOMODCACHE_DIR) go build -o $(BINARY) ./cmd/happyagent

build-eval:
	mkdir -p bin
	mkdir -p $(GOCACHE_DIR)
	mkdir -p $(GOMODCACHE_DIR)
	GOCACHE=$(GOCACHE_DIR) GOMODCACHE=$(GOMODCACHE_DIR) go build -o $(EVAL_BINARY) ./cmd/happyagent-eval

run: build
	./$(BINARY)

check:
	mkdir -p $(GOCACHE_DIR)
	mkdir -p $(GOMODCACHE_DIR)
	GOCACHE=$(GOCACHE_DIR) GOMODCACHE=$(GOMODCACHE_DIR) go build ./...

test:
	mkdir -p $(GOCACHE_DIR)
	mkdir -p $(GOMODCACHE_DIR)
	GOCACHE=$(GOCACHE_DIR) GOMODCACHE=$(GOMODCACHE_DIR) go test ./...

eval-smoke: build-eval
	./$(EVAL_BINARY) -cases eval/smoke_cases.json -output logs/eval/smoke-report.json -trace-dir logs/eval/smoke-traces
