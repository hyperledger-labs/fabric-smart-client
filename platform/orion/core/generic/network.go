/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

var logger = flogging.MustGetLogger("orion-sdk.core")

type network struct {
	sp   view2.ServiceProvider
	ctx  context.Context
	name string
}

func NewNetwork(
	ctx context.Context,
	sp view2.ServiceProvider,
	name string,
) (*network, error) {
	// Load configuration
	n := &network{
		ctx:  ctx,
		sp:   sp,
		name: name,
	}
	return n, nil
}

func (f *network) Name() string {
	return f.name
}
