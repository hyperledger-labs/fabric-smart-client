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
  # It's also used as the unique name to resolve this node's identity and grpc server endpoint.
  # It is required: the node fails to start without it.
  id: someid

  # This is the identity of the node
  identity:
    # Selects the key-management driver. file is the default and the only built-in driver;
    # an unknown value is rejected with an error.
    type: file
    cert:
      file: /path/to/cert.pem
    key:
      file: /path/to/key.pwm

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
    # Whether clients must present a certificate. fsc.grpc and fsc.web interpret it together
    # with clientRootCAs in the same three states; see fsc.web below.
    clientAuthRequired: false
    # The TLS version range to negotiate, as crypto/tls version numbers: 771 = TLS 1.2,
    # 772 = TLS 1.3. Both are inherited from the parent tls block when absent. fsc.grpc's
    # listener defaults to 771/771; other listeners (fsc.web, fsc.metrics) and every dialled
    # connection default to 771/772. The values below are fsc.grpc's own default, shown for
    # completeness -- copying maxVersion: 771 into a real config also caps fsc.web and
    # fsc.metrics at TLS 1.2, since both inherit from this block and otherwise default to
    # 772.
    minVersion: 771
    maxVersion: 771

  # ------------------- GRPC Server Configuration -------------------------
  # The gRPC listener. It serves the remote view API when fsc.view.remote.backends lists
  # grpc; enabling it without fsc.view.remote fails startup.
  grpc:
    enabled: true
    # The listen address of this server
    address: 0.0.0.0:20000
    # ConnectionTimeout specifies the timeout for connection establishment for all new connections
    # If not specified or set to <=0 then it will default to 5 seconds
    connectionTimeout: 10s
    # The largest message in bytes this server will accept and send. Omit or set to 0
    # to use the default of 100 MiB. Unlike fsc.p2p.maxRecvMsgSize below, zero here means
    # the default, not unlimited.
    maxRecvMsgSize: 104857600
    maxSendMsgSize: 104857600

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

      # Root certificates used to verify client TLS certificates. Client authentication has
      # the same three states as on fsc.web (see below): clientAuthRequired: true demands a
      # certificate and verifies it against these roots, which must then be non-empty (that
      # combination fails at startup, since an empty pool would reject every client); these
      # roots without clientAuthRequired verify a certificate only if the client offers one;
      # no roots means client certificates are not requested. Whenever a client certificate
      # was verified, the view service accepts only commands whose tlsCertHash names it.
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
    # Buffer size for the incoming messages channel. Default: 1024
    # This controls how many messages received from all streams can wait for the dispatcher
    # before the stream readers block. Each session has its own fixed-size queue.
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
    # The limit cannot be disabled: zero or negative values fall back to the default.
    # Applies to every p2p type, including websocket, where it also sizes the websocket read limit.
    maxRecvMsgSize: 10485760
    # Maximum allowed size (in bytes) for outgoing P2P messages. Default: 10485760 (10 MiB)
    # The limit cannot be disabled: zero or negative values fall back to the default.
    # Applies to every p2p type, including websocket.
    maxSendMsgSize: 10485760
    # Maximum number of responder views running concurrently for one remote peer. Default: 1000
    # A first message over the limit is rejected with an error to the initiator; once as many
    # rejections are pending for that peer, further first messages are dropped silently. This
    # bounds the goroutines, view contexts and sessions a single peer can make the node hold.
    # The same number separately bounds the rejections pending for one peer.
    # The limit cannot be disabled: zero or negative values fall back to the default.
    maxRespondersPerPeer: 1000
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
          rootCAs:
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
      # Sets the maximum number of cached items.
      # If not specified, default is 100. Must be >= 1: the cache cannot be
      # disabled, and a size below 1 is rejected at startup.
      size:

  # ------------------- Persistence Configuration -------------------------
  # All persistence configurations of the application are defined here
  # Then each store chooses a configuration by referencing it by the key (e.g. mysqlite)
  # If a store does not define one, the 'default' will be picked if it is defined.
  # Creating a store fails if it references a key that is not defined here,
  # or if the referenced configuration does not set a type.
  persistences:
    # The default persistence configuration for all stores that do not define one or do not support customization.
    # Define it: without it, such stores fall back to memory, log a warning, and lose their data on restart.
    default:
      # The type can be memory, sqlite, postgres. Required.
      type: memory
    # The persistence configuration for all stores that define the option
    # persistence: mysqlite
    # See more details on available options below
    mysqlite:
      type: sqlite
      opts:
        dataSource: /path/to/sqlite
        maxIdleConns: 10
        skipPragmas: false
    mypostgres:
      type: postgres
      opts:
        dataSource: host=localhost port=5432 user=postgres password=example dbname=tokendb sslmode=disable
        maxOpenConns: 20
  # ------------------- View Configuration -------------------------
  view:
    # The remote view API: the view service as reachable from other processes. It is off
    # unless enabled is true, and is never switched on by a listener's enabled flag. A key
    # not listed here fails startup, so a misspelt key never leaves its default in place.
    remote:
      # false (the default): the remote view API is served on no listener, and no key of
      # this block other than enabled may be set. Local view calls are unaffected.
      enabled: true
      # The transports the remote view API is served on: grpc (the fsc.grpc listener) and
      # web (the HTTP binding on the fsc.web listener). Required when enabled; each listed
      # transport needs its listener enabled. From the environment:
      # CORE_FSC_VIEW_REMOTE_BACKENDS=grpc,web.
      backends: [grpc, web]
      # Execution timeout of views called remotely, on every backend; views called locally
      # keep their caller's context. Default: 2m. A view still running when it expires ends
      # with the error "view execution timed out". Views must observe their context (e.g.
      # ctx.Done()) for the timeout to take effect. Zero or negative values fall back to the
      # default.
      timeout: 2m
      # A command is accepted only if its header timestamp is within this window of the
      # local clock, and only once: accepted commands are remembered for twice the window.
      # The memory is per process, so a restarted node, or another node serving the same
      # identities, accepts a command again within its window. Default: 15m; must be a
      # positive duration.
      timeWindow: 15m
      # Identities, besides the node's default identity (fsc.identity), allowed to call
      # views. An entry that does not load as a certificate fails startup. From the
      # environment, a comma-separated list of paths.
      clients:
      - path/to/client-cert.pem
      # Largest SignedCommand, or later stream frame, the remote view API accepts, in bytes.
      # Optional; must be positive when set. The view service rejects a larger command on
      # every backend with "command exceeds fsc.view.remote.maxMessageSize". Each listener's
      # own limit applies first: fsc.grpc.maxRecvMsgSize on fsc.grpc, fsc.web.maxRecvMsgSize
      # on fsc.web.
      maxMessageSize: 10485760
  # ------------------- Web Server Configuration -------------------------
  # The web listener serves the view service's HTTP binding when fsc.view.remote.backends
  # lists web (POST /v1/ViewService/ProcessCommand and the WebSocket GET
  # /v1/ViewService/StreamCommand; see docs/platform/view/services/view-service-http.md)
  # and, without fsc.metrics.address, the operations endpoints /metrics and /logspec.
  # Callers of the view service are authenticated and authorized by their signed commands
  # exactly as on fsc.grpc.
  #
  # The listener serves two probe endpoints that skip the client-certificate check of the
  # view endpoints. The TLS handshake still applies: with tls.clientAuthRequired: true, a
  # client without a certificate is rejected before it reaches them.
  #   GET /healthz -> 200 while the process serves HTTP. It never checks external
  #                   components, so a peer or orderer outage does not fail liveness.
  #   GET /readyz  -> 200 when every component that reports connection state
  #                   (fsc_connection_state) has been connected since startup and none
  #                   is stopped; reconnecting is still ready. Otherwise 503, with a JSON
  #                   list of the unready components (network, channel, component, and
  #                   reason: stopped or never connected). Endpoints are on /metrics.
  #
  # When a view fails, the view service returns its error text to the client verbatim, in
  # Error.Message, on fsc.grpc and fsc.web alike. A view that exceeds fsc.view.remote.timeout
  # gets the fixed message "view execution timed out" instead (see fsc.view.remote). The text is the
  # full error chain of the view and everything it called, including the value of a panic,
  # and can carry database errors, file paths, addresses or identities. Anyone who can call
  # views on this listener sees it, so set tls.clientAuthRequired: true below unless the
  # listener is reachable only by the node's own applications. Without fsc.metrics.address,
  # /metrics and /logspec share this listener; give them an address of their own when
  # parties that must not call views need to reach them.
  web:
    enabled: true
    # Listen address (host:port). Required when enabled; the node refuses to start without it.
    address: 0.0.0.0:20002
    # The largest request body or WebSocket message, in bytes, the view service's HTTP
    # binding reads; a larger one gets HTTP 413 or ends the stream. Omit or set to 0 to use
    # the default of 100 MiB; a negative value fails startup. It counts protojson bytes,
    # which are larger than the binary encoding of the same command.
    maxRecvMsgSize: 104857600
    # As with fsc.grpc, only the fields that differ from fsc.tls need to appear here.
    tls:
      # Whether TLS is enabled. Inherited from fsc.tls when absent.
      enabled:  true
      # Inherited from fsc.tls when absent.
      cert:
        file: /path/to/tls/server.crt
      key:
        file: /path/to/tls/server.key
      # Client authentication has THREE states, on this listener and on fsc.grpc:
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
      # Root certificates used to verify client TLS certificates, in the three states above.
      # The view service does not authorize callers by certificate: it authorizes the signer
      # of each command against the node's identity and fsc.view.remote.clients, and binds a
      # command to a verified client certificate when the connection carries one.
      clientRootCAs:
        files:
        - path/to/client/tls/ca.crt

  # ------------------- Tracing Configuration -------------------------
  tracing:
    # Type of provider to be used: none (default), file, otlp, console.
    # Values are case-insensitive; any other value is a startup error.
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
      # Fraction of root spans to sample, in [0, 1]. Defaults to 1 (sample every trace)
      # when unset. Spans continuing a remote trace follow the caller's sampling decision,
      # so 0 records only traces that were sampled upstream.
      ratio: 0.8

  # ------------------- Metrics Configuration -------------------------
  metrics:
    # provider can be prometheus, none or disabled
    provider: prometheus

    # Require a verified client certificate on /metrics and /logspec. Defaults to true when
    # absent, because /logspec changes the node's log spec; set false explicitly to serve the
    # endpoints without a certificate, e.g. on a network-restricted fsc.metrics.address.
    # A value that is not a boolean is a startup error. On a listener that never requests a
    # client certificate (TLS disabled, or no clientRootCAs), an explicit true is a startup
    # error too; when the key is absent, the node starts and logs a warning, and the
    # endpoints reject every request.
    #
    # This is NOT transport TLS, and is deliberately separate from any listener's tls block:
    # it can be stricter than the listener. A common shape is a web listener that verifies a
    # client certificate only if one is offered, while scraping metrics requires one.
    # Replaces fsc.metrics.prometheus.tls, which meant this despite its name.
    # clientAuthRequired: true

    # Serve the operations endpoints (/metrics, /logspec) on a listener of their own.
    #
    # Without an address they are served on the fsc.web listener and share its TLS. With one,
    # they get their own listener and fsc.metrics.tls applies to it, inheriting from fsc.tls
    # field by field like any other listener — so a plaintext metrics endpoint behind a TLS
    # web listener becomes expressible.
    #
    # fsc.metrics.tls without fsc.metrics.address is a startup error: the shared listener
    # cannot honour it, and the configuration would be claiming transport security it does
    # not have.
    #
    # A plaintext listener cannot verify a client certificate, so it also needs
    # clientAuthRequired: false, replacing the clientAuthRequired line above.
    # address: 0.0.0.0:20003
    # tls:
    #   enabled: false
    # clientAuthRequired: false

    
  # ------------------- FSC Node endpoint resolvers -------------------------
  # The endpoint section tells how to reach other FSC node in the network.
  # Each entry needs a name and an identity; domain, addresses and aliases are optional.
  # The node fails to start if an entry has no name or no identity.path, or if an address key is not
  # one of Listen, View or P2P (case-insensitive).
  # An entry that reuses the name of an earlier entry, such as a replica of the same node, does not get
  # its own addresses: its identity is bound to the identity of the earlier entry.
  # A replicated node is therefore reached at one P2P address, that of a TCP load balancer in front of
  # its replicas. The balancer must pass TCP through rather than terminate TLS, because the websocket
  # transport authenticates both sides from the TLS certificates, and every replica must hold the same
  # TLS key pair. Use GET /readyz on the fsc.web listener as the balancer's health check.
  endpoint:
    resolvers:
    # name is a name that describes the FSC node (must also match name used in the view) it isn't a P2P bootstrap node
    - name: fscNodeA
      # domain can be used to distinguish nodes if name isn't unique
      domain:
      # the public identity of this node. With websocket P2P this must be the node's fsc.identity
      # certificate: the node's peer ID is derived from it, and a websocket client accepts the
      # node's TLS server certificate only if it carries the same public key.
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
    # This demonstrates the other keys available for addresses:
    - name: otheraddressestypes
      domain:
      identity:
        path: /path/to/some-cert.pem
      addresses:
        # Port at which the fsc node might listen for some service
        Listen:
        # Port at which the View Service Server respond
        View:



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
    # pseudonyms are generated in batches of the given size to be ready to be used. 0 disables the cache.
    # if not specified then the default is 3. A negative or non-integer value is a startup error.
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
        path: /path/to/myanonymous/idemix
        # TBD: Optional, applies only to idemix, need to define the scale and meaning and what 0 means
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
      # The TLS version range to negotiate, as crypto/tls version numbers: 771 = TLS 1.2,
      # 772 = TLS 1.3. Both are inherited from the parent tls block when absent. This block
      # is client-side only; every dialled connection defaults to 771/772, shown here.
      minVersion: 771
      maxVersion: 772

    # Client gRPC settings for every connection to this network's peers and orderers.
    # This section can be omitted.
    keepalive:
      # ConnectionTimeout: how long to wait when dialling a peer or orderer that does
      # not set its own connectionTimeout. An entry under orderers or peers overrides
      # this. Format: Go duration string. Default: 10s.
      connectionTimeout: 10s
      # Time: how often the client sends keepalive pings to the server. Zero or omitted
      # disables keepalive; a connection that dies without an error then goes unnoticed
      # for hours. Values below 10s are raised to 10s. Fabric peers and orderers close
      # connections that ping more often than their configured minimum interval
      # (default 60s), so do not set it lower than that.
      # Format: Go duration string (e.g. "90s", "2m").
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
      # maximum number of concurrent connections per pool. CFT keeps one pool for the network;
      # BFT keeps one per orderer, so 4 orderers allow up to 4 x connectionPoolSize connections.
      # It defaults to 10; a value below 1 is a startup error.
      connectionPoolSize: 10
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
        # Connection timeout for this orderer. Overrides keepalive.connectionTimeout.
        # Omit or set to 0 to use the network keepalive value.
        connectionTimeout: 10s
        # The largest message in bytes to send to and accept from this endpoint. Omit or
        # set to 0 for the default of 100 MiB -- unlike fsc.p2p.maxRecvMsgSize above, zero
        # here is not "unlimited".
        maxRecvMsgSize: 104857600
        maxSendMsgSize: 104857600
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
        # Connection timeout for this peer. Overrides keepalive.connectionTimeout.
        # Omit or set to 0 to use the network keepalive value.
        connectionTimeout: 10s
        # The largest message in bytes to send to and accept from this endpoint. Omit or
        # set to 0 for the default of 100 MiB -- unlike fsc.p2p.maxRecvMsgSize above, zero
        # here is not "unlimited".
        maxRecvMsgSize: 104857600
        maxSendMsgSize: 104857600
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
      # One or more endpoints, in order of preference. Each gets its own connection, dialled
      # without blocking, and every request uses the first one that is not failing. List a
      # second sidecar so the node keeps working while the first is down; it moves back to
      # the first once that one reconnects.
      endpoints:
        - address: 'sidecar:4001'
          # Endpoints are dialled without blocking, so an unreachable one does not delay
          # startup: the node starts immediately and the failure shows up on the first RPC.
          # connectionTimeout bounds a blocking dial only, so it has no effect here.
          connectionTimeout: 10s
          # The largest message in bytes to send to and accept from this endpoint. Omit or
          # set to 0 for the default of 100 MiB -- unlike fsc.p2p.maxRecvMsgSize above, zero
          # here is not "unlimited".
          maxRecvMsgSize: 104857600
          maxSendMsgSize: 104857600
          tls:
            enabled: true
            # Fabric-x requires TLS 1.3, so minVersion here defaults to 772 rather than the
            # 771/772 (TLS 1.2-1.3) range every other dialled connection defaults to. Set it
            # explicitly to override.
            rootCAs:
              files:
              - /path/to/sidecar/ca.crt
            clientCert:
              file: /path/to/client.crt
            clientKey:
              file: /path/to/client.key
        - address: 'sidecar2:4001'
    # Fabric-x notification service: pushes transaction finality events to this node.
    # endpoints and tls take the same shape as queryService.
    notificationService:
      # Sent to the committer as the subscription timeout. Unlike queryService, a
      # non-positive value lets the committer choose its own maximum timeout.
      requestTimeout: 0s
      endpoints:
        - address: 'sidecar:4001'
      # Deadline on the context passed to one finality listener callback. Advisory:
      # a callback that ignores cancellation is not stopped. Default: 5s.
      handlerTimeout: 5s
      # How many listener callbacks may run at the same time. A callback that never
      # returns occupies one worker for good. Default: 16.
      handlerWorkers: 16
      # How many pending callbacks are buffered while all workers are busy. Default: 1000.
      handlerQueueSize: 1000
      # How long a listener waits for a notification before it is settled as Unknown.
      # Default: 2m. An explicit 0 disables this local expiry.
      listenerTTL: 2m
      # How often expired listeners are collected, so a listener lives at most
      # listenerTTL + sweepInterval. Default: 30s.
      sweepInterval: 30s

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

    # List of channels and deployed chaincode. Opening a channel not listed here is an error.
    channels:
      - name: mychannel
        # The default channel is used when a channel is requested with an empty name.
        default: true
        numRetries: 3 # number of retries on a chaincode operation failure
        retrySleep: 1s # waiting time before retry again a failed chaincode operation
        # section about the finality service
        # The remote peer probe and the local event wait run in sequence, so
        # finality.waitForEventTimeout and committer.waitForEventTimeout add up
        # rather than being alternatives: size a caller's context for the sum.
        # Both are ceilings under the caller's context, which wins when shorter.
        finality:
          waitForEventTimeout: 20s
          forPartiesWaitTimeout: 1m
          # goroutines delivering finality events to listeners. 0 or unset means 1;
          # a negative value is a startup error.
          eventQueueWorkers: 1
        # section about the committer service
        committer:
          waitForEventTimeout: 300s
          pollingTimeout: 1s
          finality:
            numRetries: 3
            unknownTxTimeout: 100ms
          parallelism: 3 # maximum go routines to commit at the same time transactions of the same block
          # Re-commit attempts for a transiently failed block, and the wait between
          # them. Transient failures are storage contention (deadlock, busy,
          # serialization failure), a lost database connection (Postgres restart or
          # failover) and an expired context. Exhausting the attempts stops this
          # channel's block stream until restart, reported as
          # fsc_fabric_core_generic_committer_commit_failures; the defaults allow
          # about a minute. A lost database connection does not use up the
          # attempts: the block is retried until the database comes back or the
          # node shuts down, waiting a random delay of up to retrySleep at first
          # and backing off to at most one minute between attempts.
          retries: 5
          retrySleep: 10s
        # section about the delivery service. The peer pushes blocks over a long-lived
        # Deliver stream that is read without a deadline, since a quiet channel
        # legitimately sends nothing. The stream is reopened only when receiving
        # fails, so a dead connection that reports no error must be detected by the
        # gRPC client keepalive.
        delivery:
          bufferSize: 1 # received blocks buffered ahead of processing (default: 1)
          # Initial reconnect delay (default: 10s). A stream that delivered a new block
          # or stayed open this long is reopened at once. Any other reconnect waits a
          # random delay below this value, doubling the bound on each further failure
          # up to 1m.
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
      persistence: mypostgres
      txidstore:
        cache:
          # TBD: What does this cache, what does 0 mean and what is the scale
          # If not specified or set to <0 it defaults to 100.
          size: 200

    # ------------------- Fabric Node resolvers -------------------------
    # The endpoint section tells how to reach other Fabric nodes in the network.
    # Entries follow the same rules as fsc.endpoint.resolvers. An entry that reuses the name of an FSC
    # resolver binds its MSP identity to the identity of that FSC node.
    endpoint:
      resolvers:
      # name of the FSC node this identity belongs to
      - name: fscnodeA
        domain:
        identity:
          # mspid of identity
          mspID: peerOrg0MSP
          # path to the public MSP (ie no crypto material) or signing cert (but I would highly recommend not specifying just the signing cert)
          path: /path/to/fscnodeA/msp
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

