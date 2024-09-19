# Example core.yaml file for Fabric

The following example provides descriptions for the various keys required for a Fabric Smart Client node that uses the Fabric SDK.

```yaml
---
# ------------------- Logging section ---------------------------
logging:
  # format is same as fabric [<logger>[,<logger>...]=]<level>[:[<logger>[,<logger>...]=]<level>...]
  format: '%{color}%{time:15:04:05.000} [%{module}] %{shortfunc} %{level:.4s}%{color:reset} %{message}'
  spec: debug

# ------------------- FSC Node Configuration -------------------------
fsc:
  # The FSC id provides a name for this node instance within the fsc network and is used when
  # naming docker resources for nwo testing as well as the unique id for tracing.
  # It's also used as the unique name to resolve this node's identity and grpc server endpoint
  id: someid

  # This is the identity of the node
  identity:
    cert:
      file: /path/to/cert.pem
    key:
      file: /path/to/key.pwm

  # This is used to list the authorized clients of this FSC node.
  # At least one client certificate must be specified
  # The GRPC service uses this list to filter the connecting clients
  client:
    certs:
    - path/to/client-cert.pem

  # ------------------- GRPC Server Configuration -------------------------
  grpc:
    enabled: true
    # The listen address of this server
    address: 0.0.0.0:20000
    # ConnectionTimeout specifies the timeout for connection establishment for all new connections
    # If not specified or set to <=0 then it will default to 5 seconds
    connectionTimeout: 10s

    tls:
      # Whether TLS is enabled or not
      enabled: true
      # Whether clients are required to provide their TLS certificates for verification
      clientAuthRequired: false
      # TLS Certificate
      cert:
        file: /path/to/tls/server.crt
      # TLS Key
      key:
        file: /path/to/tls/server.key

      # Root certificates to be able to verify client TLS certificates, only required
      # if clientAuthRequired is set to true
      clientRootCAs:
        files:
        - /path/to/client/tls/ca.crt

    # GRPC Server keepalive parameters
    keepalive:
      # minInterval is the minimum permitted time between client pings.
      # If clients send pings more frequently, the peer server will
      # disconnect them
      # If not specified, default is 60 seconds
      minInterval: 60s
      # interval is the duration after which if the server does not see
      # any activity from the client it pings the client to see if it's alive
      # If not specified, default is 2 hours
      interval: 300s
      # Timeout is the duration the server waits for a response
      # from the client after sending a ping before closing the connection
      # If not specified, default is 20 seconds
      timeout: 600s

  # ------------------- P2P Configuration -------------------------
  p2p:
    # Type of p2p communication. Currently supported: libp2p (default), rest
    type: libp2p
    # listen address see https://github.com/libp2p/specs/blob/master/addressing/README.md
    # for information on the format
    listenAddress: /dns4/myhostname/tcp/20001
    opts:
      # Only needed when type == libp2p
      # bootstrap node
      # if it's empty then this node is the bootstrap node, otherwise it's the name
      # of the bootstrap node, which must be defined in the FSC endpoint resolvers section
      # and that entry must have an address with an entry P2P.
      bootstrapNode: theBootstrapNode
      # Only needed when type == rest
      # Defines how to instantiate a router, i.e. the component that maps a service name (e.g. alice) to one or more IPs
      routing:
        # The path to the file that contains the routing, if the routing is static
        path: /path/to/routing-config.yaml

  # ------------------- KVS Configuration -------------------------
  # Internal key/value store used by the node to store information
  # such as bindings (eg resolvers)
  kvs:
    cache:
      # Sets the maximum number of cached items
      # If not specified, default is 100 (TBD: What is the scale here ?, what does 0 mean)
      size:
    persistence:
      # type can be badger (disk) or memory
      type: badger
      opts:
        # persistence location
        path: /some/path

  # ------------------- Web Server Configuration -------------------------
  # Web server must be enabled to support healthz, version and prometheus /metrics
  # end points.
  web:
    enabled: true
    address: 0.0.0.0:20002
    tls:
      # If tls is enabled then all clients must use mutualTLS
      enabled:  true
      cert:
        file: /path/to/tls/server.crt
      key:
        file: /path/to/tls/server.key
      # Whether clients are required to provide their TLS certificates for verification
      # Require client certificates / mutual TLS for inbound connections.
      # Note that clients that are not configured to use a certificate will
      # fail to connect to the node.
      clientAuthRequired: false
      # If mutual TLS is enabled, clientRootCAs.files contains a list of additional root certificates
      # used for verifying certificates of client connections.
      clientRootCAs:
        files:
        - path/to/client/tls/ca.crt

  # ------------------- Tracing Configuration -------------------------
  tracing:
    # Type of provider to be used: none (default), file, optl, console
    provider: optl
    # Tracer configuration when provider == 'file'
    file:
      # The file where the traces are going to be stored
      path: /path/to/client/trace.out
    # Tracer configuration when provider == 'optl'
    optl:
      # The address of collector where we should send the traces
      address: 127.0.0.1:8125
    sampling:
      # The ratio of the traces to be sampled
      ratio: 0.8

  # ------------------- Metrics Configuration -------------------------
  metrics:
    # provider can be statsd, prometheus, none or disabled
    provider: prometheus

    # only required if provider is set to statsd
    statsd:
      # network type: tcp or udp
      network: udp
      # statsd server address
      address: 127.0.0.1:8125
      # the interval at which locally cached counters and gauges are pushed
      # to statsd; timings are pushed immediately
      # No default, this must be specified
      writeInterval: 10s
      # prefix is prepended to all emitted statsd metrics
      prefix:


  # ------------------- FSC Node endpoint resolvers -------------------------
  # The endpoint section tells how to reach other FSC node in the network.
  # For each node, the name, the domain, the identity of the node, and its addresses must be specified.
  endpoint:
    resolvers:
    # name is a name that describes the FSC node (must also match name used in the view) it isn't a P2P bootstrap node
    - name: fscNodeA
      # domain can be used to distinguish nodes if name isn't unique
      domain:
      # the public identity of this node
      identity:
        path: /path/to/fscNodeA-cert.pem
      # endpoint addresses to associate with the resolver
      addresses:
      # alias names which can be used as alternative for lookups
      aliases:
      - anotherName
    # here is the definition of the bootstrap node. If this core.yaml is for this node, it doesn't need to be declared in the resolver list
    - name: theBootstrapNode
      domain:
      identity:
        path: /path/to/theBootstrapNode-cert.pem
      addresses:
        # P2P endpoint address for this node
        P2P: thebootstrapFQDN:20001
      aliases:
    # This demonstrates other keys available for addresses:, TBD
    - name: otheraddressestypes
      domain:
      identity:
        path: /path/to/some-cert.pem
      addresses:
        # Port at which the fsc node might listen for some service
        Listen:
        # Port at which the View Service Server respond
        View:
        # Port at which the Web Server respond
        Web:



# ----------------------- Fabric Driver Configuration ---------------------------
fabric:
  # Is the fabric-sdk enabled
  enabled: true
  mynetwork: # unique name of the fabric network configuration
    # it is the driver to use to provide the implementations of the Fabric API (client-side)
    # `generic` supports Fabric 2.x
    driver: generic
    # defines whether this is the default fabric network
    default: true
    # Cache size to use when handling idemix pseudonyms. If the value is larger than 0, the cache is enabled and
    # pseudonyms are generated in batches of the given size to be ready to be used.
    # if not specified then the default is 3
    mspCacheSize: 500
    # the default msp for this node (matches the id in the msps key)
    # TBD: what does being the default mean ?
    defaultMSP: mymsp
    # 1 or more msps this node can represent
    # TBD: but what does that mean ???? how do you know which one will be used ?
    msps:
        # a unique id for this msp
      - id: mymsp
        # type of msp, can be bccsp, bccsp-folder, idemix or idemix-folder
        mspType: bccsp
        # fabric mspid of this fsc node
        mspID: peerOrg2MSP
        # path to full local fabric defined msp structure (including private keys) of this fsc node
        path: /path/to/mymsp
        # Options, currently only key available is BCCSP (so do we need the BCCSP key ?)
        opts:
          BCCSP:
            # Can be SW or PKCS11
            Default: SW
            # Define the properties for a software based X509 system as opposed to a HSM based system
            # Only needs to be defined if the BCCSP Default is set to SW
            SW:
              Hash: SHA2
              Security: 256
            # Definition of PKCS11 configuration parameters when using a Hardware HSM
            # Only needs to be defined if the BCCSP Default is set to PKCS11.
            # NOTE: in order to use pkcs11, you have to build the application with "go build -tags pkcs11"
            PKCS11:
              # PKCS11 library
              Library: /path/to/pkcs11_library.so
              # PKCS11 Label
              Label: someLabel
              # PKCS11 Pin
              Pin: 98765432
              Hash: SHA2
              Security: 256

      # For Anonymous identities you need to define an entry with an id of `idemix`
      # and must be of mspType idemix
      - id: idemix
        mspType: idemix
        mspID: IdemixOrgMSP
        # Path to idemix credentials
        path: /path/to/myanonousmous/idemix
        # TDB: Optional, applies only to idemix, need to define the scale and meaning and what 0 means
        # used to override the MSPCacheSize
        cacheSize: 3

      # TBD: idemix-folder, bccsp-folder

    tls:
      # Species the fabric network requires TLS or not
      enabled:  true
      # Specifies whether the fabric network requires mutualTLS
      clientAuthRequired: false
      # The client tls certificate if mutualTLS is required
      clientCert:
        file: /path/to/client.crt
      # The client tls key if mutualTLS is required
      clientKey:
        file: /path/to/client.key

    # Client keepalive settings for GRPC
    keepalive:
      # If not provided, the default is 60 seconds
      interval: 60s
      # If not provided, the default is 20 seconds
      timeout: 600s
      connectionTimeout: 10s
      tlsEnabled: true

    ordering:
      # number of retries to attempt to send a transaction to an orderer
      # If not specified or set to 0, it will default to 3 retries
      numRetries: 3
      # retryInternal specifies the amount of time to wait before retrying a connection to the ordering service, it has no default and must be specified
      retryInterval: 3s

    # List of orderers on top of those discovered in the channel
    # This is optional and as such it should be left to those orderers discovered on the channel
    orderers:
        # address of orderer
      - address: 'orderer0:7050'
        # connection timeout
        connectionTimeout: 10s
        # path to orderer org's ca cert if tls is enabled
        tlsRootCertFile: /path/to/ordererorg/ca.crt
        # server name override if tls cert SANS doesn't match address
        serverNameOverride:

    # List of trusted peers this node can connect to.
    # usually this will be the fabric peers in the same organisation as the FSC node
    peers:
        # address of orderer
      - address: 'peer2:7051'
        # connection timeout
        connectionTimeout: 10s
        # path to peer org's ca cert if tls is enabled
        tlsRootCertFile: /path/to/peerorg/ca.crt
        serverNameOverride:

    # List of channels and deployed chaincode
    channels:
      - name: mychannel
        # whether this is the default channel or not
        # TBD: What is the meaning of a default channel ?
        default: true
        numRetries: 3 # number of retries on a chaincode operation failure
        retrySleep: 1s # waiting time before retry again a failed chaincode operation
        # section about the finality service
        finality:
          waitForEventTimeout: 20s
          forPartiesWaitTimeout: 1m
        # section about the committer service
        committer:
          waitForEventTimeout: 300s
          pollingTimeout: 100ms
          finality:
            numRetries: 3
            unknownTxTimeout: 100ms
          parallelism: 3 # maximum go routines to commit at the same time transactions of the same block
        # section about the delivery service
        delivery:
          waitForEventTimeout: 300s
          sleepAfterFailure: 10s
        # section about the discovery service  
        discovery:
          timeout: 10s
        # section about the chaincode this node should be aware of  
        chaincodes:
            # chaincode id
          - name: mychaincode
            # whether it is a fabric private chaincode or not
            private: false

    # ----------------------- Fabric Driver Configuration ---------------------------
    # Internal vault used to keep track of the RW sets assembed by this node during in progress transactions
    vault:
      persistence:
        # type can be badger (disk) sql or memory. See below for sql configuration options.
        type: badger
        opts:
          # persistence location
          path: /some/path
      txidstore:
        cache:
          # TBD: What does this cache, what does 0 mean and what is the scale
          # If not specified or set to <0 it defaults to 100.
          size: 200

    # ------------------- Fabric Node resolvers -------------------------
    # The endpoint section tells how to reach other Fabric nodes in the network.
    endpoint:
      resolvers:
      # a unique name which has to match what the view references ?
      - name: fscnodeA
        domain:
        identity:
          # mspid of identity
          mspID: peerOrg0MSP
          # path to the public MSP (ie no crypto material) or signing cert (but I would highly recommend not specifying just the signing cert)
          path: /path/to/fscnodeA/msp
          # TBD
          addresses:
          aliases:
          - anotherName
      - name: fscnodeB
        domain:
        identity:
          mspID: peerOrg2MSP
          path: /path/to/fscnodeB/msp
          addresses:
```

