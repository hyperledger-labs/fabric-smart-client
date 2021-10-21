/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"

	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

var logger = flogging.MustGetLogger("orion-sdk.core")

type network struct {
	sp  view2.ServiceProvider
	ctx context.Context

	config *config2.Config
	name   string

	sessionManager  *SessionManager
	identityManager *IdentityManager
}

func NewNetwork(
	ctx context.Context,
	sp view2.ServiceProvider,
	config *config2.Config,
	name string,
) (*network, error) {
	// Load configuration
	n := &network{
		ctx:    ctx,
		sp:     sp,
		name:   name,
		config: config,
	}
	ids, err := config.Identities()
	if err != nil {
		return nil, err
	}
	var dids []*driver.Identity
	for _, id := range ids {
		dids = append(dids, &driver.Identity{
			Name: id.Name,
			Cert: id.Cert,
			Key:  id.Key,
		})
	}
	n.identityManager = &IdentityManager{
		identities:      dids,
		defaultIdentity: dids[0].Name,
	}
	n.sessionManager = &SessionManager{
		config:          config,
		identityManager: n.identityManager,
	}
	return n, nil
}

func (f *network) Name() string {
	return f.name
}

func (f *network) IdentityManager() driver.IdentityManager {
	return f.identityManager
}

func (f *network) SessionManager() driver.SessionManager {
	return f.sessionManager
}
