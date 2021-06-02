/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package state

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func NewCollectEndorsementsView(tx *Transaction, parties ...view.Identity) view.View {
	return endorser.NewCollectEndorsementsView(tx.tx, parties...)
}

func NewCollectApprovesView(tx *Transaction, parties ...view.Identity) view.View {
	return endorser.NewCollectEndorsementsView(tx.tx, parties...)
}

func NewEndorseView(tx *Transaction, ids ...view.Identity) view.View {
	return endorser.NewEndorseView(tx.tx, ids...)
}

func NewParallelCollectEndorsementsOnProposalView(tx *Transaction, parties ...view.Identity) view.View {
	return endorser.NewParallelCollectEndorsementsOnProposalView(tx.tx, parties...)
}

func NewEndorsementOnProposalResponderView(tx *Transaction) view.View {
	return endorser.NewEndorsementOnProposalResponderView(tx.tx)
}
