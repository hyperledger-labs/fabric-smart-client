# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

TOP = .

# pinned versions
FABRIC_VERSION ?= 3.1.4
FABRIC_TWO_DIGIT_VERSION = $(shell echo $(FABRIC_VERSION) | cut -d '.' -f 1,2)

FABRIC_X_TOOLS_VERSION ?= v1.0.0
FABRIC_X_COMMITTER_VERSION ?= 1.0.0

# need to install fabric binaries outside of fsc tree for now (due to chaincode packaging issues)
FABRIC_BINARY_BASE ?= $(PWD)/../fabric
FAB_BINS ?= $(FABRIC_BINARY_BASE)/bin

# integration test options
GINKGO_TEST_OPTS ?=
GINKGO_TEST_OPTS += --keep-going

# Run `make help` to find the supported targets
.DEFAULT_GOAL := help

.PHONY: help
help: ## List all commands with documentation
	@echo "Available commands:"
	@awk 'BEGIN {FS = ":.*?## "}; /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-25s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

#########################
# Install tools
#########################

.PHONY: install-tools
install-tools: ## Install all tools
# Thanks for great inspiration https://marcofranssen.nl/manage-go-tools-via-go-modules
	@echo Installing tools from tools/tools.go
	@cd tools; cat tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -tI % go install %

.PHONY: install-linter-tool
install-linter-tool: ## Install linter in $(go env GOPATH)/bin
	@echo "Installing golangci Linter"
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $$(go env GOPATH)/bin v2.12.2

.PHONY: install-fabricx-tools
install-fabricx-tools: ## Install fxconfig in $(FAB_BINS)
	@env GOBIN=$(FAB_BINS) go install $(GO_FLAGS) github.com/hyperledger/fabric-x/tools/fxconfig@$(FABRIC_X_TOOLS_VERSION)
	@env GOBIN=$(FAB_BINS) go install $(GO_FLAGS) github.com/hyperledger/fabric-x/tools/configtxgen@$(FABRIC_X_TOOLS_VERSION)
	@env GOBIN=$(FAB_BINS) go install $(GO_FLAGS) github.com/hyperledger/fabric-x/tools/cryptogen@$(FABRIC_X_TOOLS_VERSION)

.PHONY: install-fabric-bins
install-fabric-bins: ## Install fabric binaries in $(FABRIC_BINARY_BASE)
	./ci/scripts/download_fabric.sh $(FABRIC_BINARY_BASE) $(FABRIC_VERSION)

.PHONY: install-softhsm
install-softhsm: ## Install softhsm
	./ci/scripts/install_softhsm.sh

.PHONY: install-fsccli
install-fsccli: ## Install fsccli
	cd integration; go install ./nwo/cmd/fsccli

#########################
# Generate protos
#########################

.PHONY: generate-protos
generate-protos: ## Delete all protoc-generated files and regenerate via compile_proto.sh
	@./scripts/find-protos.sh > /dev/null
	@./scripts/find-protos.sh --delete
	./scripts/compile_proto.sh

#########################
# Generate mocks
#########################

.PHONY: generate-mocks
generate-mocks: ## Delete all counterfeiter mock folders and regenerate via go generate ./...
	@./scripts/find-mocks.sh > /dev/null
	@./scripts/find-mocks.sh --delete
	go generate ./...

#########################
# Container
#########################

.PHONY: pull-images-fabric fabric-baseos fabric-ccenv
pull-images-fabric: fabric-baseos fabric-ccenv ## Pull fabric images

.PHONY: pull-images-fabricx fabric-x-committer-test-node
pull-images-fabricx: fabric-x-committer-test-node ## Pull fabric-x images

.PHONY: pull-images-monitoring explorer-db explorer prometheus grafana jaeger
pull-images-monitoring: explorer-db explorer prometheus grafana jaeger ## Pull images for monitoring

.PHONY: pull-images-database postgres
pull-images-database: postgres ## Pull images for system testing

fabric-baseos:
	docker pull ghcr.io/hyperledger/fabric-baseos:$(FABRIC_TWO_DIGIT_VERSION)
	docker tag ghcr.io/hyperledger/fabric-baseos:$(FABRIC_TWO_DIGIT_VERSION) hyperledger/fabric-baseos:latest

fabric-ccenv:
	docker pull ghcr.io/hyperledger/fabric-ccenv:$(FABRIC_TWO_DIGIT_VERSION)
	docker tag ghcr.io/hyperledger/fabric-ccenv:$(FABRIC_TWO_DIGIT_VERSION) hyperledger/fabric-ccenv:latest

fabric-x-committer-test-node:
	docker pull ghcr.io/hyperledger/fabric-x-committer-test-node:$(FABRIC_X_COMMITTER_VERSION)
	docker tag ghcr.io/hyperledger/fabric-x-committer-test-node:$(FABRIC_X_COMMITTER_VERSION) hyperledger/fabric-x-committer-test-node:$(FABRIC_X_COMMITTER_VERSION)

explorer-db:
	docker pull ghcr.io/hyperledger-labs/explorer-db:latest

explorer:
	docker pull ghcr.io/hyperledger-labs/explorer:latest

prometheus:
	docker pull prom/prometheus:latest

grafana:
	docker pull grafana/grafana:latest

jaeger:
	docker pull cr.jaegertracing.io/jaegertracing/jaeger:2.12.0

postgres:
	docker pull postgres:16.2-alpine
	docker tag postgres:16.2-alpine fsc.itests/postgres:latest

