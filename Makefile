.DEFAULT_GOAL := help

PROJECT_NAME := ChaChaChá
GO_FILES := $(shell find . -type f -name '*.go')

#-----------------------------------------------------------------------
# Help
#-----------------------------------------------------------------------
.PHONY: help
help:
	@echo "------------------------------------------------------------------------"
	@echo "${PROJECT_NAME}"
	@echo "------------------------------------------------------------------------"
	@grep -E '^[a-zA-Z0-9_/%\-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

#-----------------------------------------------------------------------
# Code Quality & Security
#-----------------------------------------------------------------------
.PHONY: format vet vulncheck check-all
format: ## Format Go code
		@echo "Formatting code..."
		@gofmt -w $(GO_FILES)
		@echo "Done formatting."

vet: ## Run go vet
		@echo "Running go vet..."
		@go vet ./...
		@echo "Code looks good!"

vulncheck: ## Check for known vulnerabilities
		@echo "Checking for vulnerabilities..."
		@go install golang.org/x/vuln/cmd/govulncheck@latest
		@govulncheck ./...

check-all: format vet vulncheck ## Run all code quality checks
		@echo "All quality checks completed!"

#-----------------------------------------------------------------------
# Tests
#-----------------------------------------------------------------------
.PHONY: test-unit
test-unit: ## Run unit tests
	go test -short -v -count=1 -race -cover ./...

test-e2e: ## Run e2e tests
	go test -v -count=1 -race -cover ./test/...
