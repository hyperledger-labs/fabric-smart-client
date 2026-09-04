# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

TOP = .

# pinned versions
FABRIC_VERSION ?= 3.1.4
FABRIC_TWO_DIGIT_VERSION = $(shell echo $(FABRIC_VERSION) | cut -d '.' -f 1,2)

FABRIC_X_TOOLS_VERSION ?= v1.0.1
FABRIC_X_COMMITTER_VERSION ?= 1.0.4

# need to install fabric binaries outside of fsc tree for now (due to chaincode packaging issues)
FABRIC_BINARY_BASE ?= $(PWD)/../fabric
FAB_BINS ?= $(FABRIC_BINARY_BASE)/bin
# The fabric-x tools ship a configtxgen and a cryptogen of their own. Installing
# them into $(FAB_BINS) would overwrite fabric's, so they get their own
# subdirectory there and both toolchains can be installed at once. Keep in sync
# with fxconfig.BinSubdir.
FABRIC_X_BINS ?= $(FAB_BINS)/fabric-x

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
install-fabricx-tools: ## Install the fabric-x tools in $(FABRIC_X_BINS)
	@env GOBIN=$(FABRIC_X_BINS) go install $(GO_FLAGS) github.com/hyperledger/fabric-x/tools/fxconfig@$(FABRIC_X_TOOLS_VERSION)
	@env GOBIN=$(FABRIC_X_BINS) go install $(GO_FLAGS) github.com/hyperledger/fabric-x/tools/configtxgen@$(FABRIC_X_TOOLS_VERSION)
	@env GOBIN=$(FABRIC_X_BINS) go install $(GO_FLAGS) github.com/hyperledger/fabric-x/tools/cryptogen@$(FABRIC_X_TOOLS_VERSION)

.PHONY: install-fabric-bins
install-fabric-bins: ## Install fabric binaries in $(FABRIC_BINARY_BASE)
	./ci/scripts/download_fabric.sh $(FABRIC_BINARY_BASE) $(FABRIC_VERSION)
	@test -x $(FABRIC_BINARY_BASE)/builders/ccaas/bin/build || { \
	  echo "==> $(FABRIC_BINARY_BASE)/builders/ccaas is missing."; \
	  echo "    Fabric $(FABRIC_VERSION) ships it; remove the directory and re-run."; \
	  exit 1; \
	}

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

.PHONY: chaincode-images
chaincode-images: ## Build container images for the in-tree CCaaS chaincodes
	@grep -vE '^\s*(#|$$)' scripts/chaincode/images.txt | \
	  while read -r image module pkg; do \
	    scripts/chaincode/build-image.sh "$$image" "$$module" "$$pkg" || exit 1; \
	  done

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

# we use a multi-module repo structure here and therefore need to carefully collect packages for unit tests
GO_PACKAGES = $$(go list ./...)
# Unit-testable packages of the integration module: the nwo harness plus the shared
# helpers the suites import. The suites themselves are integration tests, not unit tests.
INTEGRATION_UNIT_PACKAGES = ./nwo/... ./fabric/common/...
# The libp2p comm host is its own module, so the root `go list ./...` cannot see
# it. Named here so `unit-tests` can step into it explicitly -- without this its
# host-conformance tests (shared with the websocket host) never run.
LIBP2P_HOST_MODULE = platform/view/services/comm/host/libp2p
GO_PACKAGES_SDK = $$(go list ./... | grep '/sdk/dig$$')
GO_TEST_PARAMS ?= -race -cover
TEST_PKGS ?= $(GO_PACKAGES)

# Instrument every package under the root module so that coverage is credited
# even when a package is exercised only by tests living in another package
# (e.g. the exported helpers in .../sql/common used by the sqlite/postgres tests).
# Kept separate from GO_TEST_PARAMS because CI overrides GO_TEST_PARAMS wholesale.
GO_COVERPKG ?= -coverpkg=./...

.PHONY: unit-tests
unit-tests: ## Run unit tests
	@echo "Running unit tests..."
	export FABRIC_LOGGING_SPEC=error; \
	export FAB_BINS=$(FAB_BINS); \
	rc=0; \
	go test $(GO_TEST_PARAMS) $(GO_COVERPKG) --skip '(Postgres)' $(TEST_PKGS) || rc=1; \
	go test -C integration $(GO_TEST_PARAMS) --skip '(Postgres)' $(INTEGRATION_UNIT_PACKAGES) || rc=1; \
	go test -C $(LIBP2P_HOST_MODULE) $(GO_TEST_PARAMS) ./... || rc=1; \
	exit $$rc

