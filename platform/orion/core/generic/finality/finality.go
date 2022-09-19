/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"golang.org/x/net/context"
)

type Committer interface {
	// IsFinal takes in input a transaction id and waits for its confirmation.
	IsFinal(ctx context.Context, txID string) error
}

type finality struct {
	committer Committer
}

func NewService(committer Committer) (*finality, error) {
	return &finality{
		committer: committer,
	}, nil
}

func (f *finality) IsFinal(ctx context.Context, txID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return f.committer.IsFinal(ctx, txID)
}

func (f *finality) IsFinalForParties(txID string, parties ...view.Identity) error {
	panic("implement me")
}
