/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	mspPb "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
)

var (
	testNonce   = []byte("nonce")
	testCreator = mustMarshal(&mspPb.SerializedIdentity{Mspid: "Org1MSP", IdBytes: []byte("creator")})
	testTxID    = protoutil.ComputeTxID(testNonce, testCreator)
)

// receivedTx is a transaction as SetFromBytes leaves it, backed by driver mocks.
type receivedTx struct {
	tx   *fabric.Transaction
	dtx  *mock.Transaction
	cm   *mock.ChannelMembership
	conf *mock.ConfigService
}

// newReceivedTx returns a transaction with a valid txID, nonce and creator, no
// signed proposal, a Fabric network, and a channel that accepts the creator.
func newReceivedTx() *receivedTx {
	dtx := &mock.Transaction{}
	dtx.IDReturns(testTxID)
	dtx.NonceReturns(testNonce)
	dtx.CreatorReturns(testCreator)

	tm := &mock.TransactionManager{}
	tm.ComputeTxIDStub = func(id *driver.TxIDComponents) string {
		return protoutil.ComputeTxID(id.Nonce, id.Creator)
	}
	conf := &mock.ConfigService{}
	conf.DriverNameReturns("generic")
	fns := &mock.FabricNetworkService{}
	fns.TransactionManagerReturns(tm)
	fns.ConfigServiceReturns(conf)

	cm := &mock.ChannelMembership{}
	dch := &mock.Channel{}
	dch.NameReturns("channel")
	dch.ChannelMembershipReturns(cm)
	fns.ChannelReturns(dch, nil)
	dtx.ChannelReturns("channel")

	return &receivedTx{
		tx:   fabric.NewTransaction(fabric.NewNetworkService(nil, fns, "net"), dtx),
		dtx:  dtx,
		cm:   cm,
		conf: conf,
	}
}

// withProposal attaches the signed proposal built by buildSignedProposal(mutate).
func (r *receivedTx) withProposal(t *testing.T, mutate func(*proposalMsgs)) *receivedTx {
	t.Helper()
	sp := &mock.SignedProposal{}
	sp.ProposalBytesReturns(buildSignedProposal(t, mutate).ProposalBytes)
	r.dtx.SignedProposalReturns(sp)
	return r
}

func (r *receivedTx) validate() error {
	return ValidateReceived(r.tx)
}

func TestValidateReceived_ForgedTxIDNoProposal(t *testing.T) {
	t.Parallel()
	r := newReceivedTx()
	r.dtx.IDReturns("attacker\nforged log line")
	err := r.validate()
	require.ErrorIs(t, err, ErrInvalidTransaction)
	require.ErrorContains(t, err, `tx "attacker\nforged log line": txID is not derived from nonce and creator`)
}

// SetFromBytes adopts the proposal's txID as given, so a remote initiator would
// choose the key under which the endorser stores the transaction.
func TestValidateReceived_ForgedTxIDWithProposal(t *testing.T) {
	t.Parallel()
	r := newReceivedTx().withProposal(t, func(m *proposalMsgs) {
		m.chdr.TxId = "attacker-chosen-txid"
	})
	r.dtx.IDReturns("attacker-chosen-txid")
	err := r.validate()
	require.ErrorIs(t, err, ErrInvalidTransaction)
	require.Contains(t, err.Error(), `"attacker-chosen-txid"`)
}

func TestValidateReceived_HeaderMismatch(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		mutate func(*proposalMsgs)
		reason string
	}{
		"nonce":         {func(m *proposalMsgs) { m.shdr.Nonce = []byte("other-nonce") }, "proposal nonce does not match"},
		"creator":       {func(m *proposalMsgs) { m.shdr.Creator = []byte("other-creator") }, "proposal creator does not match"},
		"txid":          {func(m *proposalMsgs) { m.chdr.TxId = "other" }, "proposal txID does not match"},
		"epoch":         {func(m *proposalMsgs) { m.chdr.Epoch = 1 }, "proposal epoch is non-zero"},
		"empty nonce":   {func(m *proposalMsgs) { m.shdr.Nonce = nil }, "proposal nonce or creator is empty"},
		"empty creator": {func(m *proposalMsgs) { m.shdr.Creator = nil }, "proposal nonce or creator is empty"},
		"unparsable":    {func(m *proposalMsgs) { m.proposal.Header = []byte("garbage") }, "malformed signed proposal"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := newReceivedTx().withProposal(t, tc.mutate).validate()
			require.ErrorIs(t, err, ErrInvalidTransaction)
			require.ErrorContains(t, err, tc.reason)
		})
	}
}

func TestValidateReceived_ConfigHeaderRejected(t *testing.T) {
	t.Parallel()
	r := newReceivedTx().withProposal(t, func(m *proposalMsgs) {
		m.chdr.Type = int32(cb.HeaderType_CONFIG)
	})
	err := r.validate()
	require.ErrorIs(t, err, ErrInvalidTransaction)
	require.ErrorContains(t, err, "proposal is not an endorser transaction")
}

// ComputeTxID would draw a random nonce for an empty one; the empty nonce is
// reported as such.
func TestValidateReceived_EmptyNonceOrCreator(t *testing.T) {
	t.Parallel()
	r := newReceivedTx()
	r.dtx.NonceReturns(nil)
	err := r.validate()
	require.ErrorIs(t, err, ErrInvalidTransaction)
	require.ErrorContains(t, err, "nonce or creator is empty")

	r = newReceivedTx()
	r.dtx.CreatorReturns(nil)
	require.ErrorContains(t, r.validate(), "nonce or creator is empty")
}

