/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package state

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// NewCollectEndorsementsView returns a view that does the following:
// 1. It contacts each passed party sequentially and sends the marshalled version of the passed transaction.
// 2. It waits for a response containing either the endorsement of the transaction or an error.
func NewCollectEndorsementsView(tx *Transaction, parties ...view.Identity) view.View {
	return endorser.NewCollectEndorsementsView(tx.tx, parties...)
}

func NewCollectApprovesView(tx *Transaction, parties ...view.Identity) view.View {
	return endorser.NewCollectEndorsementsView(tx.tx, parties...)
}

// NewEndorseView returns a view that does the following:
// 1. It signs the transaction with the signing key of each passed identity
// 2. Send the transaction back on the context's session.
func NewEndorseView(tx *Transaction, ids ...view.Identity) view.View {
	return endorser.NewEndorseView(tx.tx, ids...)
}

func NewParallelCollectEndorsementsOnProposalView(tx *Transaction, parties ...view.Identity) view.View {
	return endorser.NewParallelCollectEndorsementsOnProposalView(tx.tx, parties...)
}

func NewEndorsementOnProposalResponderView(tx *Transaction) view.View {
	return endorser.NewEndorsementOnProposalResponderView(tx.tx)
}
