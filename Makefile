# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

#########################
# Pinned versions
#########################

FABRIC_VERSION ?= 3.1.4
FABRIC_TWO_DIGIT_VERSION = $(shell echo $(FABRIC_VERSION) | cut -d '.' -f 1,2)

FABRIC_X_TOOLS_VERSION ?= v1.0.3
FABRIC_X_COMMITTER_VERSION ?= 1.0.7

# scripts/release reports FABRIC_VERSION, FABRIC_X_TOOLS_VERSION,
# FABRIC_X_COMMITTER_VERSION and POSTGRES_VERSION as the versions a release was
# tested with.
POSTGRES_VERSION ?= 16.2-alpine

# Monitoring images. nwo runs the same refs; TestImagePins in integration/nwo fails
# when the two disagree.
JAEGER_VERSION ?= 2.12.0
PROMETHEUS_VERSION ?= v3.15.1
GRAFANA_VERSION ?= 13.3.0
EXPLORER_VERSION ?= 2.0.0

# Must match `version:` in .github/workflows/golangci-lint.yml.
GOLANGCI_LINT_VERSION ?= v2.14.0

#########################
# Paths
#########################

# need to install fabric binaries outside of fsc tree for now (due to chaincode packaging issues)
FABRIC_BINARY_BASE ?= $(PWD)/../fabric
FAB_BINS ?= $(FABRIC_BINARY_BASE)/bin
# The fabric-x tools ship a configtxgen and a cryptogen of their own. Installing
# them into $(FAB_BINS) would overwrite fabric's, so they get their own
# subdirectory there and both toolchains can be installed at once. Keep in sync
# with fxconfig.BinSubdir.
FABRIC_X_BINS ?= $(FAB_BINS)/fabric-x

# Run `make help` to find the supported targets
.DEFAULT_GOAL := help

.PHONY: help
help: ## List all commands with documentation
	@echo "Available commands:"
	@awk 'BEGIN {FS = ":.*?## "}; /^[a-zA-Z0-9_-]+:.*?## / {printf "\033[36m%-25s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# lint, go fix, docs links and govulncheck
include checks.mk

#########################
# Tools
#########################

.PHONY: install-tools
install-tools: ## Install all tools
# Thanks for great inspiration https://marcofranssen.nl/manage-go-tools-via-go-modules
	@echo Installing tools from tools/tools.go
	@cd tools; cat tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -tI % go install %

.PHONY: install-linter-tool
install-linter-tool: ## Install linter in $(go env GOPATH)/bin
	@echo "Installing golangci Linter"
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/$(GOLANGCI_LINT_VERSION)/install.sh | sh -s -- -b $$(go env GOPATH)/bin $(GOLANGCI_LINT_VERSION)

.PHONY: install-fabric-bins
install-fabric-bins: ## Install fabric binaries in $(FABRIC_BINARY_BASE)
	./ci/scripts/download_fabric.sh $(FABRIC_BINARY_BASE) $(FABRIC_VERSION)
	@test -x $(FABRIC_BINARY_BASE)/builders/ccaas/bin/build || { \
	  echo "==> $(FABRIC_BINARY_BASE)/builders/ccaas is missing."; \
	  echo "    Fabric $(FABRIC_VERSION) ships it; remove the directory and re-run."; \
	  exit 1; \
	}

.PHONY: install-fabricx-tools
install-fabricx-tools: ## Install the fabric-x tools in $(FABRIC_X_BINS)
	@env GOBIN=$(FABRIC_X_BINS) go install $(addprefix github.com/hyperledger/fabric-x/tools/,$(addsuffix @$(FABRIC_X_TOOLS_VERSION),fxconfig configtxgen cryptogen))

.PHONY: install-softhsm
install-softhsm: ## Install softhsm
	./ci/scripts/install_softhsm.sh

.PHONY: install-fsccli
install-fsccli: ## Install fsccli
	cd integration; go install ./nwo/cmd/fsccli

#########################
# Code generation
#########################

