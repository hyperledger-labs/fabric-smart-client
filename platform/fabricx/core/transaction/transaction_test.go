/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	commondriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/transaction/mocks"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric-x-common/api/applicationpb"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mocks/rwset.go --fake-name FakeRWSet github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.RWSet
//go:generate counterfeiter -o mocks/vault.go --fake-name FakeVault github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Vault
//go:generate counterfeiter -o mocks/metadata_service.go --fake-name FakeMetadataService github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.MetadataService
//go:generate counterfeiter -o mocks/channel.go --fake-name FakeChannel github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Channel

func TestTransactionGetters(t *testing.T) {
	t.Parallel()

	tx := &Transaction{
		TCreator:    view.Identity([]byte("creator")),
		TNonce:      []byte("nonce"),
		TTxID:       "tx1",
		TNetwork:    "network1",
		TChannel:    "channel1",
		TChaincode:  "cc",
		TFunction:   "invoke",
		TParameters: [][]byte{[]byte("a")},
		TTransient:  driver.TransientMap{"k": []byte("v")},
	}

	require.Equal(t, view.Identity([]byte("creator")), tx.Creator())
	require.Equal(t, []byte("nonce"), tx.Nonce())
	require.Equal(t, "tx1", tx.ID())
	require.Equal(t, "network1", tx.Network())
	require.Equal(t, "channel1", tx.Channel())
	require.Equal(t, "cc", tx.Chaincode())
	require.Equal(t, "invoke", tx.Function())
	require.Equal(t, [][]byte{[]byte("a")}, tx.Parameters())
	require.Equal(t, driver.TransientMap{"k": []byte("v")}, tx.Transient())
}

func TestTransactionFunctionAndParameters(t *testing.T) {
	t.Parallel()

	tx := &Transaction{TFunction: "invoke", TParameters: [][]byte{[]byte("a"), []byte("b")}}
	f, params := tx.FunctionAndParameters()
	require.Equal(t, "invoke", f)
	require.Equal(t, []string{"a", "b"}, params)
}

func TestTransactionResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		tx            *Transaction
		expected      []byte
		expectedError string
	}{
		{
			name:          "no proposal responses",
			tx:            &Transaction{},
			expectedError: "transaction has no proposal responses",
		},
		{
			name:     "returns first payload",
			tx:       &Transaction{TProposalResponses: []*peer.ProposalResponse{{Payload: []byte("first")}, {Payload: []byte("second")}}},
			expected: []byte("first"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := tc.tx.Results()
			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expected, res)
		})
	}
}

func TestTransactionFrom(t *testing.T) {
	t.Parallel()

	src := &Transaction{
		TCreator:          view.Identity([]byte("creator")),
		TNonce:            []byte("nonce"),
		TTxID:             "tx1",
		TNetwork:          "network1",
		TChannel:          "channel1",
		TChaincode:        "cc",
		TChaincodeVersion: "v1",
		TFunction:         "invoke",
		TParameters:       [][]byte{[]byte("a"), []byte("b")},
		RWSet:             []byte("rwset"),
		TProposal:         &peer.Proposal{Payload: []byte("proposal")},
		TTransient:        driver.TransientMap{"k": []byte("v")},
		TProposalResponses: []*peer.ProposalResponse{{
			Payload: []byte("response"),
		}},
	}

	tests := []struct {
		name          string
		input         driver.Transaction
		expectedError string
		assert        func(*testing.T, *Transaction)
	}{
		{
			name:          "wrong type",
			input:         nil,
			expectedError: "wrong transaction type",
		},
		{
			name:  "copies fields",
			input: src,
			assert: func(t *testing.T, dst *Transaction) {
				t.Helper()
				require.Equal(t, src.TCreator, dst.TCreator)
				require.Equal(t, src.TNonce, dst.TNonce)
				require.Equal(t, src.TTxID, dst.TTxID)
				require.Equal(t, src.TNetwork, dst.TNetwork)
				require.Equal(t, src.TChannel, dst.TChannel)
				require.Equal(t, src.TChaincode, dst.TChaincode)
				require.Equal(t, src.TChaincodeVersion, dst.TChaincodeVersion)
				require.Equal(t, src.TFunction, dst.TFunction)
				require.Equal(t, src.TParameters, dst.TParameters)
				require.Equal(t, src.RWSet, dst.RWSet)
				require.Equal(t, src.TProposal, dst.TProposal)
				require.Equal(t, src.TTransient, dst.TTransient)
				require.Equal(t, src.TProposalResponses, dst.TProposalResponses)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dst := &Transaction{}
			err := dst.From(tc.input)
			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
				return
			}

			require.NoError(t, err)
			tc.assert(t, dst)
		})
	}
}

