/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Committer interface {
	// IsFinal takes in input a transaction id and waits for its confirmation.
	IsFinal(txID string) error
}

type finality struct {
	committer Committer
}

func NewService(committer Committer) (*finality, error) {
	return &finality{
		committer: committer,
	}, nil
}

func (f *finality) IsFinal(txID string) error {
	return f.committer.IsFinal(txID)
}

func (f *finality) IsFinalForParties(txID string, parties ...view.Identity) error {
	panic("implement me")
}
