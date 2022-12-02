/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
)

func (c *Committer) handleConfig(block *common.Block, i int, env *common.Envelope) error {
	committer, err := c.network.Committer(c.channel)
	if err != nil {
		return errors.Wrapf(err, "cannot get Committer for channel [%s]", c.channel)
	}
	if err := committer.CommitConfig(block.Header.Number, block.Data.Data[i], env); err != nil {
		return errors.Wrapf(err, "cannot commit config envelope for channel [%s]", c.channel)
	}
	return nil
}
