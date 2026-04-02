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
func generateQSExtension(n *network.Network) {
	context := n.Context

	fscTop, ok := context.TopologyByName("fsc").(*fsc.Topology)
	if !ok {
		utils.Must(errors.New("cannot get fsc topo instance"))
	}

	// TODO set correct values
	queryServiceHost := "localhost"
	queryServicePort := 7001

	// TODO: most of this logic should go into the query service package

	c := config.Config{
		RequestTimeout: 10 * time.Second,
		Endpoints: []config.Endpoint{
			{
				Address:           fmt.Sprintf("%s:%v", queryServiceHost, queryServicePort),
				ConnectionTimeout: grpc.DefaultConnectionTimeout,
				TLS: &config.TLSConfig{
					// TODO: allow TLS and mTLS integration tests
					Enabled: false,
					// note that this bundle contains all root certs of the network
					RootCertPaths: []string{n.CACertsBundlePath()},
				},
			},
		},
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

	for _, fscNode := range fscTop.Nodes {
		// TODO: find the correct SC instance to connect ...

		logger.Infof(">>> %v", fscNode)
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
