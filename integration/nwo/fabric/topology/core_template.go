/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package topology

const DefaultCoreTemplate = `---
logging:
  spec: {{ .Logging.Spec }} 
  format: {{ .Logging.Format }}

peer:
  id: {{ Peer.ID }}
  networkId: {{ .NetworkID }}
  address: {{ .PeerAddress Peer "Listen" }}
  addressAutoDetect: true
  listenAddress: {{ .PeerAddress Peer "Listen" }}
  chaincodeListenAddress: 0.0.0.0:{{ .PeerPort Peer "Chaincode" }}
  keepalive:
    minInterval: 60s
    client:
      interval: 60s
      timeout: 600s
    deliveryClient:
      interval: 60s
      timeout: 20s
  gossip:
    bootstrap: {{ .PeerAddress Peer "Listen" }}
    endpoint: {{ .PeerAddress Peer "Listen" }}
    externalEndpoint: {{ .PeerAddress Peer "Listen" }}
    useLeaderElection: false
    orgLeader: true
    membershipTrackerInterval: 5s
    maxBlockCountToStore: 100
    maxPropagationBurstLatency: 10ms
    maxPropagationBurstSize: 10
    propagateIterations: 1
    propagatePeerNum: 3
    pullInterval: 4s
    pullPeerNum: 3
    requestStateInfoInterval: 4s
    publishStateInfoInterval: 4s
    stateInfoRetentionInterval:
    publishCertPeriod: 10s
    dialTimeout: 3s
    connTimeout: 2s
    recvBuffSize: 20
    sendBuffSize: 200
    digestWaitTime: 1s
    requestWaitTime: 1500ms
    responseWaitTime: 2s
    aliveTimeInterval: 5s
    aliveExpirationTimeout: 25s
    reconnectInterval: 25s
    election:
      startupGracePeriod: 15s
      membershipSampleInterval: 1s
      leaderAliveThreshold: 10s
      leaderElectionDuration: 5s
    pvtData:
      pullRetryThreshold: 7s
      transientstoreMaxBlockRetention: 20000
      pushAckTimeout: 3s
      btlPullMargin: 10
      reconcileBatchSize: 10
      reconcileSleepInterval: 10s
      reconciliationEnabled: true
      skipPullingInvalidTransactionsDuringCommit: false
      implicitCollectionDisseminationPolicy:
        requiredPeerCount: 0
        maxPeerCount: 1
    state:
       enabled: true
       checkInterval: 10s
       responseTimeout: 3s
       batchSize: 10
       blockBufferSize: 100
       maxRetries: 3
  events:
    address: {{ .PeerAddress Peer "Events" }}
    buffersize: 100
    timeout: 10ms
    timewindow: 15m
    keepalive:
      minInterval: 60s
  tls:
    enabled: {{ .TLSEnabled }}
    clientAuthRequired: {{ .ClientAuthRequired }}
    cert:
      file: {{ .PeerLocalTLSDir Peer }}/server.crt
    key:
      file: {{ .PeerLocalTLSDir Peer }}/server.key
    clientCert:
      file: {{ .PeerLocalTLSDir Peer }}/server.crt
    clientKey:
      file: {{ .PeerLocalTLSDir Peer }}/server.key
    rootcert:
      file: {{ .PeerLocalTLSDir Peer }}/ca.crt
    clientRootCAs:
      files:
      - {{ .PeerLocalTLSDir Peer }}/ca.crt
  authentication:
    timewindow: 15m
  fileSystemPath: filesystem
  BCCSP:
    Default: SW
    SW:
      Hash: SHA2
      Security: 256
      FileKeyStore:
        KeyStore:
  mspConfigPath: {{ .PeerLocalMSPDir Peer }}
  localMspId: {{ (.Organization Peer.Organization).MSPID }}
  deliveryclient:
    reconnectTotalTimeThreshold: 3600s
    policy: {{ .PeerDeliveryClientPolicy }}
  localMspType: bccsp
  profile:
    enabled:     false
    listenAddress: {{ .PeerAddress Peer "ProfilePort" }}
  handlers:
    authFilters:
    - name: DefaultAuth
    - name: ExpirationCheck
    - name: TimeWindowCheck
    decorators:
    - name: DefaultDecorator
    endorsers:
      escc:
        name: DefaultEndorsement
    validators:
      vscc:
        name: DefaultValidation
  validatorPoolSize:
  discovery:
    enabled: true
    authCacheEnabled: true
    authCacheMaxSize: 1000
    authCachePurgeRetentionRatio: 0.75
    orgMembersAllowedAccess: false
  limits:
    concurrency:
      endorserService: 100
      deliverService: 100
  gateway:
    enabled: {{ .GatewayEnabled }}

vm:
  endpoint: unix:///var/run/docker.sock
  docker:
    tls:
      enabled: false
      ca:
        file: docker/ca.crt
      cert:
        file: docker/tls.crt
      key:
        file: docker/tls.key
    attachStdout: true
    hostConfig:
      NetworkMode: host
      LogConfig:
        Type: json-file
        Config:
          max-size: "50m"
          max-file: "5"
      Memory: 2147483648

chaincode:
  builder: $(DOCKER_NS)/fabric-ccenv:latest
  pull: false
  golang:
    runtime: $(DOCKER_NS)/fabric-baseos:latest
    dynamicLink: false
  java:
    runtime: $(DOCKER_NS)/fabric-javaenv:latest
  node:
    runtime: $(DOCKER_NS)/fabric-nodeenv:latest
  installTimeout: 1200s
  startuptimeout: 1200s
  executetimeout: 1200s
  mode: net
  keepalive: 0
  system:
    _lifecycle: enable
    cscc:       enable
    lscc:       enable
    qscc:       enable
  runtimeParams:
    useWriteBatch: {{ .UseWriteBatch }}
    maxSizeWriteBatch: 1000
    useGetMultipleKeys: {{ .UseGetMultipleKeys }}
    maxSizeGetMultipleKeys: 1000
  logging:
    level:  info
    shim:   warning
    format: '%{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}'
  externalBuilders: {{ range .ExternalBuilders }}
    - path: {{ .Path }}
      name: {{ .Name }}
      propagateEnvironment: {{ range .PropagateEnvironment }}
         - {{ . }}
      {{- end }}
  {{- end }}

ledger:
  blockchain:
  state:
    stateDatabase: goleveldb
    couchDBConfig:
      couchDBAddress: 127.0.0.1:5984
      username:
      password:
      maxRetries: 3
      maxRetriesOnStartup: 10
      requestTimeout: 35s
      queryLimit: 10000
      maxBatchUpdateSize: 1000
  history:
    enableHistoryDatabase: true
  pvtdataStore:
    deprioritizedDataReconcilerInterval: 60m
    purgeInterval: 1

operations:
  listenAddress: {{ .PeerAddress Peer "Operations" }}
  tls:
    enabled: {{ .TLSEnabled }}
    cert:
      file: {{ .PeerLocalTLSDir Peer }}/server.crt
    key:
      file: {{ .PeerLocalTLSDir Peer }}/server.key
    clientAuthRequired: {{ .ClientAuthRequired }}
    clientRootCAs:
      files:
      - {{ .PeerLocalTLSDir Peer }}/ca.crt
metrics:
  provider: {{ .MetricsProvider }}
  statsd:
    {{- if .StatsdEndpoint }}
    network: tcp
    address: {{ .StatsdEndpoint }}
    {{- else }}
    network: udp
    address: 127.0.0.1:8125
    {{- end }}
    writeInterval: 5s
    prefix: {{ ReplaceAll (ToLower Peer.ID) "." "_" }}
`