func TestTransactionSetProposal(t *testing.T) {
	t.Parallel()

	tx := &Transaction{}
	tx.SetProposal("cc", "v1", "invoke", "a", "b")

	require.Equal(t, "cc", tx.Chaincode())
	require.Equal(t, "v1", tx.ChaincodeVersion())
	require.Equal(t, "invoke", tx.Function())
	require.Equal(t, [][]byte{[]byte("a"), []byte("b")}, tx.Parameters())
}

func TestTransactionAppendAndSetParameter(t *testing.T) {
	t.Parallel()

	tx := &Transaction{TParameters: [][]byte{[]byte("a")}}
	tx.AppendParameter([]byte("b"))
	require.Equal(t, [][]byte{[]byte("a"), []byte("b")}, tx.Parameters())

	err := tx.SetParameterAt(1, []byte("c"))
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("a"), []byte("c")}, tx.Parameters())

	err = tx.SetParameterAt(5, []byte("x"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid index")
}

func TestTransactionResetTransient(t *testing.T) {
	t.Parallel()

	tx := &Transaction{TTransient: driver.TransientMap{"a": []byte("1")}}
	tx.ResetTransient()
	require.NotNil(t, tx.Transient())
	require.Empty(t, tx.Transient())
}

func TestTransactionSignedProposal(t *testing.T) {
	t.Parallel()
	require.Nil(t, (&Transaction{}).SignedProposal())
}

func TestTransactionProposalResponses(t *testing.T) {
	t.Parallel()

	tx := &Transaction{
		TTxID: "tx1",
		TProposalResponses: []*peer.ProposalResponse{
			{
				Payload:     []byte("payload-1"),
				Endorsement: &peer.Endorsement{Endorser: []byte("endorser-1"), Signature: []byte("signature-1")},
				Response:    &peer.Response{Status: 200, Message: "ok"},
			},
			{
				Payload:     []byte("payload-2"),
				Endorsement: &peer.Endorsement{Endorser: []byte("endorser-2"), Signature: []byte("signature-2")},
				Response:    &peer.Response{Status: 201, Message: "accepted"},
			},
		},
	}

	resps, err := tx.ProposalResponses()
	require.NoError(t, err)
	require.Len(t, resps, 2)
	require.Equal(t, []byte("payload-1"), resps[0].Payload())
	require.Equal(t, []byte("payload-2"), resps[1].Payload())
}

func TestTransactionProposalResponse(t *testing.T) {
	t.Parallel()

	tx := &Transaction{proposalResponse: &peer.ProposalResponse{
		Payload:     []byte("payload"),
		Endorsement: &peer.Endorsement{Endorser: []byte("endorser"), Signature: []byte("signature")},
		Response:    &peer.Response{Status: 200, Message: "ok"},
	}}

	raw, err := tx.ProposalResponse()
	require.NoError(t, err)

	decoded := &peer.ProposalResponse{}
	err = proto.Unmarshal(raw, decoded)
	require.NoError(t, err)
	require.True(t, proto.Equal(tx.proposalResponse, decoded))
}

func TestAppendProposalResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		existing      []*peer.ProposalResponse
		response      *peer.ProposalResponse
		expectedCount int
		assert        func(*testing.T, *Transaction)
	}{
		{
			name:          "appends new endorser",
			existing:      []*peer.ProposalResponse{},
			response:      &peer.ProposalResponse{Endorsement: &peer.Endorsement{Endorser: []byte("endorser-1")}},
			expectedCount: 1,
		},
		{
			name:          "skips duplicate endorser",
			existing:      []*peer.ProposalResponse{{Endorsement: &peer.Endorsement{Endorser: []byte("endorser-1")}}},
			response:      &peer.ProposalResponse{Endorsement: &peer.Endorsement{Endorser: []byte("endorser-1")}},
			expectedCount: 1,
			assert: func(t *testing.T, tx *Transaction) {
				t.Helper()
				require.Equal(t, []byte("endorser-1"), tx.TProposalResponses[0].Endorsement.Endorser)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tx := &Transaction{TProposalResponses: tc.existing}
			err := tx.appendProposalResponse(tc.response)
			require.NoError(t, err)
			require.Len(t, tx.TProposalResponses, tc.expectedCount)
			if tc.assert != nil {
				tc.assert(t, tx)
			}
		})
	}
}

