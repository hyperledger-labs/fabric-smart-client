/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"

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

type Finality struct {
	channel       string
	network       Network
	sp            view2.ServiceProvider
	committer     Committer
	TLSEnabled    bool
	channelConfig *config.Channel
}

func NewService(sp view2.ServiceProvider, network Network, channelConfig *config.Channel, committer Committer) (*Finality, error) {
	return &Finality{
		sp:            sp,
		network:       network,
		committer:     committer,
		channel:       channelConfig.Name,
		channelConfig: channelConfig,
		TLSEnabled:    true,
	}, nil
}

func (f *Finality) IsFinal(ctx context.Context, txID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return f.committer.IsFinal(ctx, txID)
}

func (f *Finality) IsFinalForParties(txID string, parties ...view.Identity) error {
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Is [%s] final for parties [%v]?", txID, parties)
	}

	for _, party := range parties {
		_, err := view2.GetManager(f.sp).InitiateView(
			NewIsFinalInitiatorView(
				f.network.Name(), f.channel, txID, party,
				f.channelConfig.FinalityForPartiesWaitTimeout(),
			),
		)
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("Is [%s] final on [%s]: [%s]?", txID, party, err)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
