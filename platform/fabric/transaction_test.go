/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestTransactionOptions(t *testing.T) {
	t.Parallel()

	t.Run("default type", func(t *testing.T) {
		t.Parallel()
		opts, err := CompileTransactionOptions(
			WithCreator([]byte("creator")),
			WithContext(t.Context()),
			WithChannel("mychannel"),
			WithNonce([]byte("nonce")),
			WithTxID("txid1"),
			WithRawRequest([]byte("req")),
		)
		require.NoError(t, err)
		require.Equal(t, view.Identity("creator"), opts.Creator)
		require.Equal(t, t.Context(), opts.Ctx)
		require.Equal(t, "mychannel", opts.Channel)
		require.Equal(t, []byte("nonce"), opts.Nonce)
		require.Equal(t, "txid1", opts.TxID)
		require.Equal(t, []byte("req"), opts.RawRequest)
		require.Equal(t, driver.EndorserTransaction, opts.TransactionType)
	})

	t.Run("override type", func(t *testing.T) {
		t.Parallel()
		opts, err := CompileTransactionOptions(
			WithTransactionType(driver.TransactionType(99)),
		)
		require.NoError(t, err)
		require.Equal(t, driver.TransactionType(99), opts.TransactionType)
	})
}

func TestTxID(t *testing.T) {
	t.Parallel()
	id := &TxID{Nonce: []byte("n"), Creator: []byte("c")}
	require.Equal(t, "[bg==:Yw==]", id.String())
}

func TestProposalResponse(t *testing.T) {
	t.Parallel()

	mpr := &mock.ProposalResponse{}
	pr := NewProposalResponse(mpr)

	mpr.ResponseStatusReturns(200)
	require.Equal(t, int32(200), pr.ResponseStatus())

	mpr.ResponseMessageReturns("ok")
	require.Equal(t, "ok", pr.ResponseMessage())

	mpr.EndorserReturns([]byte("endorser"))
	require.Equal(t, []byte("endorser"), pr.Endorser())

	mpr.PayloadReturns([]byte("payload"))
	require.Equal(t, []byte("payload"), pr.Payload())

	mpr.EndorserSignatureReturns([]byte("sig"))
	require.Equal(t, []byte("sig"), pr.EndorserSignature())

	mpr.ResultsReturns([]byte("results"))
	require.Equal(t, []byte("results"), pr.Results())

	mpr.BytesReturns([]byte("bytes"), nil)
	b, err := pr.Bytes()
	require.NoError(t, err)
	require.Equal(t, []byte("bytes"), b)

	mpr.VerifyEndorsementReturns(nil)
	require.NoError(t, pr.VerifyEndorsement(nil))
}

func TestProposal(t *testing.T) {
	t.Parallel()

	mp := &mock.Proposal{}
	p := &Proposal{p: mp}

	mp.HeaderReturns([]byte("header"))
	require.Equal(t, []byte("header"), p.Header())

	mp.PayloadReturns([]byte("payload"))
	require.Equal(t, []byte("payload"), p.Payload())
}

func TestSignedProposalWrapper(t *testing.T) {
	t.Parallel()

	msp := &mock.SignedProposal{}
	sp := &SignedProposal{s: msp}

	msp.ProposalBytesReturns([]byte("bytes"))
	require.Equal(t, []byte("bytes"), sp.ProposalBytes())

	msp.SignatureReturns([]byte("sig"))
	require.Equal(t, []byte("sig"), sp.Signature())

	msp.ProposalHashReturns([]byte("hash"))
	require.Equal(t, []byte("hash"), sp.ProposalHash())

	msp.ChaincodeNameReturns("cc")
	require.Equal(t, "cc", sp.ChaincodeName())

	msp.ChaincodeVersionReturns("v1")
	require.Equal(t, "v1", sp.ChaincodeVersion())
}