func TestAppendProposalResponseDriverWrapper(t *testing.T) {
	t.Parallel()

	wrappedResp, err := NewProposalResponseFromResponse(&peer.ProposalResponse{Endorsement: &peer.Endorsement{Endorser: []byte("endorser-1")}})
	require.NoError(t, err)

	tests := []struct {
		name          string
		response      driver.ProposalResponse
		expectedError string
		assert        func(*testing.T, *Transaction)
	}{
		{
			name:          "wrong type",
			response:      nil,
			expectedError: "wrong proposal response type",
		},
		{
			name:     "wrapped proposal response",
			response: wrappedResp,
			assert: func(t *testing.T, tx *Transaction) {
				t.Helper()
				require.Len(t, tx.TProposalResponses, 1)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tx := &Transaction{}
			err := tx.AppendProposalResponse(tc.response)
			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
				return
			}

			require.NoError(t, err)
			tc.assert(t, tx)
		})
	}
}

func TestToMSPSignerIdentityWithCertificate(t *testing.T) {
	t.Parallel()

	raw, err := proto.Marshal(&msp.SerializedIdentity{Mspid: "Org1MSP", IdBytes: []byte("cert-bytes")})
	require.NoError(t, err)

	tests := []struct {
		name          string
		identity      view.Identity
		expectedMSP   string
		expectedCert  []byte
		expectedError string
	}{
		{
			name:         "success",
			identity:     view.Identity(raw),
			expectedMSP:  "Org1MSP",
			expectedCert: []byte("cert-bytes"),
		},
		{
			name:          "invalid serialized identity",
			identity:      view.Identity([]byte("not-a-protobuf")),
			expectedError: "unmarshal serialized identity",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			id, err := toMSPSignerIdentityWithCertificate(tc.identity)
			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expectedMSP, id.GetMspId())
			require.Equal(t, tc.expectedCert, id.GetCertificate())
		})
	}
}

type testSerializableSigner struct {
	creator []byte
	signRes []byte
	signErr error
}

func (s *testSerializableSigner) Sign(message []byte) ([]byte, error) { return s.signRes, s.signErr }
func (s *testSerializableSigner) Serialize() ([]byte, error)          { return s.creator, nil }

func testSignedProposalBytes(t *testing.T) *peer.SignedProposal {
	t.Helper()
	signerIdentityRaw, err := proto.Marshal(&msp.SerializedIdentity{Mspid: "Org1MSP", IdBytes: []byte("cert-bytes")})
	require.NoError(t, err)

	tx := &Transaction{
		TTxID:             "tx-signed",
		TNonce:            []byte("nonce"),
		TCreator:          view.Identity(signerIdentityRaw),
		TChannel:          "channel1",
		TChaincode:        "cc",
		TChaincodeVersion: "v1",
		TFunction:         "invoke",
		TParameters:       [][]byte{[]byte("a"), []byte("b")},
	}

	err = tx.generateProposal(&testSerializableSigner{creator: signerIdentityRaw, signRes: []byte("sig")})
	require.NoError(t, err)
	return tx.TSignedProposal
}

func TestTransactionSetRWSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		tx                     *Transaction
		expectedNewRWSetCalls  int
		expectedFromBytesCalls int
		expectedFromBytesArg   []byte
	}{
		{
			name:                  "from scratch",
			tx:                    &Transaction{ctx: t.Context(), TTxID: "tx1"},
			expectedNewRWSetCalls: 1,
		},
		{
			name:                   "from existing bytes",
			tx:                     &Transaction{ctx: t.Context(), TTxID: "tx2", RWSet: []byte("raw")},
			expectedFromBytesCalls: 1,
			expectedFromBytesArg:   []byte("raw"),
		},
		{
			name: "from proposal response payload",
			tx: &Transaction{
				ctx:   t.Context(),
				TTxID: "tx3",
				TProposalResponses: []*peer.ProposalResponse{{
					Payload: []byte("proposal-rwset"),
				}},
			},
			expectedFromBytesCalls: 1,
			expectedFromBytesArg:   []byte("proposal-rwset"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fakeVault := &mocks.FakeVault{}
			fakeRWSet := &mocks.FakeRWSet{}
			fakeVault.NewRWSetReturns(fakeRWSet, nil)
			fakeVault.NewRWSetFromBytesReturns(fakeRWSet, nil)

			ch := &mocks.FakeChannel{}
			ch.VaultReturns(fakeVault)
			tc.tx.channel = ch

			err := tc.tx.SetRWSet()
			require.NoError(t, err)
			require.Equal(t, tc.expectedNewRWSetCalls, fakeVault.NewRWSetCallCount())
			require.Equal(t, tc.expectedFromBytesCalls, fakeVault.NewRWSetFromBytesCallCount())
			if tc.expectedNewRWSetCalls == 1 {
				_, gotTxID := fakeVault.NewRWSetArgsForCall(0)
				require.Equal(t, commondriver.TxID(tc.tx.TTxID), gotTxID)
			}
			if tc.expectedFromBytesCalls == 1 {
				_, gotTxID, gotBytes := fakeVault.NewRWSetFromBytesArgsForCall(0)
				require.Equal(t, commondriver.TxID(tc.tx.TTxID), gotTxID)
				require.Equal(t, tc.expectedFromBytesArg, gotBytes)
			}
			require.Same(t, fakeRWSet, tc.tx.RWS())
		})
	}
}

func TestTransactionDoneRawGetRWSetAndClose(t *testing.T) {
	t.Parallel()

	t.Run("done stores rwset bytes", func(t *testing.T) {
		t.Parallel()
		fakeRWSet := &mocks.FakeRWSet{}
		fakeRWSet.BytesReturns([]byte("rwset-bytes"), nil)
		fakeRWSet.NamespacesReturns([]commondriver.Namespace{"ns1"})

		tx := &Transaction{TTxID: "tx1", rwset: fakeRWSet}
		err := tx.Done()
		require.NoError(t, err)
		require.Equal(t, 1, fakeRWSet.DoneCallCount())
		require.Equal(t, []byte("rwset-bytes"), tx.RWSet)
	})

	t.Run("done wraps rwset bytes error", func(t *testing.T) {
		t.Parallel()
		fakeRWSet := &mocks.FakeRWSet{}
		fakeRWSet.BytesReturns(nil, errors.New("boom"))

		tx := &Transaction{TTxID: "tx1", rwset: fakeRWSet}
		err := tx.Done()
		require.Error(t, err)
		require.Contains(t, err.Error(), "marshalling rws")
	})

	t.Run("raw serializes current rwset", func(t *testing.T) {
		t.Parallel()
		fakeRWSet := &mocks.FakeRWSet{}
		fakeRWSet.BytesReturns([]byte("raw-rwset"), nil)

		tx := &Transaction{TTxID: "tx1", rwset: fakeRWSet}
		raw, err := tx.Raw()
		require.NoError(t, err)
		require.Contains(t, string(raw), `"RWSet":"cmF3LXJ3c2V0"`)
	})

	t.Run("raw wraps rwset bytes error", func(t *testing.T) {
		t.Parallel()
		fakeRWSet := &mocks.FakeRWSet{}
		fakeRWSet.BytesReturns(nil, errors.New("boom"))

		tx := &Transaction{TTxID: "tx1", rwset: fakeRWSet}
		_, err := tx.Raw()
		require.Error(t, err)
		require.Contains(t, err.Error(), "marshalling rws")
	})

	t.Run("get rwset returns existing one", func(t *testing.T) {
		t.Parallel()
		fakeRWSet := &mocks.FakeRWSet{}
		tx := &Transaction{rwset: fakeRWSet}
		got, err := tx.GetRWSet()
		require.NoError(t, err)
		require.Same(t, fakeRWSet, got)
	})

	t.Run("get rwset initializes it", func(t *testing.T) {
		t.Parallel()
		fakeVault := &mocks.FakeVault{}
		fakeRWSet := &mocks.FakeRWSet{}
		fakeVault.NewRWSetReturns(fakeRWSet, nil)

		tx := &Transaction{ctx: t.Context(), channel: func() *mocks.FakeChannel { ch := &mocks.FakeChannel{}; ch.VaultReturns(fakeVault); return ch }(), TTxID: "tx2"}
		got, err := tx.GetRWSet()
		require.NoError(t, err)
		require.Equal(t, 1, fakeVault.NewRWSetCallCount())
		require.Same(t, fakeRWSet, got)
	})

	t.Run("close terminates and clears rwset", func(t *testing.T) {
		t.Parallel()
		fakeRWSet := &mocks.FakeRWSet{}
		tx := &Transaction{TTxID: "tx3", rwset: fakeRWSet}
		tx.Close()
		require.Equal(t, 1, fakeRWSet.DoneCallCount())
		require.Nil(t, tx.RWS())
	})
}