#########################
# Tests
#########################

# include the checks target
include $(TOP)/checks.mk

GO_PACKAGES = $$(go list ./... | grep -Ev '/(integration/)'; go list ./integration/nwo/...)
GO_PACKAGES_SDK = $$(go list ./... | grep '/sdk/dig$$')
GO_TEST_PARAMS ?= -race -cover
TEST_PKGS ?= $(GO_PACKAGES)

.PHONY: unit-tests
unit-tests: ## Run unit tests
	@echo "Running unit tests..."
	export FABRIC_LOGGING_SPEC=error; \
	export FAB_BINS=$(FAB_BINS); \
	go test $(GO_TEST_PARAMS) --skip '(Postgres)' $(TEST_PKGS)

.PHONY: unit-tests-postgres
unit-tests-postgres: ## Run unit tests for postgres (requires container images as defined in testing-docker-images)
	@echo "Running unit tests..."
	export FABRIC_LOGGING_SPEC=error; \
	go test $(GO_TEST_PARAMS) --run '(Postgres)' $(TEST_PKGS)

.PHONY: unit-tests-sdk
unit-tests-sdk: ## Run sdk wiring tests
	@echo "Running SDK tests..."
	go test $(GO_TEST_PARAMS) --run "(TestWiring)" $(GO_PACKAGES_SDK)

run-otlp:
	cd platform/view/services/tracing; docker-compose up -d

INTEGRATION_TARGETS =

## fsc section
INTEGRATION_TARGETS += fsc-pingpong
INTEGRATION_TARGETS += fsc-stoprestart

## fabric section
INTEGRATION_TARGETS += fabric-atsa
INTEGRATION_TARGETS += fabric-atsachaincode
INTEGRATION_TARGETS += fabric-events
INTEGRATION_TARGETS += fabric-iou
INTEGRATION_TARGETS += fabric-stoprestart
INTEGRATION_TARGETS += fabric-twonets

## hsm section (require -tags pkcs11 for test binary compilation)
HSM_INTEGRATION_TARGETS = fabric-iouhsm

## fabricx section
INTEGRATION_TARGETS += fabricx-iou
INTEGRATION_TARGETS += fabricx-simple
INTEGRATION_TARGETS += fabricx-deployment
INTEGRATION_TARGETS += fabricx-multiendorsement

.PHONE: list-integration-tests
list-integration-tests: ## List all integration tests
	@$(foreach t,$(INTEGRATION_TARGETS) $(HSM_INTEGRATION_TARGETS),echo "$(t)";)

.PHONY: integration-tests
integration-tests: $(addprefix integration-tests-,$(INTEGRATION_TARGETS) $(HSM_INTEGRATION_TARGETS)) ## Run all integration tests

$(addprefix integration-tests-,$(HSM_INTEGRATION_TARGETS)) : integration-tests-%:
	export FAB_BINS=$(FAB_BINS); \
		cd ./integration/$(firstword $(subst -, ,$*))/$(subst $(firstword $(subst -, ,$*))-,,$*); \
		GOFLAGS="-tags=pkcs11" ginkgo $(GINKGO_TEST_OPTS) .

$(addprefix integration-tests-,$(INTEGRATION_TARGETS)) : integration-tests-%:
	export FAB_BINS=$(FAB_BINS); \
		cd ./integration/$(firstword $(subst -, ,$*))/$(subst $(firstword $(subst -, ,$*))-,,$*); \
		ginkgo $(GINKGO_TEST_OPTS) .

#########################
# Release
#########################

.PHONY: tag-release
tag-release: ## Create git tags for all modules at HEAD. Usage: make tag-release VERSION=v0.13.0 [DRY=1]
ifndef VERSION
	$(error VERSION is required. Usage: make tag-release VERSION=v0.13.0)
endif
	./scripts/tag-release.sh $(if $(DRY),--dry) $(VERSION)

#########################
# Cleaning
#########################

.PHONY: clean
clean: $(addprefix clean-,$(INTEGRATION_TARGETS) $(HSM_INTEGRATION_TARGETS)) ## Clean generated testdata
	rm -rf ./integration/nwo/cmd/fsccli/out
	rm -rf ./out

$(addprefix clean-,$(INTEGRATION_TARGETS) $(HSM_INTEGRATION_TARGETS)) : clean-%:
	rm -rf ./integration/$(firstword $(subst -, ,$*))/$(subst $(firstword $(subst -, ,$*))-,,$*)/out

.PHONY: tidy
tidy: ## Run go mod tidy everywhere
	go mod tidy
	cd tools; go mod tidy
	cd platform/fabric/services/state/cc/query; go mod tidy

.PHONY: fmt
fmt: ## Run gofmt on the entire project
	@echo "Running gofmt..."
	@gofmt -l -s -w .

.PHONY: clean-fabric-peer-images
clean-fabric-peer-images: ## Clean up generated fabric peer images
	docker images -a | grep "_peer_" | awk '{print $3}' | xargs docker rmi

.PHONY: coverage-local
coverage-local: ## Run unit tests and show filtered coverage
	@echo "Running unit tests with coverage..."
	@env FABRIC_LOGGING_SPEC=error FAB_BINS=$(FAB_BINS) go test $(GO_TEST_PARAMS) -coverprofile=coverage.tmp $(TEST_PKGS)
	@./scripts/filter-coverage.sh coverage.tmp coverage.out
	@go tool cover -func=coverage.out | tail -n 1
	@rm coverage.tmp
