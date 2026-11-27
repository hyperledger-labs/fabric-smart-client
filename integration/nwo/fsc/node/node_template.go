/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

// DefaultTemplate is the main of a node: it installs the node's SDKs and registers its
// factories and responders in the setup function of cli.Execute.
const DefaultTemplate = `/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
	fsccli "github.com/hyperledger-labs/fabric-smart-client/node/cli"

	{{ if InstallView }}viewregistry "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"{{ end }}
	{{- range .Imports }}
	{{ Alias . }} "{{ . }}"{{ end }}
)

func main() {
	n := fscnode.NewFromConfPath("")
	{{- range .SDKs }}
	n.InstallSDK({{ .Type }})
	{{ end }}
	fsccli.Execute(n, func() error {
		{{- if InstallView }}
		registry := viewregistry.GetRegistry(n)
		{{- range .Factories }}
		if err := registry.RegisterFactory("{{ .Id }}", {{ .Type }}); err != nil {
			return err
		}{{ end }}
		{{- range .Responders }}
		if err := registry.RegisterResponder({{ .Responder }}, {{ .Initiator }}); err != nil {
			return err
		}{{ end }}
		{{ end }}
		return nil
	})
}
`

// TemplateWithoutCLI is the main of DefaultTemplate for an FSC release without the node/cli
// package, v0.29 and older; the mixed-version suites build it at such a release.
// ponytail: delete once FSC_OLDER_RELEASE and FSC_PREVIOUS_RELEASE both have node/cli.
const TemplateWithoutCLI = `/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"

	{{ if InstallView }}viewregistry "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"{{ end }}
	{{- range .Imports }}
	{{ Alias . }} "{{ . }}"{{ end }}
)

func main() {
	n := fscnode.New()
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
