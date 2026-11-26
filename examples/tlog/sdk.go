/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tlog

import (
	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog/views"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	common "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	fabricx "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

// SDK is the fabricx SDK plus the services the tlog views look up: the state
// vault and the node's LogService.
type SDK struct {
	common.SDK
}

// NewSDK returns the tlog SDK on top of a fresh fabricx SDK.
func NewSDK(registry services.Registry) *SDK {
	return &SDK{SDK: fabricx.NewSDK(registry)}
}

// Install installs the wrapped SDK, then registers the tlog's services.
func (p *SDK) Install() error {
	if err := p.SDK.Install(); err != nil {
		return err
	}
	if err := p.Container().Provide(func() *views.LogService { return &views.LogService{} }); err != nil {
		return errors.Wrap(err, "failed providing the log service")
	}
	return errors.Join(
		digutils.Register[state.VaultService](p.Container()),
		digutils.Register[*views.LogService](p.Container()),
	)
}
