/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

const configTxTemplate = `---
{{ with $w := . -}}
Organizations:{{ range .PeerOrgs }}
- &{{ .MSPID }}
  Name: {{ .Name }}
  ID: {{ .MSPID }}
  MSPDir: {{ $w.PeerOrgMSPDir . }}
  Policies:
    {{- if .EnableNodeOUs }}
    Readers:
      Type: Signature
      Rule: OR('{{.MSPID}}.admin', '{{.MSPID}}.peer', '{{.MSPID}}.client')
    Writers:
      Type: Signature
      Rule: OR('{{.MSPID}}.admin', '{{.MSPID}}.client')
    Endorsement:
      Type: Signature
      Rule: OR('{{.MSPID}}.peer')
    Admins:
      Type: Signature
      Rule: OR('{{.MSPID}}.admin')
    {{- else }}
    Readers:
      Type: Signature
      Rule: OR('{{.MSPID}}.member')
    Writers:
      Type: Signature
      Rule: OR('{{.MSPID}}.member')
    Endorsement:
      Type: Signature
      Rule: OR('{{.MSPID}}.member')
    Admins:
      Type: Signature
      Rule: OR('{{.MSPID}}.admin')
    {{- end }}
  AnchorPeers:{{ range $w.AnchorsInOrg .Name }}
  - Host: {{ $w.PeerHost . }}
    Port: {{ $w.PeerPort . "Listen" }}
  {{- end }}
{{- end }}
{{- range .IdemixOrgs }}
- &{{ .MSPID }}
  Name: {{ .Name }}
  ID: {{ .MSPID }}
  MSPDir: {{ $w.IdemixOrgMSPDir . }}
  MSPType: idemix
  Policies:
    {{- if .EnableNodeOUs }}
    Readers:
      Type: Signature
      Rule: OR('{{.MSPID}}.admin', '{{.MSPID}}.peer', '{{.MSPID}}.client')
    Writers:
      Type: Signature
      Rule: OR('{{.MSPID}}.admin', '{{.MSPID}}.client')
    Endorsement:
      Type: Signature
      Rule: OR('{{.MSPID}}.peer')
    Admins:
      Type: Signature
      Rule: OR('{{.MSPID}}.admin')
    {{- else }}
    Readers:
      Type: Signature
      Rule: OR('{{.MSPID}}.member')
    Writers:
      Type: Signature
      Rule: OR('{{.MSPID}}.member')
    Endorsement:
      Type: Signature
      Rule: OR('{{.MSPID}}.member')
    Admins:
      Type: Signature
      Rule: OR('{{.MSPID}}.admin')
    {{- end }}
{{ end }}
{{- range .OrdererOrgs }}
- &{{ .MSPID }}
  Name: {{ .Name }}
  ID: {{ .MSPID }}
  MSPDir: {{ $w.OrdererOrgMSPDir . }}
  Policies:
  {{- if .EnableNodeOUs }}
    Readers:
      Type: Signature
      Rule: OR('{{.MSPID}}.admin', '{{.MSPID}}.orderer', '{{.MSPID}}.client')
    Writers:
      Type: Signature
      Rule: OR('{{.MSPID}}.admin', '{{.MSPID}}.orderer', '{{.MSPID}}.client')
    Admins:
      Type: Signature
      Rule: OR('{{.MSPID}}.admin')
  {{- else }}
    Readers:
      Type: Signature
      Rule: OR('{{.MSPID}}.member')
    Writers:
      Type: Signature
      Rule: OR('{{.MSPID}}.member')
    Admins:
      Type: Signature
      Rule: OR('{{.MSPID}}.admin')
  {{- end }}
  OrdererEndpoints:
  {{- range $i, $v := $w.OrderersInOrg .Name }}
  - {{printf "id=%d,broadcast,%s" $i ($w.OrdererAddress $v "Listen")}}
  - {{printf "id=%d,deliver,%s" $i ($w.OrdererAddress $v "Listen")}}
  {{- end }}
{{ end }}

Channel: &ChannelDefaults
  Capabilities:
    V2_0: true
  Policies: &DefaultPolicies
    Readers:
      Type: ImplicitMeta
      Rule: ANY Readers
    Writers:
      Type: ImplicitMeta
      Rule: ANY Writers
    Admins:
      Type: ImplicitMeta
      Rule: MAJORITY Admins

Profiles:{{ range .Profiles }}
  {{ .Name }}:
    {{- if .ChannelCapabilities}}
    Capabilities:{{ range .ChannelCapabilities}}
      {{ . }}: true
    {{- end}}
    Policies:
      <<: *DefaultPolicies
    {{- else }}
    <<: *ChannelDefaults
    {{- end}}
    {{- if .Orderers }}
    Orderer:
      OrdererType: {{ $w.Consensus.Type }}
      {{- if .Blocks}}
      BatchTimeout: {{ .Blocks.BatchTimeout }}s
      BatchSize:
        MaxMessageCount: {{ .Blocks.MaxMessageCount }}
        AbsoluteMaxBytes: {{ .Blocks.AbsoluteMaxBytes }} MB
        PreferredMaxBytes: {{ .Blocks.PreferredMaxBytes }} KB
      {{- else }}
      BatchTimeout: 1s
      BatchSize:
        MaxMessageCount: 1
        AbsoluteMaxBytes: 98 MB
        PreferredMaxBytes: 2 MB
      {{- end}}
	  {{- if eq $w.Consensus.Type "arma" }}
      Arma:
        Path: path/to/shared_config.binpb
      {{- end}}
      Capabilities:
        V2_0: true
      {{- if eq $w.Consensus.Type "BFT" }}
      {{- if .SmartBFT}}
      SmartBFT:
        RequestBatchMaxCount:      100
        RequestBatchMaxBytes:      10485760
        RequestBatchMaxInterval:   50ms
        IncomingMessageBufferSize: 200
        RequestPoolSize:           400
        RequestForwardTimeout:     2s
        RequestComplainTimeout:    20s
        RequestAutoRemoveTimeout:  3m
        ViewChangeResendInterval:  5s
        ViewChangeTimeout:         20s
        LeaderHeartbeatTimeout:    {{ .SmartBFT.LeaderHeartbeatTimeout }}s
        LeaderHeartbeatCount:      {{ .SmartBFT.LeaderHeartbeatCount }}
        CollectTimeout:            1s
        SyncOnStart:               false
        SpeedUpViewChange:         false
      {{- end }}
      ConsenterMapping:{{ range $index, $orderer := .Orderers }}{{ with $w.Orderer . }}
      - ID: {{ .Id }}
        {{ $w.OrdererHost . }}
        Port: {{ $w.OrdererPort . "Cluster" }}
        MSPID: {{ ($w.Organization .Organization).MSPID}}
        ClientTLSCert: {{ $w.OrdererLocalCryptoDir . "tls" }}/server.crt
        ServerTLSCert: {{ $w.OrdererLocalCryptoDir . "tls" }}/server.crt
        Identity: {{ $w.OrdererSignCert .}}
        {{- end }}{{- end }}
      {{- end }}
      {{- if eq $w.Consensus.Type "etcdraft" }}
      EtcdRaft:
        Options:
          TickInterval: 500ms
          SnapshotIntervalSize: 1 KB
        Consenters:{{ range .Orderers }}{{ with $w.Orderer . }}
        - Host: 127.0.0.1
          Port: {{ $w.OrdererPort . "Cluster" }}
          ClientTLSCert: {{ $w.OrdererLocalCryptoDir . "tls" }}/server.crt
          ServerTLSCert: {{ $w.OrdererLocalCryptoDir . "tls" }}/server.crt
        {{- end }}{{- end }}
      {{- end }}
      Organizations:{{ range $w.OrgsForOrderers .Orderers }}
      - *{{ .MSPID }}
      {{- end }}
      Policies:
        Readers:
          Type: ImplicitMeta
          Rule: ANY Readers
        Writers:
          Type: ImplicitMeta
          Rule: ANY Writers
        Admins:
          Type: ImplicitMeta
          Rule: MAJORITY Admins
        BlockValidation:
          Type: ImplicitMeta
          Rule: ANY Writers
    {{- end }}
    Application:
      Capabilities:
      {{- if .AppCapabilities }}{{ range .AppCapabilities }}
        {{ . }}: true
        {{- end }}
      {{- else }}
        V1_3: true
      {{- end }}
      Organizations:{{ range .Organizations }}
      - *{{ ($w.Organization .).MSPID }}
      {{- end}}
      MetaNamespaceVerificationKeyPath: {{ MetaNamespaceVerificationKeyPath }}
      Policies:
      {{- if .Policies }}{{ range .Policies }} 
        {{ .Name }}:
          Type: {{ .Type }}
          Rule: {{ .Rule }}
      {{- end }}
      {{- else }}
        Readers:
          Type: ImplicitMeta
          Rule: ANY Readers
        Writers:
          Type: ImplicitMeta
          Rule: ANY Writers
        Admins:
          Type: ImplicitMeta
          Rule: MAJORITY Admins
        LifecycleEndorsement:
          Type: ImplicitMeta
          Rule: "MAJORITY Endorsement"
        Endorsement:
          Type: ImplicitMeta
          Rule: "MAJORITY Endorsement"
    {{- end }}
    Consortium: {{ .Consortium }}
{{- end }}
{{ end }}
`
