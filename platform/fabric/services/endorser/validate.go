/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"bytes"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/security"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// fabricxDriverName is the driver name of Fabric-x networks; it must equal
// fabricx.DriverName (platform/fabricx/provider.go). The endorser package does
// not import platform/fabricx, which depends on it.
const fabricxDriverName = "fabricx"

// ErrInvalidTransaction is wrapped by the errors ValidateReceived returns for a
// transaction that must be refused.
var ErrInvalidTransaction = errors.New("invalid transaction")

// ValidateReceived checks a transaction received from a remote peer before the
// node endorses or stores it, so the sender cannot present a creator that
// disagrees with its proposal or, on Fabric, choose the txID the transaction is
// stored under. It checks that:
//
//   - the nonce and creator are non-empty;
//   - the txID equals ComputeTxID(nonce, creator), unless the network of tx is a
//     Fabric-x network, whose committer does not derive the txID from nonce and
//     creator;
//   - a signed proposal, if present, is an ENDORSER_TRANSACTION with epoch 0, a
//     non-empty nonce and creator, and a header whose txID, nonce and creator
//     equal those of tx;
//   - the creator is valid for the MSPs of the channel of tx, in the network tx
//     was decoded in.
//
// SetFromBytes copies txID, nonce and creator from the signed proposal, so the
// header equality checks fail only for a transaction a view changed after
// decoding it; endorseView validates again before signing for that reason.
//
// It does not verify the proposal signature: the proposal is signed by the
// first endorser, which need not be the creator.
//
// Failures wrap ErrInvalidTransaction and name only the txID, quoted, no other
// bytes taken from tx. While the channel has no configuration in force, the
// creator check returns an error wrapping driver.ErrNotInitialized or
// driver.ErrConfigRejected instead, which does not wrap ErrInvalidTransaction.
func ValidateReceived(tx *fabric.Transaction) error {
	txID := tx.ID()
	invalid := func(reason string) error {
		return errors.Wrapf(ErrInvalidTransaction, "tx %q: %s", txID, reason)
	}

	fns := tx.FabricNetworkService()
	ch, err := fns.Channel(tx.Channel())
	if err != nil {
		return errors.WithMessagef(err, "failed getting channel [%s]", tx.Channel())
	}

	// ComputeTxID draws a random nonce for an empty one, so check it first.
	if len(tx.Nonce()) == 0 || len(tx.Creator()) == 0 {
		return invalid("nonce or creator is empty")
	}
	if fns.ConfigService().DriverName() != fabricxDriverName {
		expected := fns.TransactionManager().ComputeTxID(&fabric.TxID{Nonce: tx.Nonce(), Creator: tx.Creator()})
		if txID != expected {
			return invalid("txID is not derived from nonce and creator")
		}
	}

	if raw := tx.SignedProposal().ProposalBytes(); raw != nil {
		up, err := transaction.UnpackSignedProposal(&pb.SignedProposal{ProposalBytes: raw})
		if err != nil {
			logger.Debugf("tx %q: failed unpacking signed proposal: %v", txID, err)
			return invalid("malformed signed proposal")
		}
		switch {
		case cb.HeaderType(up.ChannelHeader.Type) != cb.HeaderType_ENDORSER_TRANSACTION:
			return invalid("proposal is not an endorser transaction")
		case up.ChannelHeader.Epoch != 0:
			return invalid("proposal epoch is non-zero")
		case len(up.SignatureHeader.Nonce) == 0 || len(up.SignatureHeader.Creator) == 0:
			return invalid("proposal nonce or creator is empty")
		case up.TxID() != txID:
			return invalid("proposal txID does not match")
		case !bytes.Equal(up.SignatureHeader.Nonce, tx.Nonce()):
			return invalid("proposal nonce does not match")
		case !bytes.Equal(up.SignatureHeader.Creator, tx.Creator()):
			return invalid("proposal creator does not match")
		}
	}

	if err := ch.MSPManager().IsValid(tx.Creator()); err != nil {
		if errors.Is(err, driver.ErrNotInitialized) || errors.Is(err, driver.ErrConfigRejected) {
			return errors.Wrapf(err, "tx %q: cannot validate creator", txID)
		}
		logger.Debugf("tx %q: invalid creator: %v", txID, err)
		return invalid("creator is not valid")
	}
	return nil
}

// checkReceived runs ValidateReceived on tx, received from peer, and reports a
// rejection to security.Rejections when the error wraps ErrInvalidTransaction,
// with the error text as the reason.
func checkReceived(viewCtx view.Context, peer view.Identity, tx *fabric.Transaction) error {
	err := ValidateReceived(tx)
	if errors.Is(err, ErrInvalidTransaction) {
		security.GetRejections(viewCtx).Report(viewCtx.Context(), security.KindInvalidTx, peer, tx.ID(), err.Error())
	}
	return err
}

// sessionCaller returns the remote peer of the session of viewCtx, or nil when
// viewCtx has none.
func sessionCaller(viewCtx view.Context) view.Identity {
	if s := viewCtx.Session(); s != nil {
		return s.Info().Caller
	}
	return nil
}
