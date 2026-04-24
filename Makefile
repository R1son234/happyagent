BINARY := bin/happyagent
GOCACHE_DIR := $(CURDIR)/.gocache

.PHONY: build run check test

build:
	mkdir -p bin
	mkdir -p $(GOCACHE_DIR)
	GOCACHE=$(GOCACHE_DIR) go build -o $(BINARY) ./cmd/happyagent

run: build
	./$(BINARY)

check:
	mkdir -p $(GOCACHE_DIR)
	GOCACHE=$(GOCACHE_DIR) go build ./...

test:
	mkdir -p $(GOCACHE_DIR)
	GOCACHE=$(GOCACHE_DIR) go test ./...