## Migrating TLS configuration

Every TLS setting is now resolved through one schema: nested `{file:}` / `{files:}`, plural CA
lists, and per-field inheritance from a parent `tls:` block. The old keys are **rejected at
startup** rather than translated, so a node whose configuration no longer means what it says
fails loudly instead of silently serving in the clear.

Two mechanisms reject them, and the error you see depends on where the key sat:

- A key **inside** a `tls:` block fails the strict decode of that block, naming the key and
  the block: `invalid TLS configuration under [fabric.mynet.tls]: ... has invalid keys:
  rootcertfile`. Look the key up below for its replacement.
- A key **outside** every `tls:` block is caught by an explicit check that names the
  replacement for you: `configuration key [...] has been removed; use [...] instead`.

| Removed | Replacement |
| --- | --- |
| `fsc.p2p.opts.websocket.tls.serverRootCAs.files` | `fsc.p2p.opts.websocket.tls.rootCAs.files` |
| `fsc.metrics.prometheus.tls` | `fsc.metrics.clientAuthRequired` |
| `fabric.<net>.tls.clientAuthRequired` | `fabric.<net>.tls.clientAuthEnabled` |
| `fabric.<net>.tls.serverHostOverride` | `fabric.<net>.tls.serverNameOverride` |
| `fabric.<net>.tls.rootCert.file` | `fabric.<net>.tls.rootCAs.files` |
| `fabric.<net>.tls.rootCertFile` | `fabric.<net>.tls.rootCAs.files` |
| `fabric.<net>.tls.cert`, `.key`, `.clientRootCAs` | removed; nothing read them (a network dials, it does not listen) |
| `fabric.<net>.ordering.tlsEnabled` | `fabric.<net>.tls.enabled` |
| `fabric.<net>.ordering.tlsClientAuthRequired` | `fabric.<net>.tls.clientAuthEnabled` |
| `orderers[].tlsEnabled`, `peers[].tlsEnabled` | `…[].tls.enabled` |
| `orderers[].tlsDisabled`, `peers[].tlsDisabled` | `…[].tls.enabled: false` |
| `orderers[].tlsClientSideAuth`, `peers[].tlsClientSideAuth` | `…[].tls.clientAuthEnabled` |
| `orderers[].tlsRootCertFile`, `peers[].tlsRootCertFile` | `…[].tls.rootCAs.files` |
| `orderers[].serverNameOverride`, `peers[].serverNameOverride` | `…[].tls.serverNameOverride` |
| `orderers[].tlsRootCertBytes`, `peers[].tlsRootCertBytes` | none; no YAML ever set it |
| Fabric-x `tls.clientKey`, `.clientCert` (flat strings) | `tls.clientKey.file`, `tls.clientCert.file` |
| Fabric-x `tls.rootCerts` (flat list) | `tls.rootCAs.files` |

