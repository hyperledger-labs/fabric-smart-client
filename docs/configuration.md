# Example configuration

The following configuration example provides descriptions for the various keys required for a Fabric Smart Client node that uses the Fabric SDK.

```yaml
---
# ------------------- Logging section ---------------------------
logging:
  # format is same as fabric [<logger>[,<logger>...]=]<level>[:[<logger>[,<logger>...]=]<level>...]
  format: '%{color}%{time:15:04:05.000} [%{module}] %{shortfunc} %{level:.4s}%{color:reset} %{message}'
  spec: grpc=error:debug
  otel:
    # sanitize makes sure that the strings sent to open telemetry as events via the logger as printable.
    # Non-printable strings can break the tracing system
    sanitize: true

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

  # ------------------- Shared server-side TLS defaults -------------------
  # Every listener this node owns (fsc.grpc and fsc.web) inherits from this
  # block FIELD BY FIELD: a service's own tls: block overrides only the fields it sets,
  # and every other field falls through to here. A service that overrides nothing needs
  # no tls: block at all.
  #
  # This block is server-shaped, because everything under fsc: that inherits from it is a
  # listener. It does NOT apply to connections this node dials out: those are configured
  # per Fabric network under fabric.<network>.tls, which has its own keys and does not
  # inherit from here. There is also no inheritance between sibling services.
  tls:
    # Whether TLS is enabled
    enabled: true
    # TLS certificate presented by every listener
    cert:
      file: /path/to/tls/server.crt
    # Private key matching that certificate
    key:
      file: /path/to/tls/server.key
    # Whether clients must present a certificate. See fsc.web below for the three states.
    clientAuthRequired: false

  # ------------------- GRPC Server Configuration -------------------------
  grpc:
    enabled: true
    # The listen address of this server
    address: 0.0.0.0:20000
    # ConnectionTimeout specifies the timeout for connection establishment for all new connections
    # If not specified or set to <=0 then it will default to 5 seconds
    connectionTimeout: 10s

    # Only the fields that differ from fsc.tls need to appear here. enabled, cert and key
    # are shown for completeness; in practice they are inherited and omitted.
    tls:
      # Whether TLS is enabled or not. Inherited from fsc.tls when absent.
      enabled: true
      # Whether clients are required to provide their TLS certificates for verification.
      # Inherited from fsc.tls when absent.
      clientAuthRequired: false
      # TLS Certificate. Inherited from fsc.tls when absent.
      cert:
        file: /path/to/tls/server.crt
      # TLS Key. Inherited from fsc.tls when absent.
      key:
        file: /path/to/tls/server.key

      # Root certificates used to verify client TLS certificates. REQUIRED when
      # clientAuthRequired is true: a node configured to require client certificates with
      # an empty pool would reject every client, so that combination fails at startup
      # rather than at the first connection.
      clientRootCAs:
        files:
        - /path/to/client/tls/ca.crt

    # GRPC Server keepalive parameters. 
    # This section can be omitted.
    keepalive:
      # MaxConnectionIdle: send GOAWAY and gracefully close if connection is idle this long (no RPCs).
      # Format: Go duration string (e.g. "5m", "30s"). Zero/omitted disables.
      max-connection-idle: "5m"
      # MaxConnectionAge: maximum lifetime of a connection before server initiates close to rotate connections.
      # Format: Go duration string. Zero/omitted disables.
      max-connection-age: "2h"
      # MaxConnectionAgeGrace: additional grace period after MaxConnectionAge to allow in-flight RPCs to finish.
      # Format: Go duration string.
      max-connection-age-grace: "5m"
      # Time: server's expectation for how often the client should send keepalive pings.
      # Format: Go duration string.
      time: "2m"
      # Timeout: how long the server waits for a keepalive ping ACK before considering the connection dead.
      # Format: Go duration string.
      timeout: "20s"
      # EnforcementPolicy: maps to keepalive.EnforcementPolicy; may be omitted to disable enforcement.
      enforcement-policy:
        # MinTime: minimum allowed time between client pings; server may close connections that ping more often.
        # Format: Go duration string.
        min-time: "1m"
        # PermitWithoutStream: allow keepalive pings even when there are no active RPC streams.
        # true = permit pings without streams (recommended if clients ping periodically).
        permit-without-stream: true

  # ------------------- P2P Configuration -------------------------
  p2p:
    # Type of p2p communication. Currently supported: websocket, libp2p.
    # There is no default -- an unset or unrecognised type is rejected at
    # startup. websocket is the primary implementation; libp2p is optional and
    # lives in its own Go module.
    type: websocket
    # listen address see https://github.com/libp2p/specs/blob/master/addressing/README.md
    # for information on the format
    listenAddress: /dns4/myhostname/tcp/20001
    # Buffer size for the incoming messages channel. Default: 4096
    # This controls how many messages can be queued before blocking message dispatch.
    # Larger values can improve throughput for high-volume message processing
    # but will consume more memory.
    incomingMessagesBufferSize: 4096
    # Buffer size for stream readers. Default: 4096
    # This controls the internal buffer size used when reading protobuf messages from streams.
    # Larger values can improve read performance for large messages
    # but will consume more memory per active connection.
    streamReaderBufferSize: 4096
    # Maximum allowed size (in bytes) for incoming P2P messages. Default: 10485760 (10 MiB)
    # This protects the node from memory exhaustion by rejecting oversized payloads before deserialization.
    # Set to 0 to disable the limit (allow arbitrarily large messages - not recommended).
    maxRecvMsgSize: 10485760
    # Maximum allowed size (in bytes) for outgoing P2P messages. Default: 10485760 (10 MiB)
    # Set to 0 to disable the limit (allow arbitrarily large messages - not recommended).
    maxSendMsgSize: 10485760
    opts:
      # ------------------- libp2p specific options -------------------------
      # Only needed when type == libp2p
      libp2p:
        # bootstrap node
        # if it's empty then this node is the bootstrap node, otherwise it's the name
        # of the bootstrap node, which must be defined in the FSC endpoint resolvers section
        # and that entry must have an address with an entry P2P.
        bootstrapNode: theBootstrapNode
        # Connection manager settings for libp2p
        connManager:
          # Low water mark - when the number of connections drops below this, the connection manager
          # will not prune any connections. Default: 100
          lowWater: 100
          # High water mark - when the number of connections exceeds this, the connection manager
          # will prune connections until it reaches the low water mark. Default: 400
          highWater: 400
          # Grace period - connections younger than this will not be pruned. Default: 60s
          # Format: duration in seconds
          gracePeriod: 60
      
      # ------------------- websocket specific options -------------------------
      # Only needed when type == websocket
      websocket:
        # Maximum number of sub-connections per peer. Default: 100
        maxSubConns: 100
        # Comma-separated list of allowed origins for CORS (Cross-Origin Resource Sharing)
        # Example: "https://example.com,https://app.example.com"
        # If not set, CORS is disabled. Only applicable when using websocket transport.
        corsAllowedOrigins: ""
        # TLS configuration for websocket connections
        tls:
          # Whether clients are required to provide certificates.
          # Defaults to true for websocket p2p when omitted.
          clientAuthRequired: true
          # Root certificates used by this node (as a websocket client) to verify remote server certificates.
          serverRootCAs:
            files:
              - /path/to/server/tls/ca.crt
          # Root certificates used by this node (as a websocket server) to verify remote client certificates.
          clientRootCAs:
            files:
              - /path/to/client/tls/ca.crt

  # ------------------- KVS Configuration -------------------------
  # Internal key/value store used by the node to store information
  # such as bindings (eg resolvers)
  kvs:
    cache:
      # Sets the maximum number of cached items
      # If not specified, default is 100 (TBD: What is the scale here ?, what does 0 mean)
      size:

  # ------------------- Persistence Configuration -------------------------
  # All persistence configurations of the application are defined here
  # Then each store chooses a configuration by referencing it by the key (e.g. my_sqlite_persistence)
  # If a store does not define one, the 'default' will be picked if it is defined 
  persistences:
    # The default persistence configuration for all stores that do not define one or do not support customization.
    # A default persistence. It is the safest choice to define one. 
    default:
      # The type can be memory, sqlite, postgres
      type: memory
    # The persistence configuration for all stores that define the option
    # persistence: my_sqlite_persistence
    # See more details on available options below
    my_sqlite_persistence:
      type: sqlite
      opts:
        dataSource: /path/to/sqlite
        maxIdleConns: 10
        skipPragmas: false
    my_postgres_persistence:
      type: postgres
      opts:
        dataSource: host=localhost port=5432 user=postgres password=example dbname=tokendb sslmode=disable
        maxOpenConns: 20
  # ------------------- Web Server Configuration -------------------------
  # Web server must be enabled to support healthz, version and prometheus /metrics
  # end points.
  web:
    enabled: true
    address: 0.0.0.0:20002
    # As with fsc.grpc, only the fields that differ from fsc.tls need to appear here.
    tls:
      # Whether TLS is enabled. Inherited from fsc.tls when absent.
      enabled:  true
      # Inherited from fsc.tls when absent.
      cert:
        file: /path/to/tls/server.crt
      key:
        file: /path/to/tls/server.key
      # Client authentication has THREE states on this listener:
      #
      #   clientAuthRequired: true                    -> a client certificate is required
      #                                                  and verified; clientRootCAs must
      #                                                  be non-empty.
      #   clientAuthRequired: false + clientRootCAs   -> a client certificate is verified
      #                                                  IF offered, but not demanded.
      #   clientAuthRequired: false, no clientRootCAs -> client certificates are ignored.
      #
      # The middle state is the reason clientRootCAs may be set while clientAuthRequired
      # is false; it is a supported configuration, not a mistake.
      clientAuthRequired: false
      # Root certificates used to verify client TLS certificates.
      clientRootCAs:
        files:
        - path/to/client/tls/ca.crt

  # ------------------- Tracing Configuration -------------------------
  tracing:
    # Type of provider to be used: none (default), file, otlp, console
    provider: otlp
    # Tracer configuration when provider == 'file'
    file:
      # The file where the traces are going to be stored
      path: /path/to/client/trace.out
    # Tracer configuration when provider == 'otlp'
    otlp:
      # The address of collector where we should send the traces
      address: 127.0.0.1:8125
      # Client-side TLS for the collector connection, in the same client template as every
      # other dialled connection. It inherits nothing: fsc.tls is server-shaped, and the
      # collector is something this node dials.
      #
      # TLS here is OPT-IN. Omit the block and the exporter stays plaintext, which is what a
      # collector reached over loopback wants and what every existing setup relies on.
      tls:
        enabled: false
        rootCAs:
          files:
          - /path/to/collector/ca.crt
        # Present a client certificate, if the collector requires one
        clientAuthEnabled: false
        clientCert:
          file: /path/to/client.crt
        clientKey:
          file: /path/to/client.key
        # if the collector's certificate does not cover the address being dialled
        serverNameOverride: ""
    sampling:
      # The ratio of the traces to be sampled
      ratio: 0.8

  # ------------------- Metrics Configuration -------------------------
  metrics:
    # provider can be prometheus, none or disabled
    provider: prometheus

    # Require a verified client certificate on /metrics and /logspec.
    #
    # This is NOT transport TLS, and is deliberately separate from any listener's tls block:
    # it can be stricter than the listener. A common shape is a web listener that verifies a
    # client certificate only if one is offered, while scraping metrics requires one.
    # Replaces fsc.metrics.prometheus.tls, which meant this despite its name.
    clientAuthRequired: false

    # Serve the operations endpoints (/metrics, /logspec) on a listener of their own.
    #
    # Without an address they are served on the fsc.web listener and share its TLS, which is
    # the historical behaviour. With one, they get their own listener and fsc.metrics.tls
    # applies to it, inheriting from fsc.tls field by field like any other listener — so a
    # plaintext metrics endpoint behind a TLS web listener becomes expressible.
    #
    # fsc.metrics.tls without fsc.metrics.address is a startup error: the shared listener
    # cannot honour it, and the configuration would be claiming transport security it does
    # not have.
    # address: 0.0.0.0:20003
    # tls:
    #   enabled: false

    
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
              # Optional. Maximum number of PKCS11 sessions kept in the pool.
              # Higher values reduce contention under concurrent signing load, at
              # the cost of more open HSM sessions. Defaults to the library
              # default (10) when unset.
              SessionCacheSize: 10

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

    # Client-side TLS for every connection this node dials on this network: orderers, peers
    # and, for Fabric-x, the query and notification services.
    #
    # This block is CLIENT-shaped, unlike fsc.tls which is server-shaped. Each endpoint under
    # orderers: and peers: inherits it FIELD BY FIELD and overrides only what it sets; there
    # is no inheritance across the fsc/fabric boundary in either direction.
    tls:
      # Whether connections to this network use TLS.
      enabled:  true
      # Present a client certificate to the server. Replaces clientAuthRequired, which named
      # a server-side concept for a block that is only ever a client. Defaults to true when
      # clientCert and clientKey both resolve; set it false to suppress inherited credentials
      # for a particular endpoint.
      clientAuthEnabled: false
      # The client tls certificate, if the server requires one
      clientCert:
        file: /path/to/client.crt
      # The client tls key, paired with clientCert
      clientKey:
        file: /path/to/client.key
      # Trust anchors used to verify the servers this node dials.
      #
      # Anchors discovered from a channel's MSPs AUGMENT this pool rather than replacing it,
      # matching Fabric's own semantics for tls.rootcert. That is what lets a node dial an
      # orderer before it has fetched the first configuration block, and it means the file
      # cannot remove an anchor the channel supplies.
      rootCAs:
        files:
        - /path/to/ca.crt
      # Override the hostname verified in the server's certificate. Needed when dialling an
      # IP address against a certificate issued for a name. Replaces serverhostoverride.
      serverNameOverride: ""

    # Client keepalive settings for GRPC.
    # This section can be omitted.
    keepalive:
      # Time: how often the client sends keepalive pings to the server.
      # Format: Go duration string (e.g. "30s", "2m"). Zero/omitted disables.
      time: "2m"
      # Timeout: how long the client waits for a keepalive ACK from the server
      # before considering the connection dead.
      # Format: Go duration string. Should be noticeably smaller than `time`.
      timeout: "20s"
      # PermitWithoutStream: allow keepalive pings even when there are no active RPCs.
      # true = permit pings without active streams (recommended for many clients).
      permit-without-stream: true

    ordering:
      # number of retries to attempt to send a transaction to an orderer
      # If not specified or set to 0, it will default to 3 retries. The orderer is picked randomly for every attempt.
      numRetries: 3
      # retryInternal specifies the amount of time to wait before retrying a connection to the ordering service, it defaults to 500ms
      retryInterval: 500ms
      #
      # ordering.tlsEnabled and ordering.tlsClientAuthRequired have been REMOVED. They existed
      # to shadow the network tls block for orderer connections alone, which meant the same
      # two settings had two homes and the narrower one silently won. The network block now
      # applies to every connection; to differ for one orderer, set tls: on that entry under
      # orderers: below. A stale key here is a startup error naming its replacement.

    # List of orderers on top of those discovered in the channel
    # This is optional and as such it should be left to those orderers discovered on the channel
    # TLS is inherited from the `tls` section above, per field, unless an entry overrides it
    orderers:
        # address of orderer
      - address: 'orderer0:7050'
        # connection timeout
        connectionTimeout: 10s
        # Per-orderer TLS. Every field is inherited from the network's tls block above; set
        # only what differs for this endpoint. Omit the block entirely to inherit all of it.
        #
        # This replaces the flat per-endpoint keys: tlsEnabled and tlsDisabled become
        # enabled, tlsClientSideAuth becomes clientAuthEnabled, tlsRootCertFile becomes
        # rootCAs.files, and serverNameOverride moves inside. A stale flat key is a startup
        # error naming the key, not a silently ignored line.
        tls:
          enabled: false
          clientAuthEnabled: true
          rootCAs:
            files:
            - /path/to/ordererorg/ca.crt
          # if the certificate's SANs do not cover the address being dialled
          serverNameOverride: orderer0.example.com

    # List of trusted peers this node can connect to.
    # usually this will be the fabric peers in the same organisation as the FSC node.
    peers:
        # address of peer
      - address: 'peer2:7051'
        # connection timeout
        connectionTimeout: 10s
        # Per-peer TLS, inherited from the network's tls block exactly as for orderers above.
        tls:
          enabled: false
          clientAuthEnabled: true
          rootCAs:
            files:
            - /path/to/peerorg/ca.crt
          serverNameOverride: peer2.example.com
        # `usage` allows the developer to specify the function for which this peer should be used.
        # The available functions are: delivery, discovery, finality, and query.
        # The default value is the empty string that means that the peer can be used for the supported operations.
        usage: 

    # Fabric-x query and notification services (FabricX only).
    #
    # Each endpoint's tls: block is the same client template as orderers and peers above, and
    # inherits from this network's tls block field by field. It replaces a block that spelled
    # the same thing its own way: rootCerts becomes rootCAs.files, and clientKey/clientCert
    # become nested {file:} rather than flat path strings. A stale flat key is a startup error.
    queryService:
      requestTimeout: 10s
      endpoints:
        - address: 'sidecar:4001'
          connectionTimeout: 10s
          tls:
            enabled: true
            rootCAs:
              files:
              - /path/to/sidecar/ca.crt
            clientCert:
              file: /path/to/client.crt
            clientKey:
              file: /path/to/client.key
    # notificationService takes the same endpoints and tls shape.

     # Channel Configuration Monitor settings (FabricX only)
     # Applies to all channels in this network
    configMonitor:
       # How often to check for configuration updates
       # Default: 1m (1 minute)
       # Format: Go duration string (e.g., "30s", "2m", "1h")
       pollInterval: 60s

       # Maximum number of retry attempts for failed operations
       # Default: 5
       # Set to 0 to disable retries
       maxRetries: 5

       # Initial delay before the first retry attempt
       # Default: 1s
       # Format: Go duration string
       initialRetryDelay: 1s

       # Maximum delay between retry attempts (exponential backoff cap)
       # Default: 5m
       # Format: Go duration string
       maxRetryDelay: 5m

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

    # ----------------------- Fabric Driver Configuration ---------------------------
    # Internal vault used to keep track of the RW sets assembled by this node during in progress transactions
    vault:
      persistence: my_postgres_persistence
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
CORE_FSC_TRACING_OTLP_ADDRESS=jaeger.example.com:4318
CORE_FABRIC_MYNETWORK_KEEPALIVE_TIMEOUT=120s
```

And so on.

## HSM Support

In order to use a hardware HSM for x.509 identities, you have to build the application with
`CGO_ENABLED=1 go build -tags pkcs11` and configure the PKCS11 settings as describe above.

## Persistence: sqlite/postgres

You can select a golang/sql compatible driver. Although the data in Fabric Smart Client is key/value and not relational,
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
  type: sqlite
  opts:
    dataSource: /some/path/fsc.sqlite
```


We use one connection for writes, and an unlimited number for concurrent read connections 
(see the excellent https://kerkour.com/sqlite-for-servers for more information).

Advanced, more customized settings:

```yaml
persistence:
  type: sqlite
  opts:
    dataSource: file:/some/path/fsc.sqlite&_txlock=immediate
    tablePrefix: fsc  # optional
    skipCreateTable: true # tells FSC _not_ to create a table when starting up (because it already exists).
    skipPragmas: true # if this is false, the pragmas we set in the datasource will be overridden with the defaults (sqlite specific).
    maxOpenConns: 20  # optional: max open read connections to the database. Defaults to unlimited. See https://go.dev/doc/database/manage-connections.
    maxIdleConns: 20  # optional: max idle read connections to the database. Defaults to 2.
    maxIdleTime: 30s  # optional: max duration a connection can be idle before it is closed. Defaults to 1 minute.
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
  type: postgres
  opts:
    dataSource: host=localhost port=5432 user=postgres password=example dbname=tokendb sslmode=disable
    maxOpenConns: 25  # optional: max open read connections to the database. Defaults to unlimited. 
    maxIdleConns: 25  # optional: max idle read connections to the database. Defaults to 2.
    maxIdleTime: 30s  # optional: max duration a connection can be idle before it is closed. Defaults to 1 minute.
    tls:
      enabled: true              # when false (or omitted) the tls block is ignored and the dataSource is used as-is
      ssl_mode: verify-full      # optional: defaults to verify-full when empty. See the table below.
      server_name: db.example.com # optional: overrides the hostname used for verify-full verification/SNI. Defaults to the dataSource host.
      cert_path: /path/to/client.crt      # optional: client certificate for mutual TLS
      key_path: /path/to/client.key       # optional: client private key for mutual TLS
      root_cert_path: /path/to/ca.crt     # optional: CA used to verify the server certificate (verify-ca / verify-full)
```

`ssl_mode` follows the standard PostgreSQL/libpq semantics
(https://www.postgresql.org/docs/current/libpq-ssl.html):

| ssl_mode      | Encrypted | Verifies CA | Verifies hostname | Notes |
|---------------|-----------|-------------|-------------------|-------|
| `disable`     | no        | no          | no                | The `dataSource` is used unchanged; no other tls option has any effect. |
| `allow`       | maybe     | no          | no                | Tries a plaintext connection first, falls back to TLS. |
| `prefer`      | maybe     | no          | no                | Tries TLS first, falls back to a plaintext connection. |
| `require`     | yes       | no          | no                | Encrypts but does not validate the server certificate. |
| `verify-ca`   | yes       | yes         | no                | Validates the certificate chain against `root_cert_path`. |
| `verify-full` | yes       | yes         | yes               | Validates the chain and that the hostname matches. **This is the default when `ssl_mode` is empty.** |

> [!NOTE]
> Unlike libpq (whose default is `prefer`), an empty `ssl_mode` defaults to the
> strictest mode, `verify-full`. Set it explicitly if you need a more permissive mode.

> [!NOTE]
> For `verify-ca` and `verify-full`, if `root_cert_path` is omitted the server
> certificate is validated against the host's system CA pool. A server using an
> internal/self-signed CA (the common case for Postgres) will therefore fail to
> verify unless you point `root_cert_path` at that CA.

For more info about managing connections, see https://go.dev/doc/database/manage-connections. Keep in mind that Fabric Smart Client
maintains two independent database instances: one for KVS and one for the Vault. The combined maxOpenConns should not exceed the
configured max_connections in the postgres server (100 by default).
