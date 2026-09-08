/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"fmt"
	"text/template"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
)

const qsExtensionTemplate = `
fabric:
  {{ .NetworkName }}:
    queryService:
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

func generateQSExtension(n *network.Network, sidecarOrg, sidecarName string) {
	scPeer := n.Peer(sidecarOrg, sidecarName)
	scAddr := fmt.Sprintf("127.0.0.1:%d", n.PeerPort(scPeer, network.QueryServicePortName))
	generateExtensions(n, scAddr, template.Must(template.New("qs").Parse(qsExtensionTemplate)))
}
