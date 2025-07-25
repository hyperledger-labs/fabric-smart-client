# pinned versions
FABRIC_VERSION ?= 3.1.1
FABRIC_TWO_DIGIT_VERSION = $(shell echo $(FABRIC_VERSION) | cut -d '.' -f 1,2)

# need to install fabric binaries outside of fsc tree for now (due to chaincode packaging issues)
FABRIC_BINARY_BASE=$(PWD)/../fabric
FAB_BINS ?= $(FABRIC_BINARY_BASE)/bin

# integration test options
GINKGO_TEST_OPTS ?=
GINKGO_TEST_OPTS += --keep-going
GINKGO_TEST_OPTS += --slow-spec-threshold=60s

TOP = .

all: install-tools install-softhsm checks unit-tests #integration-tests

.PHONY: install-tools
install-tools:
# Thanks for great inspiration https://marcofranssen.nl/manage-go-tools-via-go-modules
	@echo Installing tools from tools/tools.go
	@cd tools; cat tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -tI % go install %

.PHONY: download-fabric
download-fabric:
	./ci/scripts/download_fabric.sh $(FABRIC_BINARY_BASE) $(FABRIC_VERSION)

# include the checks target
include $(TOP)/checks.mk

.PHONY: unit-tests
unit-tests: testing-docker-images
	@export FAB_BINS=$(FAB_BINS); go test -cover $(shell go list ./... | grep -v '/integration/')
	cd integration/nwo/; go test -cover ./...

.PHONY: unit-tests-dig
unit-tests-dig:
	cd platform/view/sdk/dig; go test -cover ./...
	cd platform/fabric/sdk/dig; go test -cover ./...

.PHONY: install-softhsm
install-softhsm:
	./ci/scripts/install_softhsm.sh

run-optl:
	cd platform/view/services/tracing; docker-compose up -d 
	
 	
.PHONY: unit-tests-race
unit-tests-race: testing-docker-images
	@export GORACE=history_size=7; export FAB_BINS=$(FAB_BINS); go test -race -cover $(shell go list ./... | grep -v '/integration/')
	cd integration/nwo/; export FAB_BINS=$(FAB_BINS); go test -race -cover ./...

.PHONY: docker-images
docker-images: fabric-docker-images monitoring-docker-images testing-docker-images

.PHONY: fabric-docker-images
fabric-docker-images:
	docker pull hyperledger/fabric-baseos:$(FABRIC_TWO_DIGIT_VERSION)
	docker image tag hyperledger/fabric-baseos:$(FABRIC_TWO_DIGIT_VERSION) hyperledger/fabric-baseos:latest
	docker pull hyperledger/fabric-ccenv:$(FABRIC_TWO_DIGIT_VERSION)
	docker image tag hyperledger/fabric-ccenv:$(FABRIC_TWO_DIGIT_VERSION) hyperledger/fabric-ccenv:latest

.PHONY: monitoring-docker-images
monitoring-docker-images:
	docker pull ghcr.io/hyperledger-labs/explorer-db:latest
	docker pull ghcr.io/hyperledger-labs/explorer:latest
	docker pull prom/prometheus:latest
	docker pull grafana/grafana:latest
	docker pull jaegertracing/all-in-one:latest

.PHONY: testing-docker-images
testing-docker-images:
	docker pull postgres:16.2-alpine
	docker tag postgres:16.2-alpine fsc.itests/postgres:latest

INTEGRATION_TARGETS = integration-tests-iou
INTEGRATION_TARGETS += integration-tests-atsacc
INTEGRATION_TARGETS += integration-tests-chaincode-events
INTEGRATION_TARGETS += integration-tests-atsafsc
INTEGRATION_TARGETS += integration-tests-twonets
INTEGRATION_TARGETS += integration-tests-pingpong
INTEGRATION_TARGETS += integration-tests-stoprestart

.PHONY: integration-tests
integration-tests: $(INTEGRATION_TARGETS)

.PHONY: integration-tests-iou
integration-tests-iou:
	cd ./integration/fabric/iou; export FAB_BINS=$(FAB_BINS); ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: integration-tests-iou-hsm
integration-tests-iou-hsm:
	@echo "Setup SoftHSM"
	@./ci/scripts/setup_softhsm.sh
	@echo "Start Integration Test"
	cd ./integration/fabric/iouhsm; export FAB_BINS=$(FAB_BINS); ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: integration-tests-atsacc
integration-tests-atsacc:
	cd ./integration/fabric/atsa/chaincode; export FAB_BINS=$(FAB_BINS); ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: integration-tests-chaincode-events
integration-tests-chaincode-events:
	cd ./integration/fabric/events/chaincode; export FAB_BINS=$(FAB_BINS); ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: integration-tests-atsafsc
integration-tests-atsafsc:
	cd ./integration/fabric/atsa/fsc; export FAB_BINS=$(FAB_BINS); ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: integration-tests-twonets
integration-tests-twonets:
	cd ./integration/fabric/twonets; export FAB_BINS=$(FAB_BINS); ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: integration-tests-fabric-stoprestart
integration-tests-fabric-stoprestart:
	cd ./integration/fabric/stoprestart; export FAB_BINS=$(FAB_BINS); ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: integration-tests-pingpong
integration-tests-pingpong:
	cd ./integration/fsc/pingpong/; export FAB_BINS=$(FAB_BINS); ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: integration-tests-stoprestart
integration-tests-stoprestart:
	cd ./integration/fsc/stoprestart; export FAB_BINS=$(FAB_BINS); ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: tidy
tidy:
	@go mod tidy
	cd tools; go mod tidy
	cd platform/fabric/services/state/cc/query; go mod tidy

.PHONY: clean
clean:
	docker network prune -f
	docker container prune -f
	rm -rf ./build
	rm -rf ./testdata
	rm -rf ./integration/fabric/atsa/chaincode/cmd
	rm -rf ./integration/fabric/events/chaincode/cmd
	rm -rf ./integration/fabric/atsa/fsc/cmd
	rm -rf ./integration/fabric/iou/cmd/
	rm -rf ./integration/fabric/iou/testdata/
	rm -rf ./integration/fabric/twonets/cmd
	rm -rf ./integration/fabric/stoprestart/cmd
	rm -rf ./integration/fsc/stoprestart/cmd
	rm -rf ./integration/fscnodes
	rm -rf ./cmd/fsccli/cmd

.PHONY: clean-fabric-peer-images
clean-fabric-peer-images:
	docker images -a | grep "_peer_" | awk '{print $3}' | xargs docker rmi

.PHONY: fsccli
fsccli:
	@go install ./cmd/fsccli