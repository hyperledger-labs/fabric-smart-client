module github.com/hyperledger-labs/fabric-smart-client/examples

go 1.27.4

replace (
	github.com/hyperledger-labs/fabric-smart-client => ../
	github.com/hyperledger-labs/fabric-smart-client/integration => ../integration
	github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state/cc/query => ../platform/fabric/services/state/cc/query
	github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/libp2p => ../platform/view/services/comm/host/libp2p
)
