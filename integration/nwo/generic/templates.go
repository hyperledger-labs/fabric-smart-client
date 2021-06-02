/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

const DefaultCryptoTemplate = `---
{{ with $w := . -}}
PeerOrgs:{{ range .PeerOrgs }}
- Name: {{ .Name }}
  Domain: {{ .Domain }}
  EnableNodeOUs: {{ .EnableNodeOUs }}
  {{- if .CA }}
  CA:{{ if .CA.Hostname }}
    hostname: {{ .CA.Hostname }}
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
  {{- end }}
  {{- end }}
  Users:
    Count: {{ .Users }}
    {{- if len .UserNames }}
    Names: {{ range .UserNames }}
    - {{ . }}
    {{- end }}
    {{- end }}

  Specs:{{ range $w.PeersInOrg .Name }}
  - Hostname: {{ .Name }}
    SANS:
    - localhost
    - 127.0.0.1
    - ::1
  {{- end }}
{{- end }}
{{- end }}
`

const DefaultViewExtensionTemplate = `
generic:
  enabled: true
  identity:
    cert:
      file: {{ .PeerLocalMSPIdentityCert Peer }}
    key:
      file: {{ .PeerLocalMSPPrivateKey Peer }}
  endpoint:
    resolves: {{ range .Resolvers }}
    - name: {{ .Name }}
      domain: {{ .Domain }}
      identity:
        id: {{ .Identity.ID }}
        path: {{ .Identity.Path }}
      addresses: {{ range $key, $value := .Addresses }}
         {{ $key }}: {{ $value }} 
      {{- end }}
  {{- end }}
`
