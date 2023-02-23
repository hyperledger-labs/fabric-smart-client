/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package topology

const DefaultCryptoTemplate = `---
{{ with $w := . -}}
OrdererOrgs:{{ range .OrdererOrgs }}
- Name: {{ .Name }}
  Domain: {{ .Domain }}
  EnableNodeOUs: {{ .EnableNodeOUs }}
  {{- if .CA }}
  CA:{{ if .CA.Hostname }}
    Hostname: {{ .CA.Hostname }}
  {{- end -}}
  {{- end }}
  Specs:{{ range $w.OrderersInOrg .Name }}
  - Hostname: {{ .Name }}
    SANS:
    - {{ $w.OrdererHost . }}
    - 0.0.0.0
    - localhost
    - 127.0.0.1
    - host.docker.internal
    - fabric
    - ::1
  {{- end }}
{{- end }}

PeerOrgs:{{ range .PeerOrgs }}
- Name: {{ .Name }}
  Domain: {{ .Domain }}
  EnableNodeOUs: {{ .EnableNodeOUs }}
  {{- if .CA }}
  CA:{{ if .CA.Hostname }}
    hostname: {{ .CA.Hostname }}
    SANS:
    - 0.0.0.0
    - localhost
    - 127.0.0.1
    - ::1
  {{- end }}
  {{- end }}
  Users:
    Count: {{ .Users }}
    {{- if len .UserSpecs }}
    Specs: {{ range .UserSpecs }}
    - Name: {{ .Name }}
      HSM: {{ .HSM }}
    {{- end }}
    {{- end }}

  Specs:{{ range $w.PeersInOrg .Name }}
  - Hostname: {{ .Name }}
    SANS:
    - {{ $w.PeerHost . }}
    - 0.0.0.0
    - localhost
    - 127.0.0.1
    - ::1
    - fabric
    - host.docker.internal
  {{- end }}
{{- end }}
{{- end }}
`