func TestValidateReceived_UnknownCreator(t *testing.T) {
	t.Parallel()
	r := newReceivedTx()
	r.cm.IsValidReturns(errors.New("unknown msp"))
	require.ErrorIs(t, r.validate(), ErrInvalidTransaction)
}

func TestValidateReceived_MSPNotInitializedIsRetryable(t *testing.T) {
	t.Parallel()
	for _, cause := range []error{driver.ErrNotInitialized, driver.ErrConfigRejected} {
		r := newReceivedTx()
		r.cm.IsValidReturns(cause)
		err := r.validate()
		require.ErrorIs(t, err, cause)
		require.NotErrorIs(t, err, ErrInvalidTransaction)
	}
}

func TestValidateReceived_Valid(t *testing.T) {
	t.Parallel()
	r := newReceivedTx()
	require.NoError(t, r.validate())
	require.Equal(t, testCreator, []byte(r.cm.IsValidArgsForCall(0)))

	require.NoError(t, newReceivedTx().withProposal(t, nil).validate())
}

// generateProposal signs with the first endorser, not the creator, so the
// proposal signature is not checked.
func TestValidateReceived_SignatureNotChecked(t *testing.T) {
	t.Parallel()
	r := newReceivedTx()
	sp := &mock.SignedProposal{}
	sp.ProposalBytesReturns(buildSignedProposal(t, nil).ProposalBytes)
	sp.SignatureReturns([]byte("signed by someone else"))
	r.dtx.SignedProposalReturns(sp)
	require.NoError(t, r.validate())
}

// skipTxIDDriver is the driver name of a test platform whose format skips the txID check, as a
// platform whose committer does not derive the txID from nonce and creator registers it.
const skipTxIDDriver = "test-skip-txid"

func init() {
	protos.RegisterTransactionFormat(skipTxIDDriver, protos.TransactionFormat{
		Component:     "test-skip-txid",
		SkipTxIDCheck: true,
		ToPayload:     func(raw []byte) ([]byte, *driver.ChannelKey, error) { return raw, nil, nil },
		ToJSON:        func(*protos.TransactionEnvelope) ([]byte, error) { return nil, errors.New("not implemented") },
	})
}

// The format of the driver decides whether the txID must equal ComputeTxID(nonce, creator):
// a driver without a format is checked, a format with SkipTxIDCheck is not.
func TestValidateReceivedUsesFormat(t *testing.T) {
	t.Parallel()
	for driverName, wantErr := range map[string]bool{"generic": true, "unregistered": true, skipTxIDDriver: false} {
		t.Run(driverName, func(t *testing.T) {
			t.Parallel()
			r := newReceivedTx().withProposal(t, func(m *proposalMsgs) { m.chdr.TxId = "forged" })
			r.conf.DriverNameReturns(driverName)
			r.dtx.IDReturns("forged")
			if wantErr {
				require.ErrorContains(t, r.validate(), "txID is not derived from nonce and creator")
				return
			}
			require.NoError(t, r.validate())
		})
	}

	// Skipping the txID check keeps the proposal checks.
	r := newReceivedTx().withProposal(t, func(m *proposalMsgs) {
		m.chdr.TxId = "forged"
		m.shdr.Nonce = []byte("other-nonce")
	})
	r.conf.DriverNameReturns(skipTxIDDriver)
	r.dtx.IDReturns("forged")
	require.ErrorContains(t, r.validate(), "proposal nonce does not match")
}

// proposalMsgs holds the layers of a signed chaincode proposal.
type proposalMsgs struct {
	cis      *pb.ChaincodeInvocationSpec
	cpp      *pb.ChaincodeProposalPayload
	ext      *pb.ChaincodeHeaderExtension
	chdr     *cb.ChannelHeader
	shdr     *cb.SignatureHeader
	header   *cb.Header
	proposal *pb.Proposal
}

// buildSignedProposal marshals a valid signed proposal for testTxID, testNonce
// and testCreator. Each layer is marshaled into the byte field of its parent
// unless mutate already set that field, so a test can replace a single layer.
func buildSignedProposal(tb testing.TB, mutate func(*proposalMsgs)) *pb.SignedProposal {
	tb.Helper()
	m := &proposalMsgs{
		cis: &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: "mycc", Version: "1.0"},
			Input:       &pb.ChaincodeInput{Args: [][]byte{[]byte("invoke"), []byte("arg1")}},
		}},
		cpp: &pb.ChaincodeProposalPayload{},
		ext: &pb.ChaincodeHeaderExtension{ChaincodeId: &pb.ChaincodeID{Name: "mycc", Version: "1.0"}},
		chdr: &cb.ChannelHeader{
			Type:      int32(cb.HeaderType_ENDORSER_TRANSACTION),
			TxId:      testTxID,
			ChannelId: "channel",
		},
		shdr:     &cb.SignatureHeader{Creator: testCreator, Nonce: testNonce},
		header:   &cb.Header{},
		proposal: &pb.Proposal{},
	}
	if mutate != nil {
		mutate(m)
	}
	fillBytes(&m.cpp.Input, m.cis)
	fillBytes(&m.proposal.Payload, m.cpp)
	fillBytes(&m.chdr.Extension, m.ext)
	fillBytes(&m.header.ChannelHeader, m.chdr)
	fillBytes(&m.header.SignatureHeader, m.shdr)
	fillBytes(&m.proposal.Header, m.header)
	return &pb.SignedProposal{ProposalBytes: mustMarshal(m.proposal), Signature: []byte("signature")}
}

func fillBytes(dst *[]byte, msg proto.Message) {
	if *dst == nil {
		*dst = mustMarshal(msg)
	}
}

func mustMarshal(msg proto.Message) []byte {
	b, err := proto.Marshal(msg)
	if err != nil {
		panic(err)
	}
	return b
}
