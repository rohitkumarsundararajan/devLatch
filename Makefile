MODULE  := $(shell go list -m)
VERSION ?= dev
LDFLAGS := -X $(MODULE)/internal/version.Version=$(VERSION)
FRONTEND := examples/demo-frontend

.DEFAULT_GOAL := help
.PHONY: help build build-demo test race cover vet fmt fmt-check lint check \
        demo-backend demo-frontend-install demo-frontend frontend-check clean

help: ## list targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-22s %s\n",$$1,$$2}'

build: ## build bin/devlatch
	go build -ldflags "$(LDFLAGS)" -o bin/devlatch ./cmd/devlatch

build-demo: ## build bin/demo-backend
	go build -o bin/demo-backend ./examples/demo-backend

test: ## run unit tests
	go test ./...

race: ## run tests with the race detector
	go test -race ./...

cover: ## tests + coverage summary
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

vet: ## go vet
	go vet ./...

fmt: ## format Go code
	gofmt -w .

fmt-check: ## fail if any Go file is not gofmt-clean
	@test -z "$$(gofmt -l .)" || (echo "not gofmt-clean:"; gofmt -l .; exit 1)

lint: ## golangci-lint (must be installed)
	golangci-lint run

check: fmt-check vet race ## what CI runs for Go (minus lint)

demo-backend: ## run the demo backend on 127.0.0.1:8000
	go run ./examples/demo-backend

demo-frontend-install: ## npm install for the demo frontend
	cd $(FRONTEND) && npm install

demo-frontend: ## run the demo frontend on :5173 (proxies /api to :8000)
	cd $(FRONTEND) && npm run dev

frontend-check: ## typecheck + build the demo frontend
	cd $(FRONTEND) && npm run build

clean: ## remove build output
	rm -rf bin coverage.out $(FRONTEND)/dist
