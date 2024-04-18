/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

func (c *Service) HandleConfig(block *common.Block, i int, event *FinalityEvent, env *common.Envelope, chHdr *common.ChannelHeader) error {
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("[%s] Config transaction received: %s", c.ChannelConfig.ID(), chHdr.TxId)
	}
	if err := c.CommitConfig(block.Header.Number, block.Data.Data[i], env); err != nil {
		return errors.Wrapf(err, "cannot commit config envelope for channel [%s]", c.ChannelConfig.ID())
	}
	return nil
}
