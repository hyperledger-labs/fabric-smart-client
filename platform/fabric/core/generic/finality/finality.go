/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"go.uber.org/zap/zapcore"
)

var logger = flogging.MustGetLogger("fabric-sdk.finality")

type Config interface {
	TLSEnabled() bool
}

type Committer interface {
	// IsFinal takes in input a transaction id and waits for its confirmation.
	IsFinal(ctx context.Context, txID string) error
}

type finality struct {
	channel    string
	network    Network
	sp         view2.ServiceProvider
	committer  Committer
	TLSEnabled bool
}

func NewService(sp view2.ServiceProvider, network Network, channel string, committer Committer) (*finality, error) {
	return &finality{
		sp:         sp,
		network:    network,
		committer:  committer,
		channel:    channel,
		TLSEnabled: true,
	}, nil
}

func (f *finality) IsFinal(ctx context.Context, txID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return f.committer.IsFinal(ctx, txID)
}

func (f *finality) IsFinalForParties(txID string, parties ...view.Identity) error {
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Is [%s] final for parties [%v]?", txID, parties)
	}

	for _, party := range parties {
		_, err := view2.GetManager(f.sp).InitiateView(NewIsFinalInitiatorView(f.network.Name(), f.channel, txID, party))
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("Is [%s] final on [%s]: [%s]?", txID, party, err)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