.PHONY: generate-protos
generate-protos: ## Delete all protoc-generated files and regenerate via compile_proto.sh
	@./scripts/find-protos.sh > /dev/null
	@./scripts/find-protos.sh --delete
	./scripts/compile_proto.sh

# The latest release tag. A proto file that is new since then has nothing to compare against.
BUF_AGAINST ?= $(shell git describe --tags --abbrev=0 --match 'v*')

.PHONY: buf-breaking
buf-breaking: ## Check that the FSC protos (buf.yaml) only add to what $(BUF_AGAINST) has
	buf breaking --against '.git#tag=$(or $(BUF_AGAINST),$(error no v* tag reachable; git fetch --tags or set BUF_AGAINST))' --against-config buf.yaml

.PHONY: generate-mocks
generate-mocks: ## Delete all counterfeiter mock folders and regenerate via go generate ./...
	@./scripts/find-mocks.sh > /dev/null
	@./scripts/find-mocks.sh --delete
	go generate ./...

#########################
# Container images
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

explorer-db:
	docker pull ghcr.io/hyperledger-labs/explorer-db:$(EXPLORER_VERSION)

explorer:
	docker pull ghcr.io/hyperledger-labs/explorer:$(EXPLORER_VERSION)

prometheus:
	docker pull ghcr.io/prometheus/prometheus:$(PROMETHEUS_VERSION)

grafana:
	docker pull grafana/grafana:$(GRAFANA_VERSION)

jaeger:
	docker pull cr.jaegertracing.io/jaegertracing/jaeger:$(JAEGER_VERSION)

postgres:
	docker pull postgres:$(POSTGRES_VERSION)
	docker tag postgres:$(POSTGRES_VERSION) fsc.itests/postgres:latest

.PHONY: chaincode-images
chaincode-images: ## Build container images for the in-tree CCaaS chaincodes
	@grep -vE '^\s*(#|$$)' scripts/chaincode/images.txt | \
	  while read -r image module pkg; do \
	    scripts/chaincode/build-image.sh "$$image" "$$module" "$$pkg" || exit 1; \
	  done

.PHONY: clean-fabric-peer-images
clean-fabric-peer-images: ## Clean up generated fabric peer images
	docker images -a | grep "_peer_" | awk '{print $$3}' | xargs docker rmi

#########################
# Unit tests
#########################

