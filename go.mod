module github.com/hyperledger-labs/fabric-smart-client

go 1.14

replace (
	github.com/fsouza/go-dockerclient => github.com/fsouza/go-dockerclient v1.4.1
	github.com/go-kit/kit => github.com/go-kit/kit v0.7.0
	github.com/golang/protobuf => github.com/golang/protobuf v1.3.3
	github.com/spf13/viper => github.com/spf13/viper v0.0.0-20150908122457-1967d93db724
	go.etcd.io/etcd => go.etcd.io/etcd v0.5.0-alpha.5.0.20181228115726-23731bf9ba55
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55
	google.golang.org/grpc => google.golang.org/grpc v1.29.1
)

require (
	github.com/OneOfOne/xxhash v1.2.5 // indirect
	github.com/Shopify/sarama v1.26.3
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20190717042225-c3de453c63f4 // indirect
	github.com/btcsuite/btcd v0.21.0-beta // indirect
	github.com/client9/misspell v0.3.4 // indirect
	github.com/dgraph-io/badger/v3 v3.2011.1
	github.com/fsouza/go-dockerclient v1.6.1
	github.com/gogo/protobuf v1.3.1
	github.com/golang/protobuf v1.4.3
	github.com/golang/snappy v0.0.3-0.20201103224600-674baa8c7fc3 // indirect
	github.com/google/addlicense v0.0.0-20210428195630-6d92264d7170 // indirect
	github.com/google/go-cmp v0.5.0
	github.com/google/uuid v1.2.0 // indirect
	github.com/gordonklaus/ineffassign v0.0.0-20210522101830-0589229737b2 // indirect
	github.com/gorilla/mux v1.8.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.2.0
	github.com/hashicorp/go-version v1.2.0
	github.com/hyperledger/fabric v1.4.0-rc1.0.20200930182727-344fda602252
	github.com/hyperledger/fabric-amcl v0.0.0-20200424173818-327c9e2cf77a
	github.com/hyperledger/fabric-chaincode-go v0.0.0-20200424173110-d7076418f212
	github.com/hyperledger/fabric-contract-api-go v1.1.1
	github.com/hyperledger/fabric-protos-go v0.0.0-20200506201313-25f6564b9ac4
	github.com/jackpal/go-nat-pmp v1.0.2 // indirect
	github.com/klauspost/compress v1.10.1 // indirect
	github.com/libp2p/go-libp2p v0.5.2
	github.com/libp2p/go-libp2p-core v0.3.0
	github.com/libp2p/go-libp2p-discovery v0.2.0
	github.com/libp2p/go-libp2p-kad-dht v0.5.0
	github.com/mattn/go-colorable v0.1.2 // indirect
	github.com/mattn/go-isatty v0.0.12 // indirect
	github.com/mitchellh/mapstructure v1.2.2
	github.com/multiformats/go-multiaddr v0.2.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.10.1
	github.com/otiai10/copy v1.5.1
	github.com/pierrec/lz4 v2.5.2+incompatible // indirect
	github.com/pkg/errors v0.9.1
	github.com/prometheus/common v0.6.0
	github.com/spf13/cobra v1.1.1
	github.com/spf13/viper v1.7.0
	github.com/stretchr/testify v1.7.0
	github.com/sykesm/zap-logfmt v0.0.4
	github.com/syndtr/goleveldb v1.0.1-0.20200815110645-5c35d600f0ca // indirect
	github.com/tedsuo/ifrit v0.0.0-20191009134036-9a97d0632f00
	github.com/test-go/testify v1.1.4
	github.com/willf/bitset v1.1.11 // indirect
	go.uber.org/atomic v1.7.0
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.16.0
	golang.org/x/crypto v0.0.0-20201221181555-eec23a3978ad // indirect
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c // indirect
	golang.org/x/text v0.3.5 // indirect
	golang.org/x/tools v0.1.4 // indirect
	google.golang.org/genproto v0.0.0-20210201184850-646a494a81ea // indirect
	google.golang.org/grpc v1.35.0
	google.golang.org/protobuf v1.25.0
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/src-d/go-git.v4 v4.13.1
	gopkg.in/yaml.v2 v2.3.0
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)
