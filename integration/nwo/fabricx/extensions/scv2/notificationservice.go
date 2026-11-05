/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"fmt"
	"text/template"

	fabric_network "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
)

const nsExtensionTemplate = `
fabric:
  {{ .NetworkName }}:
    notificationService:
      requestTimeout: {{ .RequestTimeout }}
      endpoints:{{- range .Endpoints }}
        - address: {{ .Address }}
          connectionTimeout: {{ .ConnectionTimeout }}
          {{- if .TLSEnabled }}
          tls:
            enabled: true
            rootCAs:
              files:{{- range .RootCertPaths }}
                - {{ . }}
              {{- end }}
            {{- if .ClientKeyPath }}
            clientKey:
              file: {{ .ClientKeyPath }}
            clientCert:
              file: {{ .ClientCertPath }}
            {{- end }}
          {{- end }}
    {{- end }}
`

func generateNSExtension(e *Extension) {
	generateExtensions(e, func(c *committer) string {
		return fmt.Sprintf("127.0.0.1:%d", e.network.PeerPort(c.sidecarPeer(), fabric_network.ListenPort))
	}, template.Must(template.New("ns").Parse(nsExtensionTemplate)))
}
