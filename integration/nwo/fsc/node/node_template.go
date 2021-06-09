/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

const DefaultTemplate = `/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk"

	{{ if InstallView }}viewregistry "github.com/hyperledger-labs/fabric-smart-client/platform/view"{{ end }}
	{{- range .Imports }}
	{{ Alias . }} "{{ . }}"{{ end }}
)

func main() {
	n := fscnode.New()
	n.InstallSDK(fabric.NewSDK(n))
	{{- range .SDKs }}
	n.InstallSDK({{ .Type }})
	{{ end }}
	n.Execute(func() error {
		{{- if InstallView }}
		registry := viewregistry.GetRegistry(n)
		{{- range .Factories }}
		if err := registry.RegisterFactory("{{ .Id }}", {{ .Type }}); err != nil {
			return err
		}{{ end }}
		{{- range .Responders }}
		registry.RegisterResponder({{ .Responder }}, {{ .Initiator }}){{ end }}
		{{ end }}
		return nil
	})
}
`
