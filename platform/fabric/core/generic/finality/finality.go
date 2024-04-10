/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("fabric-sdk.finality")

type Committer interface {
	// IsFinal takes in input a transaction id and waits for its confirmation.
	IsFinal(ctx context.Context, txID string) error
}

type Finality struct {
	committer Committer
}

func NewService(committer Committer) (*Finality, error) {
	return &Finality{
		committer: committer,
	}, nil
}

func (f *Finality) IsFinal(ctx context.Context, txID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return f.committer.IsFinal(ctx, txID)
}