# we use a multi-module repo structure here and therefore need to carefully collect packages for unit tests
GO_PACKAGES = $$(go list ./...)
# Unit-testable packages of the integration module: the nwo harness plus the shared
# helpers the suites import. The suites themselves are integration tests, not unit tests.
INTEGRATION_UNIT_PACKAGES = ./nwo/... ./fabric/common/...
# Extension modules: optional drivers with their own go.mod, invisible to the root
# `go list ./...`. Without stepping into them explicitly, the libp2p
# host-conformance tests (shared with the websocket host) never run.
EXTENSION_MODULES = platform/view/services/comm/host/libp2p
GO_PACKAGES_SDK = $$(go list ./... | grep '/sdk/dig$$')
GO_TEST_PARAMS ?= -race -cover
TEST_PKGS ?= $(GO_PACKAGES)
# The packages of TEST_PKGS with a test file that mentions Postgres. --run '(Postgres)'
# selects tests by top-level name, and test names live in _test.go files, so this set
# covers every package where the filter matches; unit-tests-postgres compiles only these.
POSTGRES_TEST_PKGS = $$(go list -f '{{.ImportPath}} {{.Dir}}' $(TEST_PKGS) | while read -r p d; do grep -qs Postgres "$$d"/*_test.go && echo "$$p"; done)

# Instrument every package under the root module so that coverage is credited
# even when a package is exercised only by tests living in another package
# (e.g. the exported helpers in .../sql/common used by the sqlite/postgres tests).
# Kept separate from GO_TEST_PARAMS because CI overrides GO_TEST_PARAMS wholesale.
GO_COVERPKG ?= -coverpkg=./...

# COVERAGE=1 leaves a filtered profile in COVERAGE_PROFILE, which has to be
# absolute: `go test -C` resolves it inside the module it tested. -count=1
# because a cached result replays its summary line but writes no profile data.
COVERAGE_PROFILE ?= $(CURDIR)/coverage.profile
ifdef COVERAGE
COVER_FLAGS = -count=1 -coverprofile=$(COVERAGE_PROFILE)
FILTER_COVERAGE = ./scripts/filter-coverage.sh $(COVERAGE_PROFILE) $(COVERAGE_PROFILE)
else
FILTER_COVERAGE = true
endif

UNIT_TEST_ENV = FABRIC_LOGGING_SPEC=error FAB_BINS=$(FAB_BINS)

.PHONY: unit-tests
unit-tests: ## Run unit tests in every module
	@$(MAKE) -k unit-tests-root unit-tests-integration unit-tests-extensions

.PHONY: unit-tests-root
unit-tests-root: ## Run unit tests of the root module
	@env $(UNIT_TEST_ENV) go test $(GO_TEST_PARAMS) $(GO_COVERPKG) $(COVER_FLAGS) --skip '(Postgres)' $(TEST_PKGS)
	@$(FILTER_COVERAGE)

.PHONY: unit-tests-integration
unit-tests-integration: ## Run unit tests of the integration module (nwo harness + shared helpers)
	@env $(UNIT_TEST_ENV) go test -C integration $(GO_TEST_PARAMS) $(COVER_FLAGS) $(INTEGRATION_UNIT_PACKAGES)
	@$(FILTER_COVERAGE)

.PHONY: unit-tests-extensions
unit-tests-extensions: ## Run unit tests of the extension modules (optional drivers with their own go.mod)
	@rc=0; raw=$$(mktemp -d); \
	for m in $(EXTENSION_MODULES); do \
		env $(UNIT_TEST_ENV) go test -C $$m $(GO_TEST_PARAMS) $(GO_COVERPKG) \
			$(if $(COVERAGE),-count=1 -coverprofile=$$raw/$$(echo $$m | tr / _)) ./... || rc=1; \
	done; \
	if [ -n "$(COVERAGE)" ] && [ $$rc -eq 0 ]; then \
		cat $$raw/* > $(COVERAGE_PROFILE) && $(FILTER_COVERAGE); \
	fi; \
	rm -rf $$raw; exit $$rc

.PHONY: unit-tests-postgres
unit-tests-postgres: ## Run unit tests for postgres (requires the images from pull-images-database)
	@env $(UNIT_TEST_ENV) go test $(GO_TEST_PARAMS) $(GO_COVERPKG) $(COVER_FLAGS) --run '(Postgres)' $(POSTGRES_TEST_PKGS)
	@$(FILTER_COVERAGE)

.PHONY: unit-tests-sdk
unit-tests-sdk: ## Run sdk wiring tests
	@env $(UNIT_TEST_ENV) go test $(GO_TEST_PARAMS) --run "(TestWiring)" $(GO_PACKAGES_SDK)

.PHONY: coverage-local
coverage-local: ## Run unit tests and show filtered coverage
	@$(MAKE) unit-tests-root COVERAGE=1 COVERAGE_PROFILE=$(CURDIR)/coverage.out
	@go tool cover -func=coverage.out | tail -n 1

#########################
# Integration tests
#########################

GINKGO_TEST_OPTS ?=
GINKGO_TEST_OPTS += --keep-going

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
INTEGRATION_TARGETS += fabric-faultrecovery
# fabric/faultrecovery runs its long Postgres outage specs, which set a shortened
# commit retry budget before their network starts, in a second target, so CI
# runs them in parallel with the rest of the suite.
INTEGRATION_TARGETS += fabric-faultrecovery-postgres-outage
# Its specs on replicated nodes need a topology of their own, so they also run in a
# target of their own.
INTEGRATION_TARGETS += fabric-faultrecovery-replication
# Its specs cut links silently through fault proxies in a network of their own.
INTEGRATION_TARGETS += fabric-faultrecovery-partition
# Its specs on SmartBFT orderers need a topology of their own.
INTEGRATION_TARGETS += fabric-faultrecovery-bft
# Its crash and catch-up specs, labelled crash, run on the main network in a target of
# their own, so neither target outlasts CI's per-job limit.
INTEGRATION_TARGETS += fabric-faultrecovery-crash
INTEGRATION_TARGETS += fabric-iou
# fabric/iou runs both comm types. They are split into two targets so CI runs
# them as parallel matrix entries rather than serially in one job.
INTEGRATION_TARGETS += fabric-iou-libp2p
# Needs softhsm (make install-softhsm) and the pkcs11 build tag, see INTEGRATION_GOFLAGS.
INTEGRATION_TARGETS += fabric-iouhsm
INTEGRATION_TARGETS += fabric-runtimeconfig
INTEGRATION_TARGETS += fabric-twonets
# A Fabric and a Fabric-x network on the same nodes; needs the Fabric-x binaries too.
INTEGRATION_TARGETS += fabric-mixednets

## fabricx section
INTEGRATION_TARGETS += fabricx-iou
INTEGRATION_TARGETS += fabricx-atsa
INTEGRATION_TARGETS += fabricx-simple
INTEGRATION_TARGETS += fabricx-deployment
INTEGRATION_TARGETS += fabricx-multiendorsement
INTEGRATION_TARGETS += fabricx-configupdate
INTEGRATION_TARGETS += fabricx-faultrecovery
# Its specs on replicated nodes run in a target of their own, as on Fabric.
INTEGRATION_TARGETS += fabricx-faultrecovery-replication
INTEGRATION_TARGETS += fabricx-faultrecovery-partition
# Its crash and catch-up specs run in a target of their own, as on Fabric.
INTEGRATION_TARGETS += fabricx-faultrecovery-crash
# Its specs on a network where Org2 runs two committers run in a target of their own.
INTEGRATION_TARGETS += fabricx-faultrecovery-committer-failover

# Integration targets that CI runs on a schedule, not on every pull request (see
# .github/workflows/mixedversion.yml): `make list-integration-tests`, which feeds
# the pull request matrix, leaves them out.
NIGHTLY_INTEGRATION_TARGETS = fabric-mixedversion

# fabric-mixedversion runs nodes of FSC_PREVIOUS_RELEASE: a module version, or the
# absolute path of a checkout. It defaults to the latest release tag other than one
# at HEAD itself. See docs/dev/integration-tests/authoring.md.
integration-tests-fabric-mixedversion: export FSC_PREVIOUS_RELEASE ?= $(shell git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname --no-contains HEAD 2>/dev/null | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$$' | head -n1)

# Targets are normally named <platform>-<suite> and map onto
# ./integration/<platform>/<suite>. Targets that run one suite under a
# different name need an explicit directory.
INTEGRATION_DIR_fabric-iou-libp2p = fabric/iou
INTEGRATION_DIR_fabric-faultrecovery-postgres-outage = fabric/faultrecovery
INTEGRATION_DIR_fabric-faultrecovery-replication = fabric/faultrecovery
INTEGRATION_DIR_fabric-faultrecovery-partition = fabric/faultrecovery
INTEGRATION_DIR_fabric-faultrecovery-bft = fabric/faultrecovery
INTEGRATION_DIR_fabric-faultrecovery-crash = fabric/faultrecovery
INTEGRATION_DIR_fabricx-faultrecovery-replication = fabricx/faultrecovery
INTEGRATION_DIR_fabricx-faultrecovery-partition = fabricx/faultrecovery
INTEGRATION_DIR_fabricx-faultrecovery-crash = fabricx/faultrecovery
INTEGRATION_DIR_fabricx-faultrecovery-committer-failover = fabricx/faultrecovery

# Per-target ginkgo flags. A suite split into several targets selects each
# target's specs by label. The suite's default target uses a negative filter
# so a spec added later without the other targets' labels still runs there,
# rather than silently running in neither job.
INTEGRATION_FLAGS_fabric-iou        = --label-filter='!libp2p'
INTEGRATION_FLAGS_fabric-iou-libp2p = --label-filter=libp2p
INTEGRATION_FLAGS_fabric-faultrecovery                 = --label-filter='!postgres-outage && !replication && !partition && !bft && !crash'
INTEGRATION_FLAGS_fabric-faultrecovery-postgres-outage = --label-filter=postgres-outage
INTEGRATION_FLAGS_fabric-faultrecovery-replication     = --label-filter=replication
INTEGRATION_FLAGS_fabric-faultrecovery-partition       = --label-filter=partition
INTEGRATION_FLAGS_fabric-faultrecovery-bft             = --label-filter=bft
INTEGRATION_FLAGS_fabric-faultrecovery-crash           = --label-filter=crash
INTEGRATION_FLAGS_fabricx-faultrecovery                    = --label-filter='!replication && !partition && !crash && !committer-failover'
INTEGRATION_FLAGS_fabricx-faultrecovery-replication        = --label-filter=replication
INTEGRATION_FLAGS_fabricx-faultrecovery-partition          = --label-filter=partition
INTEGRATION_FLAGS_fabricx-faultrecovery-crash              = --label-filter=crash
INTEGRATION_FLAGS_fabricx-faultrecovery-committer-failover = --label-filter=committer-failover

# Per-target GOFLAGS for the test binary. Every file of fabric/iouhsm is built
# with the pkcs11 tag; without it the suite compiles to no specs and passes.
INTEGRATION_GOFLAGS_fabric-iouhsm = -tags=pkcs11

integration_default_dir = $(firstword $(subst -, ,$(1)))/$(subst $(firstword $(subst -, ,$(1)))-,,$(1))
integration_dir = $(or $(INTEGRATION_DIR_$(1)),$(call integration_default_dir,$(1)))

.PHONY: list-integration-tests
list-integration-tests: ## List all integration tests
	@$(foreach t,$(INTEGRATION_TARGETS),echo "$(t)";)

.PHONY: integration-tests
integration-tests: $(addprefix integration-tests-,$(INTEGRATION_TARGETS)) ## Run all integration tests

$(addprefix integration-tests-,$(INTEGRATION_TARGETS) $(NIGHTLY_INTEGRATION_TARGETS)) : integration-tests-%:
	export FAB_BINS=$(FAB_BINS); \
		cd ./integration/$(call integration_dir,$*); \
		$(if $(INTEGRATION_GOFLAGS_$*),GOFLAGS="$(INTEGRATION_GOFLAGS_$*)" )ginkgo $(GINKGO_TEST_OPTS) $(INTEGRATION_FLAGS_$*) .

#########################
# Release
#########################

.PHONY: tag-release
tag-release: ## Create git tags for all modules at HEAD. Usage: make tag-release VERSION=v0.13.0 [DRY=1] [PUSH=<remote>]
ifndef VERSION
	$(error VERSION is required. Usage: make tag-release VERSION=v0.13.0)
endif
	./scripts/tag-release.sh $(if $(DRY),--dry) $(if $(PUSH),--push=$(PUSH)) $(VERSION)

# The release steps of docs/dev/releasing.md. Each asks before it changes anything outside
# the checkout. VERSION, DRY_RUN, RELEASE_REPO, REMOTE and YES reach the script from the make
# command line or the environment; scripts/release.sh describes them.
.PHONY: release-prepare release-pr release-publish
release-prepare: ## Generate the release files of the next minor on release/vX.Y.Z, staged
	@./scripts/release.sh prepare
release-pr: ## Commit the release files, push the branch and open the release PR
	@./scripts/release.sh pr
release-publish: ## Tag the merged release commit, push the tags and create the GitHub release
	@./scripts/release.sh publish

#########################
# Housekeeping
#########################

.PHONY: tidy
tidy: ## Run go mod tidy everywhere
	@./scripts/gomate.sh tidy

.PHONY: clean
clean: $(addprefix clean-,$(INTEGRATION_TARGETS) $(NIGHTLY_INTEGRATION_TARGETS)) ## Clean generated testdata
	rm -rf ./integration/nwo/cmd/fsccli/out
	rm -rf ./out

$(addprefix clean-,$(INTEGRATION_TARGETS) $(NIGHTLY_INTEGRATION_TARGETS)) : clean-%:
	rm -rf ./integration/$(call integration_dir,$*)/out