func TestTransactionBytesNoTransient(t *testing.T) {
	t.Parallel()

	fakeRWSet := &mocks.FakeRWSet{}
	fakeRWSet.BytesReturns([]byte("rwset-bytes"), nil)
	fakeRWSet.NamespacesReturns([]commondriver.Namespace{"ns1"})

	tx := &Transaction{TTxID: "tx1", TTransient: driver.TransientMap{"secret": []byte("value")}, rwset: fakeRWSet}
	raw, err := tx.BytesNoTransient()
	require.NoError(t, err)

	var decoded Transaction
	err = json.Unmarshal(raw, &decoded)
	require.NoError(t, err)
	require.Empty(t, decoded.TTransient)
	require.Equal(t, []byte("rwset-bytes"), decoded.RWSet)
}

func TestTransactionSetFromBytes(t *testing.T) {
	t.Parallel()

	signedProposal := testSignedProposalBytes(t)
	serializedTx := &Transaction{TTxID: "ser-1", TChannel: "ch1", TProposalResponses: []*peer.ProposalResponse{{Payload: []byte("p")}}}
	serializedRaw, err := serializedTx.Bytes()
	require.NoError(t, err)

	fullPopulationRaw, err := json.Marshal(&Transaction{TSignedProposal: signedProposal})
	require.NoError(t, err)

	tests := []struct {
		name             string
		raw              []byte
		channelErr       error
		expectedTxID     string
		expectedChannel  string
		expectSignedProp bool
		expectedError    string
	}{
		{
			name:            "from serialized bytes",
			raw:             serializedRaw,
			expectedTxID:    "ser-1",
			expectedChannel: "ch1",
		},
		{
			name:             "full population from signed proposal",
			raw:              fullPopulationRaw,
			expectedTxID:     "tx-signed",
			expectedChannel:  "channel1",
			expectSignedProp: true,
		},
		{
			name:          "invalid json",
			raw:           []byte("not-json"),
			expectedError: "json unmarshal from bytes",
		},
		{
			name:          "channel lookup fails",
			raw:           serializedRaw,
			channelErr:    errors.New("boom"),
			expectedError: "get channel [ch1]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fakeFNS := &mocks.FakeFabricNetworkService{}
			fakeFNS.ChannelReturns(&mocks.FakeChannel{}, tc.channelErr)

			tx := &Transaction{fns: fakeFNS}
			err := tx.SetFromBytes(tc.raw)
			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expectedTxID, tx.ID())
			require.Equal(t, tc.expectedChannel, tx.Channel())
			require.Equal(t, 1, fakeFNS.ChannelCallCount())
			if tc.expectSignedProp {
				require.NotNil(t, tx.SignedProposal())
			}
		})
	}
}

