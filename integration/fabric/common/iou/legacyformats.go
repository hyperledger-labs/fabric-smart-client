/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou

import (
	"context"
	"slices"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/preamble"
)

// LegacyFormatsSDK makes its node support only version 1 of the fabric and fabricx formats,
// as a node of a release without the protobuf envelopes does. It sets the components in
// Install, which the node runs for every SDK before it starts any, so the comm service
// publishes the narrowed ranges from its first stream. Components belong to the process, so
// the SDK narrows the ranges of every node in the binary.
type LegacyFormatsSDK struct{}

// NewLegacyFormatsSDK returns a LegacyFormatsSDK. nwo calls it from the generated main.
func NewLegacyFormatsSDK(services.Registry) *LegacyFormatsSDK { return &LegacyFormatsSDK{} }

// Install sets fabric and fabricx to versions 1 to 1, keeping the other FSC components.
func (*LegacyFormatsSDK) Install() error {
	cs := slices.Clone(preamble.FSCComponents)
	for i := range cs {
		if cs[i].Name == "fabric" || cs[i].Name == "fabricx" {
			cs[i].Range = preamble.Range{Min: 1, Max: 1}
		}
	}
	return preamble.SetComponents(cs)
}

// Start does nothing.
func (*LegacyFormatsSDK) Start(context.Context) error { return nil }
