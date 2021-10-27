/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

const ExtensionTemplate = `
orion:
  enabled: true
  {{ Name }}:
    server:
      id: {{ ServerID }}
      url: {{ ServerURL }}
      ca: {{ CACertPath }}
    identities: {{ range Identities }}
      - name: {{ .Name }}
        cert: {{ .Cert }}
        key: {{ .Key }}
	{{- end }}
`
