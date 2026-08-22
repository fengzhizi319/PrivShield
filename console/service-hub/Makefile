# ============================================================================
# Service Hub (数据服务调度中枢) Build Automation
# ============================================================================

.PHONY: build run test lint clean

build:
	go mod tidy
	go build -o bin/service-hub ./cmd/server

run:
	go mod tidy
	go build -o bin/service-hub ./cmd/server
	./bin/service-hub

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
