# ============================================================================
# Audit Log (脱敏审计日志) Build Automation
# ============================================================================

.PHONY: build run test lint clean

build:
	go mod tidy
	go build -o bin/audit-log ./cmd/server

run:
	go mod tidy
	go build -o bin/audit-log ./cmd/server
	./bin/audit-log

dev: run

test:
	go test ./...

test-unit:
	go test -short ./...

lint:
	go fmt ./...
	go vet ./...

clean:
	rm -rf bin
