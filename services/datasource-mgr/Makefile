# ============================================================================
# Datasource Manager (数据源管理) Build Automation
# ============================================================================

.PHONY: build run test lint clean

build:
	go mod tidy
	go build -o bin/datasource-mgr ./cmd/server

run:
	go mod tidy
	go build -o bin/datasource-mgr ./cmd/server
	./bin/datasource-mgr

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