func TestTransactionSetFromEnvelopeBytes(t *testing.T) {
	t.Parallel()

	tx := &Transaction{}
	err := tx.SetFromEnvelopeBytes([]byte("not-an-envelope"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unpack envelope from bytes")
}

func TestEndorseWithIdentity(t *testing.T) {
	t.Parallel()

	testID := view.Identity([]byte("test-id"))

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		fakeFNS := &mocks.FakeFabricNetworkService{}
		fakeSS := &mocks.FakeSignerService{}
		fakeSigner := &mocks.FakeSigner{}
		fakeSigner.SignReturns([]byte("sig"), nil)
		fakeFNS.SignerServiceReturns(fakeSS)
		fakeSS.GetSignerReturns(fakeSigner, nil)

		tx := &Transaction{
			ctx:            t.Context(),
			fns:            fakeFNS,
			signedProposal: &SignedProposal{},
			channel: func() *mocks.FakeChannel {
				ch := &mocks.FakeChannel{}
				ch.MetadataServiceReturns(&mocks.FakeMetadataService{})
				return ch
			}(),
		}

		err := tx.EndorseWithIdentity(testID)
		require.NoError(t, err)
		require.Equal(t, 1, fakeSS.GetSignerCallCount())
		require.Equal(t, testID, fakeSS.GetSignerArgsForCall(0))
	})
}

func TestGetProposalResponse(t *testing.T) {
	t.Parallel()

	signerIdentityRaw, err := proto.Marshal(&msp.SerializedIdentity{Mspid: "Org1MSP", IdBytes: []byte("cert-bytes")})
	require.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		txPayload := &applicationpb.Tx{Namespaces: []*applicationpb.TxNamespace{{NsId: "ns1"}, {NsId: "ns2"}}}
		rwsetBytes, err := proto.Marshal(txPayload)
		require.NoError(t, err)

		fakeRWSet := &mocks.FakeRWSet{}
		fakeRWSet.BytesReturns(rwsetBytes, nil)
		fakeSigner := &testSerializableSigner{creator: signerIdentityRaw, signRes: []byte("signature-data")}

		signedProposal := testSignedProposalBytes(t)
		sp, err := newSignedProposal(signedProposal)
		require.NoError(t, err)

		tx := &Transaction{TTxID: "tx1", signedProposal: sp, rwset: fakeRWSet}
		resp, err := tx.getProposalResponse(fakeSigner)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, int32(200), resp.Response.Status)
		require.Equal(t, []byte("tx1"), resp.Response.Payload)
		require.Equal(t, rwsetBytes, resp.Payload)
		require.NotNil(t, resp.Endorsement)
		require.Equal(t, signerIdentityRaw, resp.Endorsement.Endorser)
	})

	t.Run("error when proposal already has endorsements", func(t *testing.T) {
		t.Parallel()
		txPayload := &applicationpb.Tx{Namespaces: []*applicationpb.TxNamespace{{NsId: "ns1"}}, Endorsements: []*applicationpb.Endorsements{{}}}
		rwsetBytes, err := proto.Marshal(txPayload)
		require.NoError(t, err)

		fakeRWSet := &mocks.FakeRWSet{}
		fakeRWSet.BytesReturns(rwsetBytes, nil)

		tx := &Transaction{TTxID: "tx1", signedProposal: &SignedProposal{}, rwset: fakeRWSet}
		resp, err := tx.getProposalResponse(&testSerializableSigner{})
		require.Error(t, err)
		require.Nil(t, resp)
		require.Contains(t, err.Error(), "transaction proposal already contains endorsements")
	})

	t.Run("error nil signed proposal", func(t *testing.T) {
		t.Parallel()
		tx := &Transaction{TTxID: "tx1"}
		resp, err := tx.getProposalResponse(&testSerializableSigner{})
		require.Error(t, err)
		require.Nil(t, resp)
		require.Contains(t, err.Error(), "getting signed proposal")
	})
}

