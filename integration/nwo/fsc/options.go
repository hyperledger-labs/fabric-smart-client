/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"

func WithOrionPersistence(network, db, creator string) node.Option {
	return func(o *node.Options) error {
		o.Put("fsc.persistence.orion", network)
		o.Put("fsc.persistence.orion.database", db)
		o.Put("fsc.persistence.orion.creator", creator)
		return nil
	}
}
