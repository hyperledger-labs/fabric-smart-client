# pinned versions
FABRIC_VERSION=2.4.4

.PHONY: checks
checks: dependencies
	@test -z $(shell gofmt -l -s $(shell go list -f '{{.Dir}}' ./...) | tee /dev/stderr) || (echo "Fix formatting issues"; exit 1)
	find . -name '*.go' | xargs addlicense -check || (echo "Missing license headers"; exit 1)
	@go vet -all $(shell go list -f '{{.Dir}}' ./...)
	@ineffassign $(shell go list -f '{{.Dir}}' ./...)
	@misspell $(shell go list -f '{{.Dir}}' ./...)

.PHONY: lint
lint:
	@golint $(shell go list -f '{{.Dir}}' ./...)

.PHONY: gocyclo
gocyclo:
	@gocyclo -over 15 $(shell go list -f '{{.Dir}}' ./...)

.PHONY: ineffassign
ineffassign:
	@ineffassign $(shell go list -f '{{.Dir}}' ./...)

.PHONY: misspell
misspell:
	@misspell $(shell go list -f '{{.Dir}}' ./...)

.PHONY: unit-tests
unit-tests: docker-images
	@go test -cover $(shell go list ./... | grep -v '/integration/')
	cd integration/nwo/; go test -cover ./...

.PHONY: unit-tests-race
unit-tests-race: docker-images
	@export GORACE=history_size=7; go test -race -cover $(shell go list ./... | grep -v '/integration/')
	cd integration/nwo/; go test -cover ./...

.PHONY: docker-images
docker-images: fabric-docker-images weaver-docker-images fpc-docker-images monitoring-docker-images

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
	docker pull orionbcdb/orion-server:latest

.PHONY: dependencies
dependencies:
	go install github.com/onsi/ginkgo/ginkgo
	go get github.com/gordonklaus/ineffassign
	go get github.com/google/addlicense
	go get github.com/client9/misspell/cmd/misspell

.PHONY: integration-tests
integration-tests: docker-images dependencies
	cd ./integration/fabric/iou; ginkgo -keepGoing --slowSpecThreshold 60 .
	cd ./integration/fabric/atsa/chaincode; ginkgo -keepGoing --slowSpecThreshold 60 .
	cd ./integration/fabric/atsa/fsc; ginkgo -keepGoing --slowSpecThreshold 60 .
	cd ./integration/fabric/twonets; ginkgo -keepGoing --slowSpecThreshold 60 .
	cd ./integration/fsc/pingpong/; ginkgo -keepGoing --slowSpecThreshold 60 .
	cd ./integration/fsc/stoprestart; ginkgo -keepGoing --slowSpecThreshold 60 .

.PHONY: integration-tests-iou
integration-tests-iou: docker-images dependencies
	cd ./integration/fabric/iou; ginkgo -keepGoing --slowSpecThreshold 60 .

.PHONY: integration-tests-atsacc
integration-tests-atsacc: docker-images dependencies
	cd ./integration/fabric/atsa/chaincode; ginkgo -keepGoing --slowSpecThreshold 60 .

.PHONY: integration-tests-atsafsc
integration-tests-atsafsc: docker-images dependencies
	cd ./integration/fabric/atsa/fsc; ginkgo -keepGoing --slowSpecThreshold 60 .

.PHONY: integration-tests-twonets
integration-tests-twonets: docker-images dependencies
	cd ./integration/fabric/twonets; ginkgo -keepGoing --slowSpecThreshold 60 .

.PHONY: integration-tests-fpc-echo
integration-tests-fpc-echo: docker-images fpc-docker-images dependencies
	cd ./integration/fabric/fpc/echo; ginkgo -keepGoing --slowSpecThreshold 60 .

.PHONY: integration-tests-weaver-relay
integration-tests-weaver-relay: docker-images weaver-docker-images dependencies
	cd ./integration/fabric/weaver/relay; ginkgo -keepGoing --slowSpecThreshold 60 .

.PHONY: integration-tests-fabric-stoprestart
integration-tests-fabric-stoprestart: docker-images dependencies
	cd ./integration/fabric/stoprestart; ginkgo -keepGoing --slowSpecThreshold 60 .

.PHONY: integration-tests-pingpong
integration-tests-pingpong: docker-images dependencies
	cd ./integration/fsc/pingpong/; ginkgo -keepGoing --slowSpecThreshold 60 .

.PHONY: integration-tests-stoprestart
integration-tests-stoprestart: docker-images dependencies
	cd ./integration/fsc/stoprestart; ginkgo -keepGoing --slowSpecThreshold 60 .

.PHONY: integration-tests-orioncars
integration-tests-orioncars: docker-images orion-server-images dependencies
	cd ./integration/orion/cars; ginkgo -keepGoing --slowSpecThreshold 60 .

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
	rm -rf ./integration/fsc/pingpong/cmd/responder
	rm -rf ./integration/orion/cars/cmd
	rm -rf ./integration/fscnodes
	rm -rf ./cmd/fsccli/cmd
	rm -rf ./samples/fabric/iou/cmd

.PHONY: fsccli
fsccli:
	@go install ./cmd/fsccli