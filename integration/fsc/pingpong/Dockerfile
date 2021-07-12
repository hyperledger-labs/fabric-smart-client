FROM golang:1.14.15

WORKDIR /go/src/github.com/hyperledger-labs/
RUN git clone https://github.com/hyperledger-labs/fabric-smart-client.git
WORKDIR ./fabric-smart-client/integration/fsc/pingpong
RUN go test

