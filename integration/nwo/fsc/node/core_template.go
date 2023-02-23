/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

const CoreTemplate = `---
# Logging section
logging:
 # Spec
 spec: {{ Topology.Logging.Spec }}
fsc:
  # The FSC id provides a name for this node instance and is used when
  # naming docker resources.
  id: {{ Peer.ID }}
  # Identity of this node, used to connect to other nodes
  identity:
    # X.509 certificate used as identity of this node
    cert:
      file: {{ .NodeLocalCertPath Peer }}
    # Private key matching the X.509 certificate
    key:
      file: {{ .NodeLocalPrivateKeyPath Peer }}
  # Client X.509 certificates
  client:
    certs:
    {{- range Peer.Admins }}
    - {{ . }} 
    {{- end }}
  grpc:
    enabled: true
    # This represents the endpoint to other FSC nodes in the same organization.
    address: 0.0.0.0:{{ .NodePort Peer "Listen" }}
    # TLS Settings
    # (We use here the same set of properties as Hyperledger Fabric)
    tls:
      # Require server-side TLS
      enabled:  true
      # Require client certificates / mutual TLS for inbound connections.
      # Note that clients that are not configured to use a certificate will
      # fail to connect to the node.
      clientAuthRequired: {{ .ClientAuthRequired }}
      # X.509 certificate used for TLS server
      cert:
        file: {{ .NodeLocalTLSDir Peer }}/server.crt
      # Private key used for TLS server
      key:
        file: {{ .NodeLocalTLSDir Peer }}/server.key
      # If mutual TLS is enabled, clientRootCAs.files contains a list of additional root certificates
      # used for verifying certificates of client connections.
      {{- if .ClientAuthRequired }}
      clientRootCAs:
        files:
        - {{ .NodeLocalTLSDir Peer }}/ca.crt
      {{- end }}
    # Keepalive settings for node server and clients
    keepalive:
      # MinInterval is the minimum permitted time between client pings.
      # If clients send pings more frequently, the peer server will
      # disconnect them
      minInterval: 60s
      # Interval is the duration after which if the server does not see
      # any activity from the client it pings the client to see if it's alive
      interval: 300s
      # Timeout is the duration the server waits for a response
      # from the client after sending a ping before closing the connection
      timeout: 600s
  # P2P configuration
  p2p:
    # Listening address
    listenAddress: /ip4/0.0.0.0/tcp/{{ .NodePort Peer "P2P" }}
    # If empty, this is a P2P boostrap node. Otherwise, it contains the name of the FCS node that is a bootstrap node
    bootstrapNode: {{ .BootstrapNode Peer }}
  # The Key-Value Store is used to store various information related to the FSC node
  kvs:
    persistence:
      # Persistence type can be \'badger\' (on disk) or \'memory\'
      type: {{ NodeKVSPersistenceType }}
      opts:
        {{- if eq NodeKVSPersistenceType "orion" }}
        network: {{ KVSOrionNetwork }}
        database: {{ KVSOrionDatabase }}
        creator: {{ KVSOrionCreator }}
        {{- else }}
        path: {{ NodeKVSPath }}
        SyncWrites: false
        {{- end }}
    cache:
        # Sets the maximum number of cached items 
        size: 200
  # HTML Server configuration for REST calls
  web:
    enabled: true
    # HTTPS server listener address
    address: 0.0.0.0:{{ .NodePort Peer "Web" }}
    tls:
      enabled:  true
      cert:
        file: {{ .NodeLocalTLSDir Peer }}/server.crt
      key:
        file: {{ .NodeLocalTLSDir Peer }}/server.key
      # Require client certificates / mutual TLS for inbound connections.
      # Note that clients that are not configured to use a certificate will
      # fail to connect to the node.
      clientAuthRequired: false
      # If mutual TLS is enabled, clientRootCAs.files contains a list of additional root certificates
      # used for verifying certificates of client connections.
      clientRootCAs:
        files:
        - {{ .NodeLocalTLSDir Peer }}/ca.crt
  tracing:
    provider: {{ Topology.TracingProvider }}
    optl:
      address: 127.0.0.1:4319
  metrics:
    # metrics provider is one of statsd, prometheus, or disabled
    provider: {{ Topology.MetricsProvider }}
    # statsd configuration
    statsd:
      # network type: tcp or udp
      network: udp
      # statsd server address
      address: 127.0.0.1:8125
      # the interval at which locally cached counters and gauges are pushed
      # to statsd; timings are pushed immediately
      writeInterval: 10s
      # prefix is prepended to all emitted statsd metrics
      prefix:

  # The endpoint section tells how to reach other FSC node in the network.
  # For each node, the name, the domain, the identity of the node, and its addresses must be specified.
  endpoint:
    resolvers: {{ range Resolvers }}
    - name: {{ .Name }}
      domain: {{ .Domain }}
      identity:
        path: {{ .Identity.Path }}
      addresses: {{ range $key, $value := .Addresses }}
         {{ $key }}: {{ $value }} 
      {{- end }}
      aliases: {{ range .Aliases }}
        - {{ . }} 
      {{- end }}
  {{- end }}

{{ range Extensions }}
{{.}}
{{- end }}
`