## Overriding configuration keys

Any value that is not a (grand-)child of a list can be overridden with an environment variable that is all uppercase, prefixed with `CORE_`,
and traversing the path in the yaml with underscores. This means that a key like fsc.endpoint.resolvers[0].name cannot be changed via environment variables. Examples:

```sh
CORE_LOGGING_LEVEL=debug
CORE_FSC_P2P_LISTENADDRESS=/ip4/0.0.0.0/tcp/9001
CORE_FSC_IDENTITY_KEY_FILE=/my/private.key
CORE_FSC_KVS_PERSISTENCE_OPTS_DATASOURCE=/mydb.sqlite
CORE_FSC_TRACING_OPTL_ADDRESS=jaeger.example.com:4318
CORE_FABRIC_MYNETWORK_KEEPALIVE_TIMEOUT=120s
```

And so on.

## HSM Support

In order to use a hardware HSM for x.509 identities, you have to build the application with
`CGO_ENABLED=1 go build -tags pkcs11` and configure the PKCS11 settings as describe above.

## Persistence: sql

Badger is the default key/valuestore for Fabric Smart Client and is the most performant. Alternatively, you can
select a golang/sql compatible driver. Although the data in Fabric Smart Client is key/value and not relational,
reasons to choose sql may include:

- Using a managed database for high availability, failover and backups
- Wanting a stateless Fabric Smart Client
- The ability to inspect stored data using standard tooling
- Compliance to organization policies.

