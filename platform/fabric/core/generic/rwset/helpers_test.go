/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	"context"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ledgerrwset "github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset/kvrwset"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
	gproto "google.golang.org/protobuf/proto"

	pkgproto "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	protofakes "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil/fakes"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type fakeRWSet struct {
	cdriver.RWSet
	namespaces []cdriver.Namespace
	doneCalls  int
}

func (f *fakeRWSet) Namespaces() []cdriver.Namespace {
	return append([]cdriver.Namespace(nil), f.namespaces...)
}

func (f *fakeRWSet) Done() {
	f.doneCalls++
}

type fakeInspector struct {
	fdriver.RWSetInspector
	newFunc     func(ctx context.Context, txID cdriver.TxID, rwset []byte) (fdriver.RWSet, error)
	inspectFunc func(ctx context.Context, rwsetBytes []byte, namespaces ...cdriver.Namespace) (fdriver.RWSet, error)
}

func (f *fakeInspector) NewRWSetFromBytes(ctx context.Context, txID cdriver.TxID, rwset []byte) (fdriver.RWSet, error) {
	if f.newFunc == nil {
		return nil, nil
	}
	return f.newFunc(ctx, txID, rwset)
}

func (f *fakeInspector) InspectRWSet(ctx context.Context, rwsetBytes []byte, namespaces ...cdriver.Namespace) (fdriver.RWSet, error) {
	if f.inspectFunc == nil {
		return nil, nil
	}
	return f.inspectFunc(ctx, rwsetBytes, namespaces...)
}

type fakeEnvelopeService struct {
	fdriver.EnvelopeService
	existsFn func(ctx context.Context, txID string) bool
	loadFn   func(ctx context.Context, txID string) ([]byte, error)
}

func (f *fakeEnvelopeService) Exists(ctx context.Context, txID string) bool {
	if f.existsFn == nil {
		return false
	}
	return f.existsFn(ctx, txID)
}

func (f *fakeEnvelopeService) LoadEnvelope(ctx context.Context, txID string) ([]byte, error) {
	if f.loadFn == nil {
		return nil, nil
	}
	return f.loadFn(ctx, txID)
}

type fakeTransactionService struct {
	fdriver.EndorserTransactionService
	existsFn func(ctx context.Context, txID string) bool
	loadFn   func(ctx context.Context, txID string) ([]byte, error)
}

func (f *fakeTransactionService) Exists(ctx context.Context, txID string) bool {
	if f.existsFn == nil {
		return false
	}
	return f.existsFn(ctx, txID)
}

func (f *fakeTransactionService) LoadTransaction(ctx context.Context, txID string) ([]byte, error) {
	if f.loadFn == nil {
		return nil, nil
	}
	return f.loadFn(ctx, txID)
}

type fakeTransactionManager struct {
	fdriver.TransactionManager
	newFromBytesFn func(ctx context.Context, channel string, raw []byte) (fdriver.Transaction, error)
}

func (f *fakeTransactionManager) NewTransactionFromBytes(ctx context.Context, channel string, raw []byte) (fdriver.Transaction, error) {
	if f.newFromBytesFn == nil {
		return nil, nil
	}
	return f.newFromBytesFn(ctx, channel, raw)
}

type fakeTransaction struct {
	fdriver.Transaction
	getRWSetFn func() (fdriver.RWSet, error)
}

func (f *fakeTransaction) GetRWSet() (fdriver.RWSet, error) {
	if f.getRWSetFn == nil {
		return nil, nil
	}
	return f.getRWSetFn()
}

type fakeRWSetHandler struct {
	fdriver.RWSetPayloadHandler
	loadFn func(payl *cb.Payload, chdr *cb.ChannelHeader) (fdriver.RWSet, fdriver.ProcessTransaction, error)
}

func (f *fakeRWSetHandler) Load(payl *cb.Payload, chdr *cb.ChannelHeader) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
	return f.loadFn(payl, chdr)
}

type fakeRWSetLoader struct {
	fdriver.RWSetLoader
	fromEnvFn func(ctx context.Context, txID cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error)
	fromETxFn func(ctx context.Context, txID cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error)
}

func (f *fakeRWSetLoader) GetRWSetFromEvn(ctx context.Context, txID cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
	return f.fromEnvFn(ctx, txID)
}

func (f *fakeRWSetLoader) GetRWSetFromETx(ctx context.Context, txID cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
	return f.fromETxFn(ctx, txID)
}

type fakeChannel struct {
	fdriver.Channel
	envService fdriver.EnvelopeService
	txService  fdriver.EndorserTransactionService
	loader     fdriver.RWSetLoader
}

