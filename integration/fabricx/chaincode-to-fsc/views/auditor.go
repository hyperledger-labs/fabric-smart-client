/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/chaincode-to-fsc/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// AuditorView is registered as a responder on every state-changing initiator
// view. Its job is to produce a witness signature that the auditor saw the
// transaction. It does not run business validation — that is the endorser's
// job — but it does record what it saw, in this example via a log line.
//
// Migration note: classical chaincode does not have a built-in auditor role.
// Producing an auditable record typically required either off-chain log
// scraping or a chaincode that explicitly emitted audit events. With FSC
// views this becomes a topology-level concern: we add an auditor FSC node,
// register it as a responder, and every state change automatically pulls
// the auditor into the endorsement collection step.
type AuditorView struct{}

func (a *AuditorView) Call(viewCtx view.Context) (interface{}, error) {
	tx, err := state.ReceiveTransaction(viewCtx)
	assert.NoError(err, "auditor failed receiving transaction")

	// Best-effort log of what the auditor saw. In a production deployment
	// this would write to a tamper-evident audit log; here it is a single
	// structured log line that the test harness asserts on.
	if tx.Commands().Count() > 0 {
		cmd := tx.Commands().At(0)
		logger.Infof("AUDIT: tx=%s namespace=%s command=%s inputs=%d outputs=%d",
			tx.ID(), Namespace, cmd.Name, tx.NumInputs(), tx.NumOutputs())
		// Snapshot the first output if any — useful when reading the test
		// log to confirm the auditor saw the right asset move.
		if tx.NumOutputs() > 0 {
			out := &states.Asset{}
			if err := tx.GetOutputAt(0, out); err == nil {
				logger.Infof("AUDIT:   output[0] id=%s owner=%s color=%s size=%d value=%d",
					out.ID, out.Owner, out.Color, out.Size, out.AppraisedValue)
			}
		}
	}

	// Auditor signs as a witness. This is only meaningful if the namespace
	// endorsement policy includes the auditor's organisation; otherwise the
	// signature is collected but ignored at validation time.
	if _, err := viewCtx.RunView(state.NewEndorseView(tx)); err != nil {
		return nil, err
	}
	return nil, nil
}