Two of these change behaviour rather than spelling:

- **`ordering.tlsEnabled` / `ordering.tlsClientAuthRequired`** used to shadow the network block
  for orderer connections alone, so the same two settings had two homes and the narrower one
  silently won. The network `tls:` block now applies to every connection, orderers included. A
  node that relied on `ordering.tlsEnabled: false` to reach its orderers in plaintext must now
  say so with a per-orderer `tls: {enabled: false}` block.
- **Metrics** now have their own `fsc.metrics.address`. Without one, `/metrics` and `/logspec`
  stay on the `fsc.web` listener and share its TLS, as before; setting one gives them a
  listener of their own, and `fsc.metrics.tls` then applies to it.

### Other removed keys

These keys sit outside every `tls:` block and fail startup with the same
`configuration key [...] has been removed; use [...] instead` error.

| Removed | Replacement |
| --- | --- |
| `fsc.web.viewTimeout` | `fsc.view.remote.timeout`, which bounds remote view calls on `fsc.grpc` and `fsc.web` alike |
| `fsc.grpc.authentication.timeWindow` | `fsc.view.remote.timeWindow` |
| `fsc.client.certs` | `fsc.view.remote.clients` |

Two further checks stop a configuration that would otherwise silently serve no view API:
`fsc.grpc.enabled: true` without `fsc.view.remote` (nothing else uses the gRPC listener), and
any `fsc.view.remote` key without `fsc.view.remote.enabled: true`. A node that serves views
adds the block explicitly, for example `fsc.view.remote: {enabled: true, backends: [grpc]}`.

