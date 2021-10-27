module github.com/hyperledger-labs/fabric-smart-client

go 1.16

replace (
	github.com/fsouza/go-dockerclient => github.com/fsouza/go-dockerclient v1.4.1
	github.com/go-kit/kit => github.com/go-kit/kit v0.7.0
	github.com/hyperledger/fabric => github.com/hyperledger/fabric v1.4.0-rc1.0.20210722174351-9815a7a8f0f7
	github.com/hyperledger/fabric-protos-go => github.com/hyperledger/fabric-protos-go v0.0.0-20201028172056-a3136dde2354
	go.etcd.io/etcd => go.etcd.io/etcd v0.5.0-alpha.5.0.20181228115726-23731bf9ba55
)

require (
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/IBM/idemix v0.0.0-20220113150823-80dd4cb2d74e
	github.com/IBM/mathlib v0.0.0-20220112091634-0a7378db6912
	github.com/OneOfOne/xxhash v1.2.5 // indirect
	github.com/ReneKroon/ttlcache/v2 v2.11.0
	github.com/Shopify/sarama v1.26.3 // indirect
	github.com/armon/go-metrics v0.0.0-20180917152333-f0300d1749da
	github.com/btcsuite/btcd v0.21.0-beta // indirect
	github.com/containerd/containerd v1.5.2 // indirect
	github.com/dgraph-io/badger/v3 v3.2103.2
	github.com/docker/docker v20.10.7+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/fsouza/go-dockerclient v1.6.1
	github.com/gogo/protobuf v1.3.2
	github.com/golang/protobuf v1.5.2
	github.com/google/go-cmp v0.5.5
	github.com/gorilla/mux v1.8.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.2.0
	github.com/hyperledger-labs/orion-sdk-go v0.0.0-20211031122748-71e8c92308b2
	github.com/hyperledger-labs/orion-server v0.1.1-0.20211020140144-04f19b212c84
	github.com/hyperledger-labs/weaver-dlt-interoperability/common/protos-go v1.2.3-alpha.1
	github.com/hyperledger-labs/weaver-dlt-interoperability/sdks/fabric/go-sdk v1.2.3-alpha.1.0.20210812140206-37f430515b8c
	github.com/hyperledger/fabric v1.4.0-rc1.0.20220128025611-fad7f691a967
	github.com/hyperledger/fabric-chaincode-go v0.0.0-20210718160520-38d29fabecb9
	github.com/hyperledger/fabric-contract-api-go v1.1.1
	github.com/hyperledger/fabric-private-chaincode v0.0.0-20210907122433-d56466264e4d
	github.com/hyperledger/fabric-protos-go v0.0.0-20210911123859-041d13f0980c
	github.com/jackpal/go-nat-pmp v1.0.2 // indirect
	github.com/libp2p/go-libp2p v0.5.2
	github.com/libp2p/go-libp2p-core v0.3.0
	github.com/libp2p/go-libp2p-discovery v0.2.0
	github.com/libp2p/go-libp2p-kad-dht v0.5.0
	github.com/mattn/go-colorable v0.1.2 // indirect
	github.com/mattn/go-isatty v0.0.12 // indirect
	github.com/miracl/conflate v1.2.1
	github.com/mitchellh/mapstructure v1.3.2
	github.com/moby/sys/mount v0.2.0 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/multiformats/go-multiaddr v0.2.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.16.0
	github.com/op/go-logging v0.0.0-20160315200505-970db520ece7
	github.com/otiai10/copy v1.5.1
	github.com/pierrec/lz4 v2.5.2+incompatible // indirect
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.1.1
	github.com/spf13/viper v1.7.0
	github.com/stretchr/testify v1.7.1-0.20210116013205-6990a05d54c2
	github.com/sykesm/zap-logfmt v0.0.4
	github.com/tedsuo/ifrit v0.0.0-20191009134036-9a97d0632f00
	github.com/test-go/testify v1.1.4
	go.uber.org/atomic v1.7.0
	go.uber.org/zap v1.18.1
	golang.org/x/net v0.0.0-20211015210444-4f30a5c0130f
	golang.org/x/sys v0.0.0-20220209214540-3681064d5158 // indirect
	golang.org/x/tools v0.1.9 // indirect
	google.golang.org/genproto v0.0.0-20210201184850-646a494a81ea // indirect
	google.golang.org/grpc v1.39.1
	google.golang.org/protobuf v1.27.1
	gopkg.in/src-d/go-git.v4 v4.13.1
	gopkg.in/yaml.v2 v2.4.0
)
