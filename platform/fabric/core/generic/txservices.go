/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/api"
)

func (c *channel) EnvelopeService() api.EnvelopeService {
	return c.envelopeService
}

func (c *channel) TransactionService() api.EndorserTransactionService {
	return c.transactionService
}

func (c *channel) MetadataService() api.MetadataService {
	return c.metadataService
}