## Overriding configuration keys

Any value that is not a (grand-)child of a list can be overridden with an environment variable that is all uppercase, prefixed with `CORE_`,
and traversing the path in the yaml with underscores. This means that a key like fsc.endpoint.resolvers[0].name cannot be changed via environment variables. Examples:

```sh
CORE_LOGGING_LEVEL=debug
CORE_FSC_P2P_LISTENADDRESS=/ip4/0.0.0.0/tcp/9001
CORE_FSC_IDENTITY_KEY_FILE=/my/private.key
CORE_FSC_PERSISTENCES_MYSQLITE_OPTS_DATASOURCE=/mydb.sqlite
CORE_FSC_TRACING_OTLP_ADDRESS=jaeger.example.com:4318
CORE_FABRIC_MYNETWORK_KEEPALIVE_TIME=120s
```

A single `_` in a variable name separates two levels of the key, and `__` stands for a
literal `_` within one level. This reaches persistence names and keys that contain an
underscore:

```sh
# fsc.persistences.token_persistence.opts.dataSource
CORE_FSC_PERSISTENCES_TOKEN__PERSISTENCE_OPTS_DATASOURCE=postgres://...
# fsc.persistences.default.opts.tls.ssl_mode
CORE_FSC_PERSISTENCES_DEFAULT_OPTS_TLS_SSL__MODE=require
```

