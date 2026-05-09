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
	fxgrpc "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

// generateNSExtensions adds the committers notification service information to the config.
// When TLS is enabled, each FSC node gets its own extension with mTLS client
// credentials taken from the fabric peer's TLS directory.
func generateNSExtension(n *network.Network, notificationServicePort uint16, notificationServiceHost string) {
	context := n.Context

	fscTop, ok := context.TopologyByName("fsc").(*fsc.Topology)
	if !ok {
		utils.Must(errors.New("cannot get fsc topo instance"))
	}

	for _, fscNode := range fscTop.Nodes {
		logger.Infof(">>> %v", fscNode)

		endpoint := fxgrpc.Endpoint{
			Address:           fmt.Sprintf("%s:%v", notificationServiceHost, notificationServicePort),
			ConnectionTimeout: grpc.DefaultConnectionTimeout,
			TLS: &fxgrpc.TLSConfig{
				Enabled:       n.TLSEnabled,
				RootCertPaths: []string{n.CACertsBundlePath()},
			},
		}

		// When TLS is enabled, the sidecar notification service requires mTLS.
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

		config := fxgrpc.Config{
			RequestTimeout: 10 * time.Second,
			Endpoints:      []fxgrpc.Endpoint{endpoint},
		}

		t, err := template.New("view_extension").Funcs(template.FuncMap{
			"NetworkName":    func() string { return n.Topology().Name() },
			"RequestTimeout": func() time.Duration { return config.RequestTimeout },
			"Endpoints":      func() []fxgrpc.Endpoint { return config.Endpoints },
		}).Parse(nsExtensionTemplate)
		utils.Must(err)

		extension := bytes.NewBuffer([]byte{})
		err = t.Execute(io.MultiWriter(extension), nil)
		utils.Must(err)

		for _, uniqueName := range fscNode.ReplicaUniqueNames() {
			context.AddExtension(uniqueName, api2.FabricExtension, extension.String())
		}
	}
}

const nsExtensionTemplate = `
fabric:
  {{ NetworkName }}:
    notificationService:
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