func (f *fakeChannel) EnvelopeService() fdriver.EnvelopeService {
	return f.envService
}

func (f *fakeChannel) TransactionService() fdriver.EndorserTransactionService {
	return f.txService
}

func (f *fakeChannel) RWSetLoader() fdriver.RWSetLoader {
	return f.loader
}

type fakeChannelProvider struct {
	channelFn func(name string) (fdriver.Channel, error)
}

func (f *fakeChannelProvider) Channel(name string) (fdriver.Channel, error) {
	return f.channelFn(name)
}

type fakeProcessTransaction struct {
	id      string
	network string
	channel string
	fn      string
	args    []string
}

func (f *fakeProcessTransaction) Network() string {
	return f.network
}

func (f *fakeProcessTransaction) Channel() string {
	return f.channel
}

func (f *fakeProcessTransaction) ID() string {
	return f.id
}

func (f *fakeProcessTransaction) FunctionAndParameters() (string, []string) {
	return f.fn, append([]string(nil), f.args...)
}

type fakeProcessor struct {
	processFn func(req fdriver.Request, tx fdriver.ProcessTransaction, rws fdriver.RWSet, ns string) error
}

func (f *fakeProcessor) Process(req fdriver.Request, tx fdriver.ProcessTransaction, rws fdriver.RWSet, ns string) error {
	return f.processFn(req, tx, rws, ns)
}

func mustMarshalProto(t *testing.T, msg gproto.Message) []byte {
	t.Helper()
	raw, err := pkgproto.Marshal(msg)
	require.NoError(t, err)
	return raw
}

func buildTestEnvelope(t *testing.T, headerType cb.HeaderType, results []byte) (*cb.Envelope, *cb.Payload, *cb.ChannelHeader, []byte, string, []string) {
	t.Helper()

	creator := []byte("creator")
	nonce := []byte("nonce")
	txID := protoutil.ComputeTxID(nonce, creator)
	function := "invoke"
	args := []string{"a", "b"}
	cis := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			Type:        pb.ChaincodeSpec_GOLANG,
			ChaincodeId: &pb.ChaincodeID{Name: "mycc", Version: "v1"},
			Input: &pb.ChaincodeInput{
				Args: [][]byte{[]byte(function), []byte(args[0]), []byte(args[1])},
			},
		},
	}

	proposal, _, err := protoutil.CreateChaincodeProposalWithTxIDNonceAndTransient(
		txID,
		headerType,
		"mychannel",
		cis,
		nonce,
		creator,
		nil,
	)
	require.NoError(t, err)

	signer := &protofakes.SignerSerializer{}
	signer.SerializeReturns(creator, nil)
	signer.SignReturns([]byte("signature"), nil)

	response := &pb.Response{Status: 200, Message: "OK"}
	prpBytes, err := protoutil.GetBytesProposalResponsePayload(
		[]byte("proposal-hash"),
		response,
		results,
		nil,
		cis.ChaincodeSpec.ChaincodeId,
	)
	require.NoError(t, err)

	env, err := protoutil.CreateSignedTx(
		proposal,
		signer,
		&pb.ProposalResponse{
			Payload:  prpBytes,
			Response: response,
			Endorsement: &pb.Endorsement{
				Endorser:  []byte("endorser"),
				Signature: []byte("endorser-sig"),
			},
		},
	)
	require.NoError(t, err)

	payl, err := protoutil.UnmarshalPayload(env.Payload)
	require.NoError(t, err)
	chdr, err := protoutil.UnmarshalChannelHeader(payl.Header.ChannelHeader)
	require.NoError(t, err)

	return env, payl, chdr, creator, function, args
}

func buildValidRWSetBytes(t *testing.T) []byte {
	t.Helper()

	return mustMarshalProto(t, &ledgerrwset.TxReadWriteSet{
		NsRwset: []*ledgerrwset.NsReadWriteSet{
			{
				Namespace: "ns1",
				Rwset: mustMarshalProto(t, &kvrwset.KVRWSet{
					Reads: []*kvrwset.KVRead{
						{Key: "k1", Version: &kvrwset.Version{BlockNum: 1, TxNum: 2}},
					},
					Writes: []*kvrwset.KVWrite{
						{Key: "k2", Value: []byte("v2")},
					},
					MetadataWrites: []*kvrwset.KVMetadataWrite{
						{
							Key: "k3",
							Entries: []*kvrwset.KVMetadataEntry{
								{Name: "m1", Value: []byte("mv1")},
							},
						},
					},
				}),
			},
		},
	})
}