A key level that starts with `_`, other than the first, cannot be set from the environment.

A key whose value is a list of strings, such as `fsc.view.remote.backends` or
`fsc.view.remote.clients`, takes a comma-separated value; spaces around an item are
trimmed and empty items are dropped. The variable replaces the whole list from the file,
and an item cannot contain a comma:

```sh
CORE_FSC_VIEW_REMOTE_BACKENDS=grpc,web
CORE_FSC_VIEW_REMOTE_CLIENTS=/etc/fsc/alice.pem,/etc/fsc/bob.pem
```

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
To try a new sql driver, add a test next to `platform/view/services/storage/driver/sql/sqlite/sql_test.go`
and `platform/view/services/storage/driver/sql/postgres/sql_test.go`.

Each example below is a persistence block under `fsc.persistences`, selected by a store through its
`persistence` key (for example `fsc.kvs.persistence: default`).

### Config example for sqlite:

Simple:

```yaml
fsc:
  persistences:
    default:
      type: sqlite
      opts:
        dataSource: /some/path/fsc.sqlite
```


The parent directory of the data source file is created when the database is opened. FSC opens two connection pools on the
file: a write pool with exactly one connection, and a read pool for concurrent reads (see
https://kerkour.com/sqlite-for-servers for why SQLite works well this way). `maxOpenConns`, `maxIdleConns` and `maxIdleTime`
apply to the read pool only.

Advanced, more customized settings:

```yaml
fsc:
  persistences:
    default:
      type: sqlite
      opts:
        dataSource: file:/some/path/fsc.sqlite?_txlock=immediate
        tablePrefix: fsc  # optional
        skipCreateTable: true # tells FSC _not_ to create its tables at startup (because they already exist), for every store on this persistence.
        skipPragmas: true # optional, SQLite only: do not add the default pragmas below; only the pragmas in the dataSource apply.
        maxOpenConns: 20  # optional: max open read connections to the database. Defaults to unlimited. See https://go.dev/doc/database/manage-connections.
        maxIdleConns: 20  # optional: max idle read connections to the database. Defaults to 2.
        maxIdleTime: 30s  # optional: max duration a connection can be idle before it is closed. Defaults to 1 minute.
```

Unless `skipPragmas` is `true`, FSC adds these pragmas to the dataSource as `_pragma` parameters:

```
_pragma=busy_timeout(5000)
_pragma=journal_mode(WAL)
_pragma=synchronous(NORMAL)
_pragma=temp_store(memory)
_pragma=foreign_keys(ON)
```

The driver runs them on every new connection in both pools. A pragma the dataSource already sets, as `_pragma=name(...)`
or as a driver shorthand such as `_busy_timeout` or `_journal_mode`, takes precedence over the default. With
`skipPragmas: true`, set at least `_pragma=journal_mode(WAL)` and `_pragma=busy_timeout(...)`: without them, readers and
the writer block each other and fail with `SQLITE_BUSY`.

A write outside a transaction that fails with `SQLITE_BUSY` is retried up to 3 times; each attempt waits up to
`busy_timeout` for the lock. Statements inside a transaction are not retried.

### Config example for postgres

The same configuration flags as above apply, except `skipPragmas`. For Postgres we always use one connection pool for reads and writes.

> [!WARNING]
> The 'dataSource' field is sensitive because it contains a password. Instead of in this file, set it in the
> `CORE_FSC_PERSISTENCES_<NAME>_OPTS_DATASOURCE` environment variable, where `<NAME>` is the persistence name in
> uppercase and each `_` doubled (for example `CORE_FSC_PERSISTENCES_DEFAULT_OPTS_DATASOURCE`, or
> `CORE_FSC_PERSISTENCES_MY__PG_OPTS_DATASOURCE` for a persistence named `my_pg`). Stores such as `fsc.kvs` and
> `fabric.<network>.vault` only name a persistence, so they take no `opts` of their own: a variable such as
> `CORE_FSC_KVS_PERSISTENCE_OPTS_DATASOURCE` replaces the persistence name with a map, and the store fails to open. See
> [Overriding configuration keys](#overriding-configuration-keys).

```yaml
fsc:
  persistences:
    default:
      type: postgres
      opts:
        dataSource: host=localhost port=5432 user=postgres password=example dbname=tokendb sslmode=disable
        maxOpenConns: 25  # optional: max open connections to the database. Defaults to unlimited.
        maxIdleConns: 25  # optional: max idle connections to the database. Defaults to 2.
        maxIdleTime: 30s  # optional: max duration a connection can be idle before it is closed. Defaults to 1 minute.
        startupConnectTimeout: 60s  # optional: how long the node waits at startup for the database to accept connections. Defaults to 60s (also when 0); waiting cannot be disabled.
        tls:
          enabled: true              # when false (or omitted) the tls block is ignored and the dataSource is used as-is
          ssl_mode: verify-full      # optional: defaults to verify-full when empty. See the table below.
          server_name: db.example.com # optional: overrides the hostname used for verify-full verification/SNI. Defaults to the host being connected to.
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

With a multi-host `dataSource` (`postgres://u:p@pg-a:5432,pg-b:5432/db`), every host
is tried in order with the attempts its `ssl_mode` requires, and each host's
certificate is verified against that host's name unless `server_name` is set. A
unix-socket host is always connected to in plaintext. With any `ssl_mode` other
than `disable`, the `sslmode` in the `dataSource` is ignored.

> [!NOTE]
> For `verify-ca` and `verify-full`, if `root_cert_path` is omitted the server
> certificate is validated against the host's system CA pool. A server using an
> internal/self-signed CA (the common case for Postgres) will therefore fail to
> verify unless you point `root_cert_path` at that CA.

#### Failover to a new primary

To keep a node running through a Postgres primary failover, list every host that can become primary in the `dataSource`
and set `target_session_attrs=read-write`:

```yaml
fsc:
  persistences:
    default:
      type: postgres
      opts:
        dataSource: postgres://fsc:secret@pg-a:5432,pg-b:5432/fsc?target_session_attrs=read-write&connect_timeout=5
        tls:
          enabled: true
          ssl_mode: verify-full
          root_cert_path: /path/to/ca.crt
```

pgx tries the hosts in the order listed and keeps the first connection whose server accepts writes, so a new connection
succeeds only on a primary. While no host has been promoted yet, connecting fails.

A commit on a Fabric channel that runs during a failover fails with a lost connection. The committer then retries the
block until the database accepts writes again or the node shuts down: a lost connection does not count against
`committer.retries` (see the channel's `committer` section above). Broken connections are dropped from the pool and the
retry opens a new one, which goes to whichever host is primary by then. No `committer` setting needs to match the failover
time of your Postgres setup. Because the wait between these retries backs off to at most one minute, commits resume up
to a minute after the new primary is up. Only the committer retries a lost connection; any other database operation
during the failover returns the error to its caller.

The spec `fails over to a promoted Postgres standby` in `integration/fabric/faultrecovery` covers the commit path: it
kills a node's primary for longer than the node's commit retry budget, promotes a streaming-replication standby, and
checks that the blocks delivered during the outage commit without a commit failure. It connects without TLS and does
not exercise database operations outside the committer.

> [!WARNING]
> Use synchronous replication (`synchronous_commit` with `synchronous_standby_names`) for the standby that is promoted.
> With asynchronous replication, a commit the old primary acknowledged but had not yet streamed is missing after
> promotion. The node does not detect this: delivery continues after the last block it received, so the vault silently
> lacks the transactions of the lost commits. The spec waits for the standby to replay the primary's WAL before the
> primary fails, so it does not cover this case.

- Set `connect_timeout` (in seconds). A host that drops packets instead of refusing the connection, such as a crashed
  VM or a partitioned network, otherwise holds each connect until the operating system's TCP timeout before pgx tries the
  next host. An expired `connect_timeout` is a lost connection, so it does not count against `committer.retries`.
- With the `tls` block above, every host is tried and each host's certificate is verified against that host's name,
  unless `server_name` is set: then every host's certificate must match `server_name`. The unit tests
  `TestCreateTLSConnConfigMultiHost` and `TestTLSConnConfigConnectsToSecondHost` in
  `platform/view/services/storage/driver/sql/postgres` cover this.
- The database metrics label a pool with the first host of the `dataSource`, whichever host it is connected to (see
  `data_source` in [Monitoring Metrics](./platform/view/services/monitoring_metrics.md)).

Postgres stores KVS and vault keys in `BYTEA` columns. Composite keys contain U+0000, which `TEXT` cannot hold, and range scans
need keys ordered bytewise, as Fabric orders them, whatever the database collation.

On startup the vault checks the `pkey` column of its state table (the table whose name ends in `vstate`):

- `BYTEA`: nothing to do.
- `TEXT`: the vault converts the column to `BYTEA` in place. A `TEXT` column stores U+0000 as `?0?` and U+10FFFF as `?1?`; the
  conversion turns both back into their UTF-8 bytes, so every key reads back as before. A key that contains a literal `?0?` or
  `?1?` cannot be told apart from an escape and is converted too; where the two overlap, as in `?0?1?`, `?0?` is converted
  first. The conversion rewrites the table and its primary-key index under an exclusive lock. On a large vault the first
  start takes longer. Only the replica that gets the lock first converts the table.
- any other type: the vault refuses to start.

The vault then checks the primary-key column order of that state table. Every state query filters on one namespace, so the primary key is `(ns, pkey)`.

- `(ns, pkey)`: nothing to do.
- `(pkey, ns)`: the vault reorders the key to `(ns, pkey)`. Only the replica that gets the lock first reorders the table, and on a large vault the first start takes longer.
  - Postgres: the vault drops the primary-key constraint and adds it back under an exclusive lock, which rebuilds the index. The constraint name is read from the catalog, so it does not depend on the table name.
  - SQLite: the vault rebuilds the table in one transaction, copying every row while it holds the write lock. Other writers on the same file wait; a replica that waits longer than its `busy_timeout` fails to start with `SQLITE_BUSY`, and finds the table reordered when it is restarted.
- any other column order: the vault refuses to start and leaves the table unchanged.

The key-value store table (default `fsc_kvs`) uses the same primary key. Its `pkey` column is `BYTEA` on Postgres and on SQLite, so startup does not convert it. A primary key of `(pkey, ns)` is reordered to `(ns, pkey)` the same way as the vault: Postgres replaces the constraint under an exclusive lock, and SQLite rebuilds the table in one transaction. An upsert uses `ON CONFLICT (ns, pkey)`, which matches either column order, so a node that has not been upgraded keeps writing to a reordered table.

On Postgres the conversion and the reorder need a database user that owns the state table and the key-value store table.
Without that privilege the node fails to start and the table is left unchanged.

> [!WARNING]
> An FSC version that stores `pkey` as `TEXT` still starts against a converted table, but it writes escaped keys that this
> version never reads, so the two silently diverge. Stop every replica before the first upgraded one starts, never run an
> older version against a converted database, and back up the database before upgrading: downgrading means restoring the
> backup.

With `skipCreateTable: true` neither the column check, the conversion, nor the primary-key reorder runs, so a schema managed outside FSC must declare the vault state table's `pkey` as
`BYTEA` with `PRIMARY KEY (ns, pkey)`, and the key-value store table the same way. To convert an existing vault `TEXT` column by hand, run:

```sql
ALTER TABLE <prefix>_vstate ALTER COLUMN pkey TYPE BYTEA USING
  decode(replace(replace(encode(convert_to(pkey, 'UTF8'), 'hex'), '3f303f', '00'), '3f313f', 'f48fbfbf'), 'hex');
```

For more info about managing connections, see https://go.dev/doc/database/manage-connections. Fabric Smart Client
opens one connection pool per distinct `dataSource` (on Postgres, per distinct `dataSource` and `tls` block), shared by
every store and persistence that uses it, including the Fabric vault. The pool settings of the first store opened on a pool
apply to all of them. On Postgres, the sum of `maxOpenConns` over the distinct data sources, across all replicas, should not
exceed the server's `max_connections` (100 by default). SQLite opens two pools per data source: one for reads, limited by
`maxOpenConns`, and one with a single connection for writes.

### Postgres errors

The Postgres driver maps server and connection errors onto the sentinels in
`platform/view/services/storage/driver`, so callers match them with `errors.Is`:

- `ErrConnectionLost`: a dropped or closed connection, a network error, an expired `connect_timeout`, any other failed
  connect without a SQLSTATE (including DNS and TLS failures), and SQLSTATE `08000`, `08001`, `08003`, `08006`, `08007`,
  `57P01`, `57P02`, `57P03` and `53300` (too_many_connections). The Fabric committer retries a block that fails with it
  until the database is back (see [Failover to a new primary](#failover-to-a-new-primary) and `commit_retries` in
  [monitoring](platform/view/services/monitoring_metrics.md#fabric-committer)).
- `UniqueKeyViolation` (`23505`), `SqlBusy` (`40001`), `DeadlockDetected` (`40P01`).

Any other SQLSTATE, including one the server returns while connecting such as `28P01` (wrong password) or `3D000` (missing
database), is returned unchanged, unless another host of a multi-host `dataSource` reported a code that maps to
`ErrConnectionLost`. The committer does not retry it and degrades the channel. At startup the node instead
retries every failed ping for `startupConnectTimeout`, then fails.
