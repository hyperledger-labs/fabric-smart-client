/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

const RoutingTemplate = `---
routes:
{{- range $key, $value := .Routing }}
  {{ $key }}:
    {{- range $value }}
    - {{ . }}
    {{- end }}
{{- end }}
`
