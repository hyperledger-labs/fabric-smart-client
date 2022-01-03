/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"github.com/hyperledger/fabric-protos-go/common"
)

func (c *committer) handleConfig(block *common.Block, i int, env *common.Envelope) {
	committer, err := c.network.Committer(c.channel)
	if err != nil {
		logger.Panicf("Cannot get Committer [%s]", err)
	}

	if err := committer.CommitConfig(block.Header.Number, block.Data.Data[i], env); err != nil {
		logger.Panicf("Cannot commit config envelope [%s]", err)
	}
}
