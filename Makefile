# pinned versions
FABRIC_VERSION=2.2

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

# include the checks target
include $(TOP)/checks.mk

.PHONY: unit-tests
unit-tests:
	@go test -cover $(shell go list ./... | grep -v '/integration/')
	cd integration/nwo/; go test -cover ./...

.PHONY: install-softhsm

install-softhsm:
	./ci/scripts/install_softhsm.sh

.PHONY: unit-tests-race
unit-tests-race:
	@export GORACE=history_size=7; go test -race -cover $(shell go list ./... | grep -v '/integration/')
	cd integration/nwo/; go test -cover ./...

.PHONY: docker-images
docker-images: fabric-docker-images weaver-docker-images fpc-docker-images orion-server-images monitoring-docker-images

.PHONY: fabric-docker-images
fabric-docker-images:
	docker pull hyperledger/fabric-baseos:$(FABRIC_VERSION)
	docker image tag hyperledger/fabric-baseos:$(FABRIC_VERSION) hyperledger/fabric-baseos:latest
	docker pull hyperledger/fabric-ccenv:$(FABRIC_VERSION)
	docker image tag hyperledger/fabric-ccenv:$(FABRIC_VERSION) hyperledger/fabric-ccenv:latest

.PHONY: weaver-docker-images
weaver-docker-images:
	docker pull ghcr.io/hyperledger-labs/weaver-fabric-driver:1.2.1
	docker image tag ghcr.io/hyperledger-labs/weaver-fabric-driver:1.2.1 hyperledger-labs/weaver-fabric-driver:latest
	docker pull ghcr.io/hyperledger-labs/weaver-relay-server:1.2.1
	docker image tag ghcr.io/hyperledger-labs/weaver-relay-server:1.2.1 hyperledger-labs/weaver-relay-server:latest

.PHONY: fpc-docker-images
fpc-docker-images:
	docker pull ghcr.io/mbrandenburger/fpc/ercc:main
	docker image tag ghcr.io/mbrandenburger/fpc/ercc:main fpc/ercc:latest
	docker pull ghcr.io/mbrandenburger/fpc/fpc-echo:main
	docker image tag ghcr.io/mbrandenburger/fpc/fpc-echo:main fpc/fpc-echo:latest

.PHONY: monitoring-docker-images
monitoring-docker-images:
	docker pull hyperledger/explorer-db:latest
	docker pull hyperledger/explorer:latest
	docker pull prom/prometheus:latest
	docker pull grafana/grafana:latest

.PHONY: orion-server-images
orion-server-images:
	docker pull orionbcdb/orion-server:v0.2.5
	docker image tag orionbcdb/orion-server:v0.2.5 orionbcdb/orion-server:latest

.PHONY: integration-tests
integration-tests:
	cd ./integration/fabric/iou; ginkgo $(GINKGO_TEST_OPTS) .
	cd ./integration/fabric/atsa/chaincode; ginkgo $(GINKGO_TEST_OPTS) .
	cd ./integration/fabric/atsa/fsc; ginkgo $(GINKGO_TEST_OPTS) .
	cd ./integration/fabric/twonets; ginkgo $(GINKGO_TEST_OPTS) .
	cd ./integration/fsc/pingpong/; ginkgo $(GINKGO_TEST_OPTS) .
	cd ./integration/fsc/stoprestart; ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: integration-tests-iou
integration-tests-iou:
	cd ./integration/fabric/iou; ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: integration-tests-iou-hsm
integration-tests-iou-hsm:
	@echo "Setup SoftHSM"
	@./ci/scripts/setup_softhsm.sh
	@echo "Start Integration Test"
	cd ./integration/fabric/iouhsm; ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: integration-tests-atsacc
integration-tests-atsacc:
	cd ./integration/fabric/atsa/chaincode; ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: integration-tests-atsafsc
integration-tests-atsafsc:
	cd ./integration/fabric/atsa/fsc; ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: integration-tests-twonets
integration-tests-twonets:
	cd ./integration/fabric/twonets; ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: integration-tests-fpc-echo
integration-tests-fpc-echo:
	cd ./integration/fabric/fpc/echo; ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: integration-tests-weaver-relay
integration-tests-weaver-relay:
	cd ./integration/fabric/weaver/relay; ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: integration-tests-fabric-stoprestart
integration-tests-fabric-stoprestart:
	cd ./integration/fabric/stoprestart; ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: integration-tests-pingpong
integration-tests-pingpong:
	cd ./integration/fsc/pingpong/; ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: integration-tests-stoprestart
integration-tests-stoprestart:
	cd ./integration/fsc/stoprestart; ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: integration-tests-orioncars
integration-tests-orioncars:
	cd ./integration/orion/cars; ginkgo $(GINKGO_TEST_OPTS) .

.PHONY: tidy
tidy:
	@go mod tidy -compat=1.17

.PHONY: clean
clean:
	docker network prune -f
	docker container prune -f
	rm -rf ./build
	rm -rf ./testdata
	rm -rf ./integration/fabric/atsa/chaincode/cmd
	rm -rf ./integration/fabric/atsa/fsc/cmd
	rm -rf ./integration/fabric/iou/cmd/
	rm -rf ./integration/fabric/iou/testdata/
	rm -rf ./integration/fabric/twonets/cmd
	rm -rf ./integration/fabric/weaver/relay/cmd
	rm -rf ./integration/fabric/fpc/echo/cmd
	rm -rf ./integration/fabric/stoprestart/cmd
	rm -rf ./integration/fsc/stoprestart/cmd
	rm -rf ./integration/orion/cars/cmd
	rm -rf ./integration/fscnodes
	rm -rf ./cmd/fsccli/cmd
	rm -rf ./samples/fabric/iou/cmd

.PHONY: fsccli
fsccli:
	@go install ./cmd/fsccli