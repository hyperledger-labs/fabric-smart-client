/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

func (c *Committer) HandleConfig(ctx context.Context, block *common.BlockMetadata, tx CommitTx) (*FinalityEvent, error) {
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("[%s] Config transaction received: %s", c.ChannelConfig.ID(), tx.TxID)
	}
	if err := c.CommitConfig(ctx, tx.BlkNum, tx.Raw, tx.Envelope); err != nil {
		return nil, errors.Wrapf(err, "cannot commit config envelope for channel [%s]", c.ChannelConfig.ID())
	}
	return &FinalityEvent{Ctx: ctx}, nil
}
