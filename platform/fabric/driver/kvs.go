/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"

type Key struct {
	Network string
	Channel string
	TxID    driver.TxID
}

type MetadataStore interface {
	driver.MetadataStore[Key, TransientMap]
}

type EnvelopeStore interface {
	driver.EnvelopeStore[Key]
}

type EndorseTxStore interface {
	driver.EndorseTxStore[Key]
}
