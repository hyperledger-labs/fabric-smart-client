/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"go.uber.org/dig"

	commondig "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/preamble"
)

// ComponentEntry provides a component into the dig value group "fsc-components". An SDK, FSC's
// or an application's, provides one per component it owns during Install; SDKs nested in each
// other share one container, so every entry is reached. The view SDK collects the group when it
// starts, before comm starts, publishes it in the node's preamble and negotiates the versions
// of each session from it. Two entries with the same name fail the start with an error naming
// both ranges.
type ComponentEntry struct {
	dig.Out
	Component preamble.Component `group:"fsc-components"`
}

// NewComponentEntry returns a constructor of the ComponentEntry for the component name at
// versions minVersion to maxVersion, to pass to Container().Provide.
func NewComponentEntry(name string, minVersion, maxVersion uint32) func() ComponentEntry {
	return func() ComponentEntry {
		return ComponentEntry{Component: preamble.Component{Name: name, Range: preamble.Range{Min: minVersion, Max: maxVersion}}}
	}
}

// installComponents makes the components of the group the node's components. An empty group,
// as in a container that holds no FSC SDK, keeps preamble.FSCComponents.
func installComponents(c commondig.Container) error {
	return c.Invoke(func(in struct {
		dig.In
		Components []preamble.Component `group:"fsc-components"`
	},
	) error {
		if len(in.Components) == 0 {
			return nil
		}
		return preamble.SetComponents(in.Components)
	})
}
