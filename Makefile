# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

TOP = .

# pinned versions
FABRIC_VERSION ?= 3.1.1
FABRIC_TWO_DIGIT_VERSION = $(shell echo $(FABRIC_VERSION) | cut -d '.' -f 1,2)

FABRIC_X_TOOLS_VERSION ?= v0.0.5
FABRIC_X_COMMITTER_VERSION ?= 0.1.5

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

.PHONY: install-linter
install-linter-tool: ## Install linter in $(go env GOPATH)/bin
	@echo "Installing golangci Linter"
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(go env GOPATH)/bin v2.4.0

.PHONY: install-fxconfig
install-fxconfig: ## Install fxconfig in $(FAB_BINS)
	@env GOBIN=$(FAB_BINS) go install $(GO_FLAGS) github.com/hyperledger/fabric-x/tools/fxconfig@$(FABRIC_X_TOOLS_VERSION)

.PHONY: install-configtxgen
install-configtxgen: ## Install configtxgen in $(FAB_BINS)
	@env GOBIN=$(FAB_BINS) go install $(GO_FLAGS) github.com/hyperledger/fabric-x/tools/configtxgen@$(FABRIC_X_TOOLS_VERSION)

.PHONY: install-fabric-bins
install-fabric-bins: ## Install fabric binaries in $(FABRIC_BINARY_BASE)
	./ci/scripts/download_fabric.sh $(FABRIC_BINARY_BASE) $(FABRIC_VERSION)

.PHONY: install-softhsm
install-softhsm: ## Install softhsm
	./ci/scripts/install_softhsm.sh

.PHONY: install-fsccli
install-fsccli: ## Install fsccli
	@go install ./cmd/fsccli

#########################
# Generate protos
#########################

.PHONY: protos
protos: ## Build all proto files
	./scripts/compile_proto.sh

#########################
# Container
#########################

.PHONY: docker-images
docker-images: fabric-docker-images monitoring-docker-images testing-docker-images

.PHONY: fabric-docker-images
fabric-docker-images: ## Pull fabric images
	docker pull hyperledger/fabric-baseos:$(FABRIC_TWO_DIGIT_VERSION)
	docker image tag hyperledger/fabric-baseos:$(FABRIC_TWO_DIGIT_VERSION) hyperledger/fabric-baseos:latest
	docker pull hyperledger/fabric-ccenv:$(FABRIC_TWO_DIGIT_VERSION)
	docker image tag hyperledger/fabric-ccenv:$(FABRIC_TWO_DIGIT_VERSION) hyperledger/fabric-ccenv:latest

.PHONY: fabricx-docker-images
fabricx-docker-images: ## Pull fabric-x images
	docker pull hyperledger/fabric-x-committer-test-node:$(FABRIC_X_COMMITTER_VERSION)

.PHONY: monitoring-docker-images
monitoring-docker-images: ## Pull images for monitoring
	docker pull ghcr.io/hyperledger-labs/explorer-db:latest
	docker pull ghcr.io/hyperledger-labs/explorer:latest
	docker pull prom/prometheus:latest
	docker pull grafana/grafana:latest
	docker pull cr.jaegertracing.io/jaegertracing/jaeger:2.12.0

.PHONY: testing-docker-images
testing-docker-images: ## Pull images for system testing
	docker pull postgres:16.2-alpine
	docker tag postgres:16.2-alpine fsc.itests/postgres:latest

#########################
# Tests
#########################

# include the checks target
include $(TOP)/checks.mk

GO_PACKAGES = $$(go list ./... | grep -Ev '/(integration/|mock|fake)'; go list ./integration/nwo)
GO_PACKAGES_SDK = $$(go list ./... | grep '/sdk/dig$$')
GO_TEST_PARAMS ?= -race -cover

.PHONY: unit-tests
unit-tests: ## Run unit tests
	@echo "Running unit tests..."
	export FABRIC_LOGGING_SPEC=error; \
	export FAB_BINS=$(FAB_BINS); \
	go test $(GO_TEST_PARAMS) --skip '(Postgres)' $(GO_PACKAGES)

.PHONY: unit-tests-postgres
unit-tests-postgres: ## Run unit tests for postgres (requires container images as defined in testing-docker-images)
	@echo "Running unit tests..."
	export FABRIC_LOGGING_SPEC=error; \
	go test $(GO_TEST_PARAMS) --run '(Postgres)' $(GO_PACKAGES)

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
INTEGRATION_TARGETS += fabric-iouhsm
INTEGRATION_TARGETS += fabric-stoprestart
INTEGRATION_TARGETS += fabric-twonets

## fabricx section
INTEGRATION_TARGETS += fabricx-iou
INTEGRATION_TARGETS += fabricx-simple

.PHONE: list-integration-tests
list-integration-tests: ## List all integration tests
	@$(foreach t,$(INTEGRATION_TARGETS),echo "$(t)";)

.PHONY: integration-tests
integration-tests: $(addprefix integration-tests-,$(INTEGRATION_TARGETS)) ## Run all integration tests

$(addprefix integration-tests-,$(INTEGRATION_TARGETS)) : integration-tests-%:
	export FAB_BINS=$(FAB_BINS); \
		cd ./integration/$(firstword $(subst -, ,$*))/$(subst $(firstword $(subst -, ,$*))-,,$*); \
		ginkgo $(GINKGO_TEST_OPTS) .

#########################
# Cleaning
#########################

.PHONY: clean
clean: $(addprefix clean-,$(INTEGRATION_TARGETS)) ## Clean generated testdata
	rm -rf ./cmd/fsccli/out

$(addprefix clean-,$(INTEGRATION_TARGETS)) : clean-%:
	rm -rf ./integration/$(firstword $(subst -, ,$*))/$(subst $(firstword $(subst -, ,$*))-,,$*)/out

.PHONY: tidy
tidy: ## Run go mod tidy everywhere
	go mod tidy
	cd tools; go mod tidy
	cd platform/fabric/services/state/cc/query; go mod tidy

.PHONY: clean-fabric-peer-images
clean-fabric-peer-images:
	docker images -a | grep "_peer_" | awk '{print $3}' | xargs docker rmi
