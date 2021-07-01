/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"time"

	view3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = flogging.MustGetLogger("fabric-sdk.finality")

type Config interface {
	TLSEnabled() bool
}

type Committer interface {
	// IsFinal takes in input a transaction id and waits for its confirmation.
	IsFinal(txID string) error
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

func (f *finality) IsFinal(txID string) error {
	return f.committer.IsFinal(txID)
}

func (f *finality) IsFinalForParties(txID string, parties ...view.Identity) error {
	logger.Debugf("Is [%s] final for parties [%v]?", txID, parties)

	var err error
	comm, err := f.network.Comm(f.channel)
	if err != nil {
		return err
	}

	for _, party := range parties {
		logger.Debugf("Asking [%s] if [%s] is final...", party, txID)
		if f.network.LocalMembership().IsMe(party) {
			logger.Debugf("[%s] is me, skipping.", party, txID)
			continue
		}

		endpoints, err := view2.GetEndpointService(f.sp).Endpoint(party)
		if err != nil {
			return err
		}
		logger.Debugf("Asking [%s] resolved from [%s] if [%s] is final...", endpoints[view2.ViewPort], party, txID)

		var certs [][]byte
		if f.TLSEnabled {
			certs, err = comm.GetTLSRootCert(party)
			if err != nil {
				return err
			}
		}

		c, err := view3.New(
			&view3.Config{
				ID: "",
				FSCNode: &grpc.ConnectionConfig{
					Address:           endpoints[view2.ViewPort],
					ConnectionTimeout: 300 * time.Second,
					TLSEnabled:        f.TLSEnabled,
					TLSRootCertBytes:  certs,
				},
			},
			f.network.LocalMembership().DefaultSigningIdentity(),
			hash.GetHasher(f.sp),
		)
		if err != nil {
			logger.Errorf("Failed connecting to [%s] resolved from [%s] to as if [%s] is final...", endpoints[view2.ViewPort], party, txID)
			return err
		}
		err = c.IsTxFinal(txID)
		logger.Debugf("Is [%s] final on [%s]: [%s]?", txID, party, err)
		if err != nil {
			return err
		}
	}

	return nil
}
