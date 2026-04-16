.PHONY: checks
checks: lint licensecheck govet ineffassign staticcheck

.PHONY: licensecheck
licensecheck:
	@echo Running license check
	@find . -name '*.go' | grep -v .pb.go | xargs addlicense -check || (echo "Missing license headers"; exit 1)


.PHONY: govet
govet:
	@echo Running go vet
	@go vet -all $(shell go list -f '{{.Dir}}' ./...) || (echo "Found some issues identified by 'go vet -all'. Please fix them!"; exit 1;)

.PHONY: staticcheck
staticcheck:
	@echo Running staticcheck
	@{ \
	OUTPUT="$$(staticcheck -tests=false ./... | grep -v .pb.go || true)"; \
	if [ -n "$$OUTPUT" ]; then \
		echo "The following staticcheck issues were flagged:"; \
		echo "$$OUTPUT"; \
		exit 1; \
	fi \
	}

.PHONY: gocyclo
gocyclo:
	@echo Running gocyclo
	@gocyclo -over 15 $(shell go list -f '{{.Dir}}' ./...) || (echo "Found some code with a Cyclomatic complexity over 15! Better refactor"; exit 1;)

.PHONY: ineffassign
ineffassign:
	@echo Running ineffassign
	@ineffassign $(shell go list -f '{{.Dir}}' ./...)

#########################
# Lint
#########################

.PHONY: lint
lint: ## Run linter
	@echo "Running Go Linters..."
	golangci-lint run --color=always --new-from-rev=main --timeout=4m

.PHONY: lint-auto-fix
lint-auto-fix: ## Run linter with auto-fix
	@echo "Running Go Linters with auto-fix..."
	golangci-lint run --color=always --new-from-rev=main --timeout=4m --fix

.PHONY: lint-fmt
lint-fmt:
	@echo "Running Go Formatters..."
	golangci-lint fmt
