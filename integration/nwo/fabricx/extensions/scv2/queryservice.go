/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"text/template"
	"time"

	api2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

// generateQSExtension adds the query service endpoint configuration to the core.yaml
// of every FSC node in the network topology.
// When TLS is enabled, each FSC node gets its own extension with mTLS client
// credentials taken from the fabric peer's TLS directory.
func generateQSExtension(n *network.Network, sidecarOrg, sidecarName string) {
	context := n.Context

	fscTop, ok := context.TopologyByName("fsc").(*fsc.Topology)
	if !ok {
		utils.Must(errors.New("cannot get fsc topo instance"))
	}

	queryServiceHost := "127.0.0.1"
	scPeer := n.Peer(sidecarOrg, sidecarName)
	queryServicePort := n.PeerPort(scPeer, "QueryServicePort")

	for _, fscNode := range fscTop.Nodes {
		logger.Infof(">>> %v", fscNode)

		endpoint := config.Endpoint{
			Address:           fmt.Sprintf("%s:%v", queryServiceHost, queryServicePort),
			ConnectionTimeout: grpc.DefaultConnectionTimeout,
			TLS: &config.TLSConfig{
				Enabled:       n.TLSEnabled,
				RootCertPaths: []string{n.CACertsBundlePath()},
			},
		}

		// When TLS is enabled, the sidecar query service requires mTLS.
		// Use the fabric peer's TLS certs (signed by the fabric org CA)
		// as client credentials so the sidecar accepts the connection.
		if n.TLSEnabled {
			fscPeer := n.FSCPeerByName(fscNode.Name)
			if fscPeer != nil {
				tlsDir := n.PeerLocalTLSDir(fscPeer)
				endpoint.TLS.ClientCertPath = filepath.Join(tlsDir, "server.crt")
				endpoint.TLS.ClientKeyPath = filepath.Join(tlsDir, "server.key")
			}
		}

		c := config.Config{
			RequestTimeout: 10 * time.Second,
			Endpoints:      []config.Endpoint{endpoint},
		}

		t, err := template.New("view_extension").Funcs(template.FuncMap{
			"NetworkName":    func() string { return n.Topology().Name() },
			"RequestTimeout": func() time.Duration { return c.RequestTimeout },
			"Endpoints":      func() []config.Endpoint { return c.Endpoints },
		}).Parse(qsExtensionTemplate)
		utils.Must(err)

		extension := bytes.NewBuffer([]byte{})
		err = t.Execute(io.MultiWriter(extension), nil)
		utils.Must(err)

		for _, uniqueName := range fscNode.ReplicaUniqueNames() {
			context.AddExtension(uniqueName, api2.FabricExtension, extension.String())
		}
	}
}

const qsExtensionTemplate = `
fabric:
  {{ NetworkName }}:
    queryService:
      requestTimeout: {{ RequestTimeout }}
      endpoints:{{- range Endpoints }}
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