const DefaultFSCFabricExtensionTemplate = `
fabric:
  enabled: true
  {{ FabricName }}:
    default: {{ DefaultNetwork }}
    driver: {{ Driver }}
    mspCacheSize: 500
    defaultMSP: {{ Peer.DefaultIdentity }}
    msps: {{ range Peer.Identities }}
      - id: {{ .ID }}
        mspType: {{ .MSPType }}
        mspID: {{ .MSPID }}
        cacheSize: {{ .CacheSize }}
        path: {{ .Path }}
        opts:
          BCCSP:
            Default: {{ .Opts.Default }}
            # Settings for the SW crypto provider (i.e. when DEFAULT: SW)
            SW:
               Hash: {{ .Opts.SW.Hash }}
               Security: {{ .Opts.SW.Security }}
            # Settings for the PKCS#11 crypto provider (i.e. when DEFAULT: PKCS11)
            PKCS11:
               # Location of the PKCS11 module library
               Library: {{ .Opts.PKCS11.Library }}
               # Token Label
               Label: {{ .Opts.PKCS11.Label }}
               # User PIN
               Pin: {{ .Opts.PKCS11.Pin }}
               Hash: {{ .Opts.PKCS11.Hash }}
               Security: {{ .Opts.PKCS11.Security }}
    {{- end }}
    tls:
      enabled:  {{ TLSEnabled }}
      clientAuthRequired: {{ .ClientAuthRequired }}
      {{- if .ClientAuthRequired }}
      clientCert:
        file: {{ .PeerLocalTLSDir Peer }}/server.crt
      clientKey:
        file: {{ .PeerLocalTLSDir Peer }}/server.key
      {{- end }} 
    keepalive:
      interval: 60s
      timeout: 600s
      connectionTimeout: 10s
    ordering:
      numRetries: 3
      retryInterval: 3s
      tlsEnabled: {{ TLSEnabled }}
    peers: {{ range Peers }}
      - address: {{ PeerAddress . "Listen" }}
        connectionTimeout: 10s        
        tlsRootCertFile: {{ CACertsBundlePath }}
        serverNameOverride:
        {{ if .TLSDisabled }}tlsDisabled: {{ .TLSDisabled }} {{ end }}
        {{ if .Usage }}usage: {{ .Usage }} {{ end }}
    {{- end }}
    channels: {{ range .Channels }}
      - name: {{ .Name }}
        default: {{ .Default }}
        numRetries: 3
        retrySleep: 1s
        delivery:
          bufferSize: 300
        committer:
          waitForEventTimeout: 300s
          pollingTimeout: 100ms
          parallelism: 5
        chaincodes: {{range Chaincodes .Name }}
          - name: {{ .Chaincode.Name }}
            private: {{ .Private }}
        {{- end }}
    {{- end }}
    vault:
      txidstore:
        cache:
          # Sets the maximum number of cached items 
          size: 200
    endpoint:
      resolvers: {{ range .Resolvers }}
      - name: {{ .Name }}
        domain: {{ .Domain }}
        identity:
          mspID: {{ .Identity.MSPID }}
          path: {{ .Identity.Path }}
        addresses: {{ range $key, $value := .Addresses }}
           {{ $key }}: {{ $value }} 
        {{- end }}
        aliases: {{ range .Aliases }}
        - {{ . }} 
        {{- end }}
    {{- end }}
`
