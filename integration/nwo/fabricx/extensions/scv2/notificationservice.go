/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io"
	"time"

	api2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

// generateNSExtensions adds the committers notification service information to the config
func generateNSExtension(n *network.Network) {
	context := n.Context

	fscTop, ok := context.TopologyByName("fsc").(*fsc.Topology)
	if !ok {
		utils.Must(errors.New("cannot get fsc topo instance"))
	}

	// TODO set correct values
	notificationServiceHost := "localhost"
	notificationServicePort := 5411

	// TODO: most of this logic should go somewhere

	config := finality.Config{
		RequestTimeout: 10 * time.Second,
		Endpoints: []finality.Endpoint{
			{
				Address:           fmt.Sprintf("%s:%v", notificationServiceHost, notificationServicePort),
				ConnectionTimeout: grpc.DefaultConnectionTimeout,
				TLSEnabled:        false,
				TLSRootCertFile:   n.CACertsBundlePath(),
			},
		},
	}

	t, err := template.New("view_extension").Funcs(template.FuncMap{
		"NetworkName":    func() string { return n.Topology().Name() },
		"RequestTimeout": func() time.Duration { return config.RequestTimeout },
		"Endpoints":      func() []finality.Endpoint { return config.Endpoints },
	}).Parse(nsExtensionTemplate)
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

const nsExtensionTemplate = `
fabric:
  {{ NetworkName }}:
    notificationService:
      requestTimeout: {{ RequestTimeout }}
      endpoints:{{- range Endpoints }}
        - address: {{ .Address }}
          connectionTimeout: {{ .ConnectionTimeout }}        
          tlsEnabled: {{ .TLSEnabled }}
          tlsRootCertFile: {{ .TLSRootCertFile }}
    {{- end }}
`
