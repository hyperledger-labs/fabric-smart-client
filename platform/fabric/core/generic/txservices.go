/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

func (c *channel) EnvelopeService() driver.EnvelopeService {
	return c.envelopeService
}

func (c *channel) TransactionService() driver.EndorserTransactionService {
	return c.transactionService
}

func (c *channel) MetadataService() driver.MetadataService {
	return c.metadataService
}