func TestEndorseProposalResponseWithIdentity(t *testing.T) {
	t.Parallel()

	signerIdentityRaw, err := proto.Marshal(&msp.SerializedIdentity{Mspid: "Org1MSP", IdBytes: []byte("cert-bytes")})
	require.NoError(t, err)
	testID := view.Identity(signerIdentityRaw)

	txPayload := &applicationpb.Tx{Namespaces: []*applicationpb.TxNamespace{{NsId: "ns1"}}}
	validRWSet, err := proto.Marshal(txPayload)
	require.NoError(t, err)

	tests := []struct {
		name          string
		mockSetup     func(*mocks.FakeFabricNetworkService, *mocks.FakeSignerService)
		rwsetPayload  []byte
		withProposal  bool
		expectedError string
	}{
		{
			name: "success",
			mockSetup: func(fns *mocks.FakeFabricNetworkService, ss *mocks.FakeSignerService) {
				fns.SignerServiceReturns(ss)
				fakeSigner := &mocks.FakeSigner{}
				fakeSigner.SignReturns([]byte("sig"), nil)
				ss.GetSignerReturns(fakeSigner, nil)
			},
			rwsetPayload: validRWSet,
			withProposal: true,
		},
		{
			name: "signer service fails",
			mockSetup: func(fns *mocks.FakeFabricNetworkService, ss *mocks.FakeSignerService) {
				fns.SignerServiceReturns(ss)
				ss.GetSignerReturns(nil, errors.New("signer not found"))
			},
			rwsetPayload:  validRWSet,
			expectedError: "get signer",
		},
		{
			name: "proposal response generation fails",
			mockSetup: func(fns *mocks.FakeFabricNetworkService, ss *mocks.FakeSignerService) {
				fns.SignerServiceReturns(ss)
				fakeSigner := &mocks.FakeSigner{}
				fakeSigner.SignReturns([]byte("sig"), nil)
				ss.GetSignerReturns(fakeSigner, nil)
			},
			rwsetPayload:  validRWSet,
			expectedError: "generate signed proposal response",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fakeFNS := &mocks.FakeFabricNetworkService{}
			fakeSS := &mocks.FakeSignerService{}
			tc.mockSetup(fakeFNS, fakeSS)

			fakeRWSet := &mocks.FakeRWSet{}
			fakeRWSet.BytesReturns(tc.rwsetPayload, nil)

			tx := &Transaction{
				ctx:   t.Context(),
				TTxID: "tx1",
				fns:   fakeFNS,
				rwset: fakeRWSet,
				channel: func() *mocks.FakeChannel {
					ch := &mocks.FakeChannel{}
					ch.MetadataServiceReturns(&mocks.FakeMetadataService{})
					return ch
				}(),
			}

			if tc.withProposal {
				signedProposal := testSignedProposalBytes(t)
				sp, err := newSignedProposal(signedProposal)
				require.NoError(t, err)
				tx.signedProposal = sp
			}

			err := tx.EndorseProposalResponseWithIdentity(testID)
			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
				return
			}

			require.NoError(t, err)
			require.Len(t, tx.TProposalResponses, 1)
			require.Equal(t, 1, fakeSS.GetSignerCallCount())
		})
	}
}

func TestEndorseProposalWithIdentity(t *testing.T) {
	t.Parallel()

	testID := view.Identity([]byte("test-identity"))

	tests := []struct {
		name          string
		mockSetup     func(*mocks.FakeFabricNetworkService, *mocks.FakeSignerService)
		expectedError string
	}{
		{
			name: "success",
			mockSetup: func(fns *mocks.FakeFabricNetworkService, ss *mocks.FakeSignerService) {
				fns.SignerServiceReturns(ss)
				ss.GetSignerReturns(&testSerializableSigner{creator: testID, signRes: []byte("prop-sig")}, nil)
			},
		},
		{
			name: "signer service fails",
			mockSetup: func(fns *mocks.FakeFabricNetworkService, ss *mocks.FakeSignerService) {
				fns.SignerServiceReturns(ss)
				ss.GetSignerReturns(nil, errors.New("identity not found"))
			},
			expectedError: "get signer",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fakeFNS := &mocks.FakeFabricNetworkService{}
			fakeSS := &mocks.FakeSignerService{}
			tc.mockSetup(fakeFNS, fakeSS)

			tx := &Transaction{
				ctx:        t.Context(),
				TTxID:      "tx1",
				TNonce:     []byte("nonce"),
				TCreator:   testID,
				fns:        fakeFNS,
				TChannel:   "mychannel",
				TChaincode: "mycc",
				TFunction:  "invoke",
			}

			err := tx.EndorseProposalWithIdentity(testID)
			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, tx.TProposal)
			require.NotNil(t, tx.TSignedProposal)
			require.NotNil(t, tx.SignedProposal())
			require.Equal(t, 1, fakeSS.GetSignerCallCount())
			require.Equal(t, testID, fakeSS.GetSignerArgsForCall(0))
		})
	}
}