The driver has been tested with the following sql drivers:

- SQLite: (pure go): modernc.org/sqlite
- Postgres (pure Go): github.com/jackc/pgx/v5/stdlib

In theory you can use any [sql driver](https://github.com/golang/go/wiki/SQLDrivers) if you import it in your application.
To try a new sql driver, add a test here: `token/services/db/driver/sql/sql_test.go`.

### Config example for sqlite:

Simple:

```yaml
persistence:
  type: sql
  opts:
    driver: sqlite
    dataSource: /some/path/fsc.sqlite
```


We use one connection for writes, and an unlimited number for concurrent read connections 
(see the excellent https://kerkour.com/sqlite-for-servers for more information).

Advanced, more customized settings:

```yaml
persistence:
  type: sql
  opts:
    driver: sqlite
    dataSource: file:/some/path/fsc.sqlite&_txlock=immediate
    tablePrefix: fsc  # optional
    skipCreateTable: true # tells FSC _not_ to create a table when starting up (because it already exists).
    skipPragmas: true # if this is false, the pragmas we set in the datasource will be overridden with the defaults.
    maxOpenConns: 50  # optional: max open read connections to the database. Defaults to unlimited.
```

By default we set the following pragmas (unless you do `skipPragmas: true`. Make sure you always have `_pragma=journal_mode(WAL`):

```sql
  PRAGMA journal_mode = WAL;
  PRAGMA busy_timeout = 5000;
  PRAGMA synchronous = NORMAL;
  PRAGMA cache_size = 1000000000;
  PRAGMA temp_store = memory;
```

### Config example for postgres

The same configuration flags as above apply, but for Postgres we always use one connection pool for reads and writes,
and the sqlite pragmas don't apply.

> [!WARNING]
> The 'dataSource' field is sensitive because it contains a password. Instead of in this file, set it in the
> `CORE_FSC_KVS_PERSISTENCE_OPTS_DATASOURCE` and `CORE_FABRIC_MYNETWORK_VAULT_PERSISTENCE_OPTS_DATASOURCE` environment
> variables (where mynetwork is the name of your network in core.yaml).

```yaml
persistence:
  type: sql
  opts:
    driver: postgres
    dataSource: host=localhost port=5432 user=postgres password=example dbname=tokendb sslmode=disable
```
