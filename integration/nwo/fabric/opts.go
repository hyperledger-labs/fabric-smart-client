/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/opts"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

const (
	ClientRole = "client"
	PeerRole   = "peer"
)

func Options(o *fsc.Options) *opts.Options {
	return opts.Get(o)
}

func WithClientRole() fsc.Option {
	return func(o *fsc.Options) error {
		Options(o).SetRole(ClientRole)
		return nil
	}
}

func WithPeerRole() fsc.Option {
	return func(o *fsc.Options) error {
		Options(o).SetRole(PeerRole)
		return nil
	}
}

func WithOrganization(Organization string) fsc.Option {
	return func(o *fsc.Options) error {
		Options(o).SetOrganization(Organization)
		return nil
	}
}

func WithAnonymousIdentity() fsc.Option {
	return func(o *fsc.Options) error {
		Options(o).SetAnonymousIdentity(true)
		return nil
	}
}