func TestTransaction(t *testing.T) {
	t.Parallel()

	mtx := &mock.Transaction{}
	tx := NewTransaction(nil, mtx)

	mtx.CreatorReturns([]byte("creator"))
	require.Equal(t, view.Identity("creator"), tx.Creator())

	mtx.NonceReturns([]byte("nonce"))
	require.Equal(t, []byte("nonce"), tx.Nonce())

	mtx.IDReturns("tx1")
	require.Equal(t, "tx1", tx.ID())

	mtx.NetworkReturns("net1")
	require.Equal(t, "net1", tx.Network())

	mtx.ChannelReturns("ch1")
	require.Equal(t, "ch1", tx.Channel())

	mtx.FunctionReturns("func1")
	require.Equal(t, "func1", tx.Function())

	mtx.ParametersReturns([][]byte{[]byte("param1")})
	require.Equal(t, [][]byte{[]byte("param1")}, tx.Parameters())

	mtx.ChaincodeReturns("cc1")
	require.Equal(t, "cc1", tx.Chaincode())

	mtx.ChaincodeVersionReturns("v1")
	require.Equal(t, "v1", tx.ChaincodeVersion())

	mtx.ResultsReturns([]byte("res1"), nil)
	r, err := tx.Results()
	require.NoError(t, err)
	require.Equal(t, []byte("res1"), r)

	mtx2 := &mock.Transaction{}
	tx2 := NewTransaction(nil, mtx2)
	mtx.FromReturns(nil)
	require.NoError(t, tx.From(tx2))

	mtx.SetFromBytesReturns(nil)
	require.NoError(t, tx.SetFromBytes([]byte("raw")))

	mtx.SetFromEnvelopeBytesReturns(nil)
	require.NoError(t, tx.SetFromEnvelopeBytes([]byte("raw")))

	mp := &mock.Proposal{}
	mtx.ProposalReturns(mp)
	require.NotNil(t, tx.Proposal())

	msp := &mock.SignedProposal{}
	mtx.SignedProposalReturns(msp)
	require.NotNil(t, tx.SignedProposal())

	tx.SetProposal("cc", "v1", "func", "p1")
	require.Equal(t, 1, mtx.SetProposalCallCount())

	tx.AppendParameter([]byte("p"))
	require.Equal(t, 1, mtx.AppendParameterCallCount())

	mtx.SetParameterAtReturns(nil)
	require.NoError(t, tx.SetParameterAt(0, []byte("p")))

	mtx.TransientReturns(driver.TransientMap{})
	require.NotNil(t, tx.Transient())

	tx.ResetTransient()
	require.Equal(t, 1, mtx.ResetTransientCallCount())

	mtx.SetRWSetReturns(nil)
	require.NoError(t, tx.SetRWSet())

	mrws := &mock.RWSet{}
	mtx.RWSReturns(mrws)
	require.NotNil(t, tx.RWS())

	mtx.DoneReturns(nil)
	require.NoError(t, tx.Done())

	tx.Close()
	require.Equal(t, 1, mtx.CloseCallCount())

	mtx.RawReturns([]byte("raw"), nil)
	raw, err := tx.Raw()
	require.NoError(t, err)
	require.Equal(t, []byte("raw"), raw)

	mtx.GetRWSetReturns(mrws, nil)
	rws, err := tx.GetRWSet()
	require.NoError(t, err)
	require.NotNil(t, rws)

	mtx.BytesReturns([]byte("bytes"), nil)
	b, err := tx.Bytes()
	require.NoError(t, err)
	require.Equal(t, []byte("bytes"), b)

	mtx.EndorseReturns(nil)
	require.NoError(t, tx.Endorse())

	mtx.EndorseWithIdentityReturns(nil)
	require.NoError(t, tx.EndorseWithIdentity([]byte("id")))

	mtx.EndorseWithSignerReturns(nil)
	require.NoError(t, tx.EndorseWithSigner([]byte("id"), nil))

	mtx.EndorseProposalReturns(nil)
	require.NoError(t, tx.EndorseProposal())

	mtx.EndorseProposalWithIdentityReturns(nil)
	require.NoError(t, tx.EndorseProposalWithIdentity([]byte("id")))

	mtx.EndorseProposalResponseReturns(nil)
	require.NoError(t, tx.EndorseProposalResponse())

	mtx.EndorseProposalResponseWithIdentityReturns(nil)
	require.NoError(t, tx.EndorseProposalResponseWithIdentity([]byte("id")))

	mpr := &mock.ProposalResponse{}
	pr := NewProposalResponse(mpr)
	mtx.AppendProposalResponseReturns(nil)
	require.NoError(t, tx.AppendProposalResponse(pr))

	mtx.ProposalHasBeenEndorsedByReturns(nil)
	require.NoError(t, tx.ProposalHasBeenEndorsedBy([]byte("party")))

	mtx.StoreTransientReturns(nil)
	require.NoError(t, tx.StoreTransient())

	mtx.ProposalResponsesReturns([]driver.ProposalResponse{mpr}, nil)
	prs, err := tx.ProposalResponses()
	require.NoError(t, err)
	require.Len(t, prs, 1)

	mtx.ProposalResponseReturns([]byte("pr"), nil)
	prBytes, err := tx.ProposalResponse()
	require.NoError(t, err)
	require.Equal(t, []byte("pr"), prBytes)

	mtx.BytesNoTransientReturns([]byte("bnt"), nil)
	bnt, err := tx.BytesNoTransient()
	require.NoError(t, err)
	require.Equal(t, []byte("bnt"), bnt)

	menv := &mock.Envelope{}
	mtx.EnvelopeReturns(menv, nil)
	env, err := tx.Envelope()
	require.NoError(t, err)
	require.NotNil(t, env)

	require.Nil(t, tx.FabricNetworkService())
}

