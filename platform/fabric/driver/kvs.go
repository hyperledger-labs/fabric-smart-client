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

type MetadataKVS interface {
	driver.MetadataKVS[Key, TransientMap]
}

type EnvelopeKVS interface {
	driver.EnvelopeKVS[Key]
}

type EndorseTxKVS interface {
	driver.EndorseTxKVS[Key]
}