.PHONY: unit-tests-postgres
unit-tests-postgres: ## Run unit tests for postgres (requires container images as defined in testing-docker-images)
	@echo "Running unit tests..."
	export FABRIC_LOGGING_SPEC=error; \
	go test $(GO_TEST_PARAMS) $(GO_COVERPKG) --run '(Postgres)' $(TEST_PKGS)

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
INTEGRATION_TARGETS += fsc-signedpingpong

## fabric section
INTEGRATION_TARGETS += fabric-atsa
INTEGRATION_TARGETS += fabric-atsachaincode
INTEGRATION_TARGETS += fabric-configupdate
INTEGRATION_TARGETS += fabric-events
INTEGRATION_TARGETS += fabric-iou
# fabric/iou runs both comm types. They are split into two targets so CI runs
# them as parallel matrix entries rather than serially in one job.
INTEGRATION_TARGETS += fabric-iou-libp2p
INTEGRATION_TARGETS += fabric-runtimeconfig
INTEGRATION_TARGETS += fabric-stoprestart
INTEGRATION_TARGETS += fabric-twonets

## hsm section (require -tags pkcs11 for test binary compilation)
HSM_INTEGRATION_TARGETS = fabric-iouhsm

## fabricx section
INTEGRATION_TARGETS += fabricx-iou
INTEGRATION_TARGETS += fabricx-atsa
INTEGRATION_TARGETS += fabricx-simple
INTEGRATION_TARGETS += fabricx-deployment
INTEGRATION_TARGETS += fabricx-multiendorsement
INTEGRATION_TARGETS += fabricx-configupdate

# Targets are normally named <platform>-<suite> and map onto
# ./integration/<platform>/<suite>. Targets that run one suite under a
# different name need an explicit directory.
INTEGRATION_DIR_fabric-iou-libp2p = fabric/iou

# Per-target ginkgo flags. A suite that declares more than one p2p comm type is
# split into one target per type. The websocket target uses a negative filter
# so a spec added later without a comm-type label still runs there, rather
# than silently running in neither job.
INTEGRATION_FLAGS_fabric-iou        = --label-filter='!libp2p'
INTEGRATION_FLAGS_fabric-iou-libp2p = --label-filter=libp2p

integration_default_dir = $(firstword $(subst -, ,$(1)))/$(subst $(firstword $(subst -, ,$(1)))-,,$(1))
integration_dir = $(or $(INTEGRATION_DIR_$(1)),$(call integration_default_dir,$(1)))

.PHONE: list-integration-tests
list-integration-tests: ## List all integration tests
	@$(foreach t,$(INTEGRATION_TARGETS) $(HSM_INTEGRATION_TARGETS),echo "$(t)";)

.PHONY: integration-tests
integration-tests: $(addprefix integration-tests-,$(INTEGRATION_TARGETS) $(HSM_INTEGRATION_TARGETS)) ## Run all integration tests

$(addprefix integration-tests-,$(HSM_INTEGRATION_TARGETS)) : integration-tests-%:
	export FAB_BINS=$(FAB_BINS); \
		cd ./integration/$(call integration_dir,$*); \
		GOFLAGS="-tags=pkcs11" ginkgo $(GINKGO_TEST_OPTS) $(INTEGRATION_FLAGS_$*) .

$(addprefix integration-tests-,$(INTEGRATION_TARGETS)) : integration-tests-%:
	export FAB_BINS=$(FAB_BINS); \
		cd ./integration/$(call integration_dir,$*); \
		ginkgo $(GINKGO_TEST_OPTS) $(INTEGRATION_FLAGS_$*) .

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
	@./scripts/gomate.sh tidy

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
	@env FABRIC_LOGGING_SPEC=error FAB_BINS=$(FAB_BINS) go test $(GO_TEST_PARAMS) $(GO_COVERPKG) -coverprofile=coverage.tmp $(TEST_PKGS)
	@./scripts/filter-coverage.sh coverage.tmp coverage.out
	@go tool cover -func=coverage.out | tail -n 1
	@rm coverage.tmp
