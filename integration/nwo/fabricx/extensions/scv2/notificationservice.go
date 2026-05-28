/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"fmt"
	"text/template"

	fabric_network "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
)

const nsExtensionTemplate = `
fabric:
  {{ .NetworkName }}:
    notificationService:
      requestTimeout: {{ .RequestTimeout }}
      endpoints:{{- range .Endpoints }}
        - address: {{ .Address }}
          connectionTimeout: {{ .ConnectionTimeout }}
          {{- if .TLS}}
          tls:
            enabled: {{ .TLS.Enabled }}
            {{- if .TLS.Enabled }}
            rootCerts:{{- range .TLS.RootCertPaths }}
              - {{ . }}
            {{- end }}
            {{- if .TLS.ClientKeyPath }}
            clientKey: {{ .TLS.ClientKeyPath }}
            {{- end }}
            {{- if .TLS.ClientCertPath }}
            clientCert: {{ .TLS.ClientCertPath }}
            {{- end }}
            {{- end }}
          {{- end }}
    {{- end }}
`

func generateNSExtension(n *network.Network, sidecarOrg, sidecarName string) {
	scPeer := n.Peer(sidecarOrg, sidecarName)
	scAddr := fmt.Sprintf("127.0.0.1:%d", n.PeerPort(scPeer, fabric_network.ListenPort))
	generateExtensions(n, scAddr, template.Must(template.New("ns").Parse(nsExtensionTemplate)))
}
