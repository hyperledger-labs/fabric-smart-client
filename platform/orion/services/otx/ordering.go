/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package otx

import (
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type orderingAndFinalityView struct {
	tx *transaction
}

func NewOrderingAndFinalityView(tx *transaction) *orderingAndFinalityView {
	return &orderingAndFinalityView{tx: tx}
}

func (o *orderingAndFinalityView) Call(context view.Context) (interface{}, error) {
	dataTx, err := o.tx.getDataTx()
	if err != nil {
		return "", errors.Wrap(err, "failed getting data tx")
	}
	_, _, err = dataTx.Commit(true)
	if err != nil {
		return "", errors.Wrap(err, "error during transaction commit")
	}

	// txEnv, err := dataTx.CommittedTxEnvelope()
	// if err != nil {
	// 	return "", errors.New("error getting transaction envelope")
	// }

	// err = saveTxEvidence(demoDir, txID, txEnv, txReceipt, lg)
	// if err != nil {
	// 	return "", err
	// }
	return nil, nil
}
