.PHONY: test test-verbose clean help

# Default target
help:
	@echo "TokenX - FabricX Token Management"
	@echo ""
	@echo "Available targets:"
	@echo "  test          - Run integration tests"
	@echo "  test-verbose  - Run tests with verbose output"
	@echo "  clean         - Clean generated artifacts"
	@echo "  help          - Show this help message"

# Run integration tests
test:
	go test ./...

# Run tests with verbose output
test-verbose:
	go test -v ./...

# Run with Ginkgo
test-ginkgo:
	ginkgo -v

# Clean generated artifacts
clean:
	rm -rf ./out
	rm -rf ./testdata

# Generate OpenAPI documentation (requires swagger-codegen)
docs:
	@echo "OpenAPI spec available at: api/openapi.yaml"
	@echo "View with: npx @redocly/cli preview-docs api/openapi.yaml"
