/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

type Key struct {
	Network driver.Network
	Channel driver.Channel
	TxID    driver.TxID
}

func (k Key) UniqueKey() string {
	return fmt.Sprintf("%s.%s.%s", k.Network, k.Channel, k.TxID)
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
