/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package topology

const DefaultOrdererTemplate = `---
{{ with $w := . -}}
General:
  ListenAddress: 0.0.0.0
  ListenPort: {{ .OrdererPort Orderer "Listen" }}
  TLS:
    Enabled: true
    PrivateKey: {{ $w.OrdererLocalTLSDir Orderer }}/server.key
    Certificate: {{ $w.OrdererLocalTLSDir Orderer }}/server.crt
    RootCAs:
    -  {{ $w.OrdererLocalTLSDir Orderer }}/ca.crt
    ClientAuthRequired: {{ $w.ClientAuthRequired }}
    ClientRootCAs:
  Cluster:
    ClientCertificate: {{ $w.OrdererLocalTLSDir Orderer }}/server.crt
    ClientPrivateKey: {{ $w.OrdererLocalTLSDir Orderer }}/server.key
    ServerCertificate: {{ $w.OrdererLocalTLSDir Orderer }}/server.crt
    ServerPrivateKey: {{ $w.OrdererLocalTLSDir Orderer }}/server.key
    DialTimeout: 5s
    RPCTimeout: 7s
    ReplicationBufferSize: 20971520
    ReplicationPullTimeout: 5s
    ReplicationRetryTimeout: 5s
    ListenAddress: 0.0.0.0
    ListenPort: {{ .OrdererPort Orderer "Cluster" }}
  Keepalive:
    ServerMinInterval: 60s
    ServerInterval: 7200s
    ServerTimeout: 20s
  BootstrapMethod: file
  BootstrapFile: {{ $w.OrdererBootstrapFile }}
  LocalMSPDir: {{ $w.OrdererLocalMSPDir Orderer }}
  LocalMSPID: {{ ($w.Organization Orderer.Organization).MSPID }}
  Profile:
    Enabled: false
    Address: {{ .OrdererAddress Orderer "Profile" }}
  BCCSP:
    Default: SW
    SW:
      Hash: SHA2
      Security: 256
      FileKeyStore:
        KeyStore:
  Authentication:
    TimeWindow: 15m
FileLedger:
  Location: {{ .OrdererDir Orderer }}/system
Debug:
  BroadcastTraceDir:
  DeliverTraceDir:
Consensus:
  WALDir: {{ .OrdererDir Orderer }}/etcdraft/wal
  SnapDir: {{ .OrdererDir Orderer }}/etcdraft/snapshot
  EvictionSuspicion: 10s
Operations:
  ListenAddress: {{ .OrdererAddress Orderer "Operations" }}
  TLS:
    Enabled: false
    PrivateKey: {{ $w.OrdererLocalTLSDir Orderer }}/server.key
    Certificate: {{ $w.OrdererLocalTLSDir Orderer }}/server.crt
    RootCAs:
    -  {{ $w.OrdererLocalTLSDir Orderer }}/ca.crt
    ClientAuthRequired: {{ $w.ClientAuthRequired }}
    ClientRootCAs:
    -  {{ $w.OrdererLocalTLSDir Orderer }}/ca.crt
Metrics:
  Provider: {{ .MetricsProvider }}
  Statsd:
    Network: udp
    Address: {{ if .StatsdEndpoint }}{{ .StatsdEndpoint }}{{ else }}0.0.0.0:8125{{ end }}
    WriteInterval: 5s
    Prefix: {{ ReplaceAll (ToLower Orderer.ID) "." "_" }}
{{- end }}
`