type mockTxManager struct {
	driver.TransactionManager
	mtx *mock.Transaction
}

func (m *mockTxManager) NewTransaction(ctx context.Context, txType driver.TransactionType, creator view.Identity, nonce []byte, txID, channel string, rawRequest []byte) (driver.Transaction, error) {
	return m.mtx, nil
}

func (m *mockTxManager) NewTransactionFromBytes(ctx context.Context, channel string, raw []byte) (driver.Transaction, error) {
	return m.mtx, nil
}

func (m *mockTxManager) NewTransactionFromEnvelopeBytes(ctx context.Context, channel string, raw []byte) (driver.Transaction, error) {
	return m.mtx, nil
}

func (m *mockTxManager) NewEnvelope() driver.Envelope {
	return &mock.Envelope{}
}

func (m *mockTxManager) NewProposalResponseFromBytes(raw []byte) (driver.ProposalResponse, error) {
	return &mock.ProposalResponse{}, nil
}

func (m *mockTxManager) ComputeTxID(id *driver.TxIDComponents) string {
	id.Nonce = []byte("nonce")
	id.Creator = []byte("creator")
	return "computed_txid"
}

func TestTransactionManager(t *testing.T) {
	t.Parallel()

	mfnsInner := &mock.FabricNetworkService{}

	// FNS wrapping
	fns := &NetworkService{fns: mfnsInner, channels: make(map[string]*Channel)}

	mtxm := &mockTxManager{mtx: &mock.Transaction{}}
	mfnsInner.TransactionManagerReturns(mtxm)

	mockCh := &mock.Channel{}
	mockCh.NameReturns("mychannel")
	mfnsInner.ChannelReturns(mockCh, nil)

	tm := &TransactionManager{fns: fns}

	tx, err := tm.NewTransaction(WithChannel("mychannel"))
	require.NoError(t, err)
	require.NotNil(t, tx)

	tx, err = tm.NewTransactionFromBytes([]byte("raw"), WithChannel("mychannel"))
	require.NoError(t, err)
	require.NotNil(t, tx)

	tx, err = tm.NewTransactionFromEnvelopeBytes([]byte("raw"), WithChannel("mychannel"))
	require.NoError(t, err)
	require.NotNil(t, tx)

	env := tm.NewEnvelope()
	require.NotNil(t, env)

	pr, err := tm.NewProposalResponseFromBytes([]byte("raw"))
	require.NoError(t, err)
	require.NotNil(t, pr)

	id := &TxID{Nonce: []byte("n"), Creator: []byte("c")}
	res := tm.ComputeTxID(id)
	require.Equal(t, "computed_txid", res)
	require.Equal(t, []byte("nonce"), id.Nonce)
	require.Equal(t, []byte("creator"), id.Creator)

	// test error cases for missing channels: the manager propagates the
	// channel-lookup error unchanged, so assert on its distinctive message.
	mfnsInner.ChannelReturns(nil, errors.New("channel unavailable"))

	_, err = tm.NewTransaction(WithChannel("badchan"))
	require.ErrorContains(t, err, "channel unavailable")

	_, err = tm.NewTransactionFromBytes([]byte("raw"), WithChannel("badchan"))
	require.ErrorContains(t, err, "channel unavailable")

	_, err = tm.NewTransactionFromEnvelopeBytes([]byte("raw"), WithChannel("badchan"))
	require.ErrorContains(t, err, "channel unavailable")

	// test CompileTransactionOptions error: option errors propagate unchanged
	badOpt := func(*TransactionOptions) error { return errors.New("bad opt") }
	_, err = tm.NewTransaction(badOpt)
	require.ErrorContains(t, err, "bad opt")
	_, err = tm.NewTransactionFromBytes([]byte("raw"), badOpt)
	require.ErrorContains(t, err, "bad opt")
	_, err = tm.NewTransactionFromEnvelopeBytes([]byte("raw"), badOpt)
	require.ErrorContains(t, err, "bad opt")
}

func TestMetadataService(t *testing.T) {
	t.Parallel()

	mms := &mockMetaSvc{existsReturns: true}
	ms := &MetadataService{ms: mms}

	ctx := t.Context()
	require.True(t, ms.Exists(ctx, "txid"))

	err := ms.StoreTransient(ctx, "txid", TransientMap{"k": []byte("v")})
	require.NoError(t, err)

	tm, err := ms.LoadTransient(ctx, "txid")
	require.NoError(t, err)
	require.Nil(t, tm)
}

func TestEnvelopeService(t *testing.T) {
	t.Parallel()

	mes := &mockEnvSvc{}
	es := &EnvelopeService{ms: mes}

	ctx := t.Context()
	require.True(t, es.Exists(ctx, "txid"))
}
