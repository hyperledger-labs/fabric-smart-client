/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric-x-common/api/applicationpb"
	"github.com/hyperledger/fabric-x-common/api/msppb"
	"github.com/hyperledger/fabric-x-common/protoutil"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	commondriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/transaction/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

//go:generate counterfeiter -o mock/rwset.go --fake-name RWSet github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.RWSet
//go:generate counterfeiter -o mock/vault.go --fake-name Vault github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Vault
//go:generate counterfeiter -o mock/metadata_service.go --fake-name MetadataService github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.MetadataService
//go:generate counterfeiter -o mock/channel.go --fake-name Channel github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Channel

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
		{
			name:          "existing entry has no endorsement",
			existing:      []*peer.ProposalResponse{{}},
			response:      &peer.ProposalResponse{Endorsement: &peer.Endorsement{Endorser: []byte("endorser-1")}},
			expectedCount: 2,
		},
		{
			name:          "incoming response has no endorsement",
			existing:      []*peer.ProposalResponse{{Endorsement: &peer.Endorsement{Endorser: []byte("endorser-1")}}},
			response:      &peer.ProposalResponse{},
			expectedCount: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tx := &Transaction{TProposalResponses: tc.existing}
			var err error
			require.NotPanics(t, func() {
				err = tx.recordProposalResponse(tc.response)
			})
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

func TestStoreTransientPersistsFieldMappings(t *testing.T) {
	t.Parallel()

	origKey, err := rwset.CreateCompositeKey("S", []string{"1234"})
	require.NoError(t, err)
	fmKey, err := rwset.CreateCompositeKey("field_mapping", []string{"asset_transfer", "S", "1234"})
	require.NoError(t, err)
	blob := []byte(`{"_root_":"cHJlaW1hZ2U="}`)
	writeVal := []byte("on-ledger-hash-bytes")

	fakeMDS := &mock.MetadataService{}
	fakeRWSet := &mock.RWSet{}
	fakeRWSet.GetStateReturns(writeVal, nil)
	ch := &mock.Channel{}
	ch.MetadataServiceReturns(fakeMDS)

	tx := &Transaction{
		ctx:         t.Context(),
		TTxID:       "tx1",
		channel:     ch,
		rwSetHandle: fakeRWSet,
		TTransient: driver.TransientMap{
			fmKey:               blob,
			"CertificationType": []byte("ChaincodesCertification"), // must be ignored
		},
	}

	require.NoError(t, tx.StoreTransient())

	// The transient blob itself is still persisted by txid.
	require.Equal(t, 1, fakeMDS.StoreTransientCallCount())

	// The field mapping is persisted under (ns, origKey, sha256(writeVal)).
	require.Equal(t, 1, fakeMDS.PutFieldMappingCallCount())
	_, ns, key, digest, mapping := fakeMDS.PutFieldMappingArgsForCall(0)
	wantDigest := sha256.Sum256(writeVal)
	require.Equal(t, "asset_transfer", ns)
	require.Equal(t, origKey, key)
	require.Equal(t, wantDigest[:], digest)
	require.Equal(t, driver.TransientMap{fmKey: blob}, mapping)

	// The write value came from the tx's own write set (FromIntermediate), for (ns, origKey).
	require.Equal(t, 1, fakeRWSet.GetStateCallCount())
	gotNs, gotKey, gotOpts := fakeRWSet.GetStateArgsForCall(0)
	require.Equal(t, commondriver.Namespace("asset_transfer"), gotNs)
	require.Equal(t, origKey, gotKey)
	require.Equal(t, []commondriver.GetStateOpt{commondriver.FromIntermediate}, gotOpts)
}

func TestStoreTransientNoFieldMappingsIsNoop(t *testing.T) {
	t.Parallel()

	fakeMDS := &mock.MetadataService{}
	fakeRWSet := &mock.RWSet{}
	ch := &mock.Channel{}
	ch.MetadataServiceReturns(fakeMDS)

	tx := &Transaction{
		ctx:         t.Context(),
		TTxID:       "tx1",
		channel:     ch,
		rwSetHandle: fakeRWSet,
		TTransient:  driver.TransientMap{"CertificationType": []byte("x")},
	}

	require.NoError(t, tx.StoreTransient())
	require.Equal(t, 1, fakeMDS.StoreTransientCallCount())
	require.Equal(t, 0, fakeMDS.PutFieldMappingCallCount())
	require.Equal(t, 0, fakeRWSet.GetStateCallCount())
}

// mustSerializedIdentityWithRealCert generates a self-signed ECDSA certificate and
// returns both its PEM encoding and the proto-marshalled msp.SerializedIdentity.
// Use this in tests that exercise code paths which parse the PEM certificate
// (e.g. toMSPSignerIdentityWithCertificateID).
func mustSerializedIdentityWithRealCert(t *testing.T, mspID string) (certPEM, serialized []byte) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: mspID},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	require.NoError(t, err)

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	serialized, err = proto.Marshal(&msp.SerializedIdentity{Mspid: mspID, IdBytes: certPEM})
	require.NoError(t, err)
	return certPEM, serialized
}

func TestToMSPSignerIdentityWithCertificateID(t *testing.T) {
	t.Parallel()

	certPEM, serialized := mustSerializedIdentityWithRealCert(t, "Org1MSP")

	// derive the expected cert ID: SHA-256 of DER bytes, hex-encoded
	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)
	digest := sha256.Sum256(block.Bytes)
	expectedCertID := hex.EncodeToString(digest[:])

	tests := []struct {
		name           string
		identity       view.Identity
		isIdemix       bool
		expectedMSP    string
		expectedCertID string
		expectedError  string
	}{
		{
			name:           "x509 success — cert-ID format",
			identity:       view.Identity(serialized),
			expectedMSP:    "Org1MSP",
			expectedCertID: expectedCertID,
		},
		{
			// Idemix identities carry no X.509 cert; the bytes are returned as-is.
			name:     "idemix identity — pass-through unchanged",
			identity: view.Identity(serialized),
			isIdemix: true,
		},
		{
			name:          "invalid serialized identity",
			identity:      view.Identity([]byte("not-a-protobuf")),
			expectedError: "unmarshal serialized identity",
		},
		{
			name: "non-PEM cert bytes",
			identity: func() view.Identity {
				raw, err := proto.Marshal(&msp.SerializedIdentity{Mspid: "Org1MSP", IdBytes: []byte("not-pem")})
				require.NoError(t, err)
				return view.Identity(raw)
			}(),
			expectedError: "failed to decode PEM certificate",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			id, err := toMSPSignerIdentityWithCertificateID(tc.identity, func(_ string) (bool, error) {
				return tc.isIdemix, nil
			})
			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
				return
			}

			require.NoError(t, err)

			if tc.isIdemix {
				// For Idemix MSPs the original bytes must be returned unchanged.
				require.Equal(t, []byte(tc.identity), id)
				return
			}

			var identity msppb.Identity
			require.NoError(t, proto.Unmarshal(id, &identity))
			require.Equal(t, tc.expectedMSP, identity.GetMspId())
			require.Equal(t, tc.expectedCertID, identity.GetCertificateId())
			require.Empty(t, identity.GetCertificate())
		})
	}
}

// TestToMSPSignerIdentityWithCertificateID_IdemixLookupFails pins the reason the
// predicate returns an error at all: an unknown answer must not fall through to
// the X.509 encoding, which would silently mis-encode an Idemix identity.
func TestToMSPSignerIdentityWithCertificateID_IdemixLookupFails(t *testing.T) {
	t.Parallel()

	_, serialized := mustSerializedIdentityWithRealCert(t, "Org1MSP")
	lookupErr := errors.New("channel [ch1] configuration not loaded")

	id, err := toMSPSignerIdentityWithCertificateID(serialized, func(string) (bool, error) {
		return false, lookupErr
	})

	require.Nil(t, id, "no creator may be produced when the MSP type is unknown")
	require.ErrorIs(t, err, lookupErr)
	require.Contains(t, err.Error(), "Org1MSP")
}

func TestToEndorserIdentityWithCertID(t *testing.T) {
	t.Parallel()

	certPEM, serialized := mustSerializedIdentityWithRealCert(t, "Org2MSP")

	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)
	digest := sha256.Sum256(block.Bytes)
	expectedCertID := hex.EncodeToString(digest[:])

	tests := []struct {
		name           string
		identity       view.Identity
		expectedMSP    string
		expectedCertID string
		expectedError  string
	}{
		{
			name:           "success — returns msppb.Identity with cert-ID",
			identity:       view.Identity(serialized),
			expectedMSP:    "Org2MSP",
			expectedCertID: expectedCertID,
		},
		{
			name:          "invalid serialized identity",
			identity:      view.Identity([]byte("not-a-protobuf")),
			expectedError: "unmarshal serialized identity",
		},
		{
			name: "non-PEM cert bytes",
			identity: func() view.Identity {
				raw, err := proto.Marshal(&msp.SerializedIdentity{Mspid: "Org2MSP", IdBytes: []byte("not-pem")})
				require.NoError(t, err)
				return view.Identity(raw)
			}(),
			expectedError: "failed to decode PEM certificate",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			id, err := toEndorserIdentityWithCertID(tc.identity)
			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, id)
			require.Equal(t, tc.expectedMSP, id.GetMspId())
			require.Equal(t, tc.expectedCertID, id.GetCertificateId())
			// Certificate bytes must be absent — only the hash is stored.
			require.Empty(t, id.GetCertificate())
		})
	}
}

func TestPemToMSPIdentity(t *testing.T) {
	t.Parallel()

	certPEM, _ := mustSerializedIdentityWithRealCert(t, "Org3MSP")

	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)
	digest := sha256.Sum256(block.Bytes)
	expectedCertID := hex.EncodeToString(digest[:])

	tests := []struct {
		name           string
		mspID          string
		raw            []byte
		expectedCertID string
		expectedError  string
	}{
		{
			name:           "success — SHA-256 cert-ID, no raw cert",
			mspID:          "Org3MSP",
			raw:            certPEM,
			expectedCertID: expectedCertID,
		},
		{
			name:          "nil PEM block",
			mspID:         "Org3MSP",
			raw:           []byte("this is not pem"),
			expectedError: "failed to decode PEM certificate",
		},
		{
			name:          "empty input",
			mspID:         "Org3MSP",
			raw:           []byte{},
			expectedError: "failed to decode PEM certificate",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			id, err := pemToMSPIdentity(tc.mspID, tc.raw)
			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
				require.Nil(t, id)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, id)
			require.Equal(t, tc.mspID, id.GetMspId())
			require.Equal(t, tc.expectedCertID, id.GetCertificateId())
			// The raw certificate bytes must not be embedded.
			require.Empty(t, id.GetCertificate())
		})
	}
}

type testSerializableSigner struct {
	creator []byte
	signRes []byte
	signErr error
}

func (s *testSerializableSigner) Sign(_ []byte) ([]byte, error) { return s.signRes, s.signErr }
func (s *testSerializableSigner) Serialize() ([]byte, error)    { return s.creator, nil }

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
			fakeVault := &mock.Vault{}
			fakeRWSet := &mock.RWSet{}
			fakeVault.NewRWSetReturns(fakeRWSet, nil)
			fakeVault.NewRWSetFromBytesReturns(fakeRWSet, nil)

			ch := &mock.Channel{}
			ch.VaultReturns(fakeVault)
			tc.tx.channel = ch

			err := tc.tx.SetRWSet()
			require.NoError(t, err)
			require.Equal(t, tc.expectedNewRWSetCalls, fakeVault.NewRWSetCallCount())
			require.Equal(t, tc.expectedFromBytesCalls, fakeVault.NewRWSetFromBytesCallCount())
			if tc.expectedNewRWSetCalls == 1 {
				_, gotTxID := fakeVault.NewRWSetArgsForCall(0)
				require.Equal(t, tc.tx.TTxID, gotTxID)
			}
			if tc.expectedFromBytesCalls == 1 {
				_, gotTxID, gotBytes := fakeVault.NewRWSetFromBytesArgsForCall(0)
				require.Equal(t, tc.tx.TTxID, gotTxID)
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
		fakeRWSet := &mock.RWSet{}
		fakeRWSet.BytesReturns([]byte("rwset-bytes"), nil)
		fakeRWSet.NamespacesReturns([]commondriver.Namespace{"ns1"})

		tx := &Transaction{TTxID: "tx1", rwSetHandle: fakeRWSet}
		err := tx.Done()
		require.NoError(t, err)
		require.Equal(t, 1, fakeRWSet.DoneCallCount())
		require.Equal(t, []byte("rwset-bytes"), tx.RWSet)
	})

	t.Run("done wraps rwset bytes error", func(t *testing.T) {
		t.Parallel()
		fakeRWSet := &mock.RWSet{}
		fakeRWSet.BytesReturns(nil, errors.New("boom"))

		tx := &Transaction{TTxID: "tx1", rwSetHandle: fakeRWSet}
		err := tx.Done()
		require.Error(t, err)
		require.Contains(t, err.Error(), "marshalling rws")
	})

	t.Run("raw serializes current rwset", func(t *testing.T) {
		t.Parallel()
		fakeRWSet := &mock.RWSet{}
		fakeRWSet.BytesReturns([]byte("raw-rwset"), nil)

		tx := &Transaction{TTxID: "tx1", rwSetHandle: fakeRWSet}
		raw, err := tx.Raw()
		require.NoError(t, err)
		require.Contains(t, string(raw), `"RWSet":"cmF3LXJ3c2V0"`)
	})

	t.Run("raw wraps rwset bytes error", func(t *testing.T) {
		t.Parallel()
		fakeRWSet := &mock.RWSet{}
		fakeRWSet.BytesReturns(nil, errors.New("boom"))

		tx := &Transaction{TTxID: "tx1", rwSetHandle: fakeRWSet}
		_, err := tx.Raw()
		require.Error(t, err)
		require.Contains(t, err.Error(), "marshalling rws")
	})

	t.Run("get rwset returns existing one", func(t *testing.T) {
		t.Parallel()
		fakeRWSet := &mock.RWSet{}
		tx := &Transaction{rwSetHandle: fakeRWSet}
		got, err := tx.GetRWSet()
		require.NoError(t, err)
		require.Same(t, fakeRWSet, got)
	})

	t.Run("get rwset initializes it", func(t *testing.T) {
		t.Parallel()
		fakeVault := &mock.Vault{}
		fakeRWSet := &mock.RWSet{}
		fakeVault.NewRWSetReturns(fakeRWSet, nil)

		tx := &Transaction{ctx: t.Context(), channel: func() *mock.Channel { ch := &mock.Channel{}; ch.VaultReturns(fakeVault); return ch }(), TTxID: "tx2"}
		got, err := tx.GetRWSet()
		require.NoError(t, err)
		require.Equal(t, 1, fakeVault.NewRWSetCallCount())
		require.Same(t, fakeRWSet, got)
	})

	t.Run("close terminates and clears rwset", func(t *testing.T) {
		t.Parallel()
		fakeRWSet := &mock.RWSet{}
		tx := &Transaction{TTxID: "tx3", rwSetHandle: fakeRWSet}
		tx.Close()
		require.Equal(t, 1, fakeRWSet.DoneCallCount())
		require.Nil(t, tx.RWS())
	})
}

func TestTransactionBytesNoTransient(t *testing.T) {
	t.Parallel()

	fakeRWSet := &mock.RWSet{}
	fakeRWSet.BytesReturns([]byte("rwset-bytes"), nil)
	fakeRWSet.NamespacesReturns([]commondriver.Namespace{"ns1"})

	tx := &Transaction{TTxID: "tx1", TTransient: driver.TransientMap{"secret": []byte("value")}, rwSetHandle: fakeRWSet}
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
			fakeFNS := &mock.FabricNetworkService{}
			fakeFNS.ChannelReturns(&mock.Channel{}, tc.channelErr)

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
		fakeFNS := &mock.FabricNetworkService{}
		fakeSS := &mock.SignerService{}
		fakeSigner := &mock.Signer{}
		fakeSigner.SignReturns([]byte("sig"), nil)
		fakeFNS.SignerServiceReturns(fakeSS)
		fakeSS.GetSignerReturns(fakeSigner, nil)

		tx := &Transaction{
			ctx:            t.Context(),
			fns:            fakeFNS,
			signedProposal: &SignedProposal{},
			channel: func() *mock.Channel {
				ch := &mock.Channel{}
				ch.MetadataServiceReturns(&mock.MetadataService{})
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

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		_, signerIdentityRaw := mustSerializedIdentityWithRealCert(t, "Org1MSP")

		txPayload := &applicationpb.Tx{Namespaces: []*applicationpb.TxNamespace{{NsId: "ns1"}, {NsId: "ns2"}}}
		rwsetBytes, err := proto.Marshal(txPayload)
		require.NoError(t, err)

		fakeRWSet := &mock.RWSet{}
		fakeRWSet.BytesReturns(rwsetBytes, nil)
		fakeSigner := &testSerializableSigner{creator: signerIdentityRaw, signRes: []byte("signature-data")}

		signedProposal := testSignedProposalBytes(t)
		sp, err := newSignedProposal(signedProposal)
		require.NoError(t, err)

		tx := &Transaction{TTxID: "tx1", signedProposal: sp, rwSetHandle: fakeRWSet}
		resp, err := tx.getProposalResponse(fakeSigner)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, int32(200), resp.Response.Status)
		require.Equal(t, []byte("tx1"), resp.Response.Payload)
		require.Equal(t, rwsetBytes, resp.Payload)
		require.NotNil(t, resp.Endorsement)
		require.Equal(t, signerIdentityRaw, resp.Endorsement.Endorser)

		// verify the endorsement identity embedded in the payload uses cert-ID format
		endorsements, err := unmarshalEndorsementsFromProposalResponse(resp.Endorsement.Signature)
		require.NoError(t, err)
		require.Len(t, endorsements, 2)
		eid := endorsements[0].EndorsementsWithIdentity[0]
		require.NotEmpty(t, eid.Identity.GetCertificateId())
		require.Empty(t, eid.Identity.GetCertificate())
	})

	t.Run("error when proposal already has endorsements", func(t *testing.T) {
		t.Parallel()
		txPayload := &applicationpb.Tx{Namespaces: []*applicationpb.TxNamespace{{NsId: "ns1"}}, Endorsements: []*applicationpb.Endorsements{{}}}
		rwsetBytes, err := proto.Marshal(txPayload)
		require.NoError(t, err)

		fakeRWSet := &mock.RWSet{}
		fakeRWSet.BytesReturns(rwsetBytes, nil)

		tx := &Transaction{TTxID: "tx1", signedProposal: &SignedProposal{}, rwSetHandle: fakeRWSet}
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

	_, signerIdentityRaw := mustSerializedIdentityWithRealCert(t, "Org1MSP")
	testID := view.Identity(signerIdentityRaw)

	txPayload := &applicationpb.Tx{Namespaces: []*applicationpb.TxNamespace{{NsId: "ns1"}}}
	validRWSet, err := proto.Marshal(txPayload)
	require.NoError(t, err)

	tests := []struct {
		name          string
		mockSetup     func(*mock.FabricNetworkService, *mock.SignerService)
		rwsetPayload  []byte
		withProposal  bool
		expectedError string
	}{
		{
			name: "success",
			mockSetup: func(fns *mock.FabricNetworkService, ss *mock.SignerService) {
				fns.SignerServiceReturns(ss)
				fakeSigner := &mock.Signer{}
				fakeSigner.SignReturns([]byte("sig"), nil)
				ss.GetSignerReturns(fakeSigner, nil)
			},
			rwsetPayload: validRWSet,
			withProposal: true,
		},
		{
			name: "signer service fails",
			mockSetup: func(fns *mock.FabricNetworkService, ss *mock.SignerService) {
				fns.SignerServiceReturns(ss)
				ss.GetSignerReturns(nil, errors.New("signer not found"))
			},
			rwsetPayload:  validRWSet,
			expectedError: "get signer",
		},
		{
			name: "proposal response generation fails",
			mockSetup: func(fns *mock.FabricNetworkService, ss *mock.SignerService) {
				fns.SignerServiceReturns(ss)
				fakeSigner := &mock.Signer{}
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
			fakeFNS := &mock.FabricNetworkService{}
			fakeSS := &mock.SignerService{}
			tc.mockSetup(fakeFNS, fakeSS)

			fakeRWSet := &mock.RWSet{}
			fakeRWSet.BytesReturns(tc.rwsetPayload, nil)

			tx := &Transaction{
				ctx:         t.Context(),
				TTxID:       "tx1",
				fns:         fakeFNS,
				rwSetHandle: fakeRWSet,
				channel: func() *mock.Channel {
					ch := &mock.Channel{}
					ch.MetadataServiceReturns(&mock.MetadataService{})
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
		mockSetup     func(*mock.FabricNetworkService, *mock.SignerService)
		expectedError string
	}{
		{
			name: "success",
			mockSetup: func(fns *mock.FabricNetworkService, ss *mock.SignerService) {
				fns.SignerServiceReturns(ss)
				ss.GetSignerReturns(&testSerializableSigner{creator: testID, signRes: []byte("prop-sig")}, nil)
			},
		},
		{
			name: "signer service fails",
			mockSetup: func(fns *mock.FabricNetworkService, ss *mock.SignerService) {
				fns.SignerServiceReturns(ss)
				ss.GetSignerReturns(nil, errors.New("identity not found"))
			},
			expectedError: "get signer",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fakeFNS := &mock.FabricNetworkService{}
			fakeSS := &mock.SignerService{}
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

// serializedRWSet builds a valid FabricX-encoded read-write set, so tests exercise the
// dedup path with the kind of payload it actually sees rather than an opaque blob.
func serializedRWSet(t *testing.T) []byte {
	t.Helper()
	raw, err := proto.Marshal(&applicationpb.Tx{
		Namespaces: []*applicationpb.TxNamespace{{
			NsId:      "ns1",
			NsVersion: 7,
			BlindWrites: []*applicationpb.Write{
				{Key: []byte("key1"), Value: []byte("val1")},
			},
		}},
	})
	require.NoError(t, err)
	return raw
}

func TestTransaction_DeduplicateRWSet(t *testing.T) {
	t.Parallel()
	rwsetBytes := serializedRWSet(t)

	tx := &Transaction{
		RWSet: rwsetBytes,
		TProposalResponses: []*peer.ProposalResponse{
			{Payload: rwsetBytes},
		},
	}

	err := tx.Done()
	require.NoError(t, err)

	// Since TProposalResponses contains the exact same bytes, RWSet should be cleared
	require.Nil(t, tx.RWSet)

	// Test non-matching payload
	tx2 := &Transaction{
		RWSet: rwsetBytes,
		TProposalResponses: []*peer.ProposalResponse{
			{Payload: []byte("different payload")},
		},
	}

	err = tx2.Done()
	require.NoError(t, err)

	// Should not be cleared
	require.NotNil(t, tx2.RWSet)
	require.Equal(t, rwsetBytes, tx2.RWSet)
}

// Dropping the RWSet field is only safe if the receiving side can rebuild an identical
// read-write set from the proposal response payload. This walks the full path the dedup
// relies on: serialize with the field dropped, ship the JSON, and reconstruct.
func TestTransaction_DeduplicatedRWSetRoundTrips(t *testing.T) {
	t.Parallel()
	rwsetBytes := serializedRWSet(t)

	sender := &Transaction{
		ctx:   t.Context(),
		TTxID: "tx1",
		RWSet: rwsetBytes,
		TProposalResponses: []*peer.ProposalResponse{
			{Payload: rwsetBytes},
		},
	}

	shipped, err := sender.Bytes()
	require.NoError(t, err)

	// The redundant payload must be gone from the wire form, while the key itself stays
	// present as an explicit null so the JSON shape is unchanged for every consumer.
	var onTheWire map[string]any
	require.NoError(t, json.Unmarshal(shipped, &onTheWire))
	require.Contains(t, onTheWire, "RWSet", "the key must still be emitted")
	require.Nil(t, onTheWire["RWSet"], "the duplicated rwset must not be carried twice")

	// The receiver reconstructs from the JSON alone.
	fakeVault := &mock.Vault{}
	fakeVault.NewRWSetFromBytesReturns(&mock.RWSet{}, nil)
	ch := &mock.Channel{}
	ch.VaultReturns(fakeVault)

	// Decoded directly rather than through SetFromBytes, which additionally resolves the
	// channel through the network service; the invariant under test is only that the
	// dropped field is recoverable from the payload.
	receiver := &Transaction{ctx: t.Context(), channel: ch}
	require.NoError(t, json.Unmarshal(shipped, receiver))
	require.Nil(t, receiver.RWSet, "the field arrives absent")

	require.NoError(t, receiver.SetRWSet())
	require.Equal(t, 1, fakeVault.NewRWSetFromBytesCallCount())
	_, _, gotBytes := fakeVault.NewRWSetFromBytesArgsForCall(0)
	require.Equal(t, rwsetBytes, gotBytes,
		"the reconstructed rwset must be built from the exact bytes the sender dropped")
}

// rwSetWithWrites builds a serialized RWSet carrying n blind writes of valueSize bytes
// each, so a benchmark can vary the payload independently of the rest of the transaction.
func rwSetWithWrites(tb testing.TB, n, valueSize int) []byte {
	tb.Helper()
	writes := make([]*applicationpb.Write, 0, n)
	for i := range n {
		value := make([]byte, valueSize)
		for j := range value {
			value[j] = byte(i + j)
		}
		writes = append(writes, &applicationpb.Write{Key: fmt.Appendf(nil, "key%08d", i), Value: value})
	}
	raw, err := proto.Marshal(&applicationpb.Tx{
		Namespaces: []*applicationpb.TxNamespace{{NsId: "ns1", NsVersion: 7, BlindWrites: writes}},
	})
	require.NoError(tb, err)
	return raw
}

// BenchmarkTransactionBytes measures the JSON encoding of an endorsed transaction, which
// is what every send pays. "Duplicated" is the pre-deduplication behaviour: the rwset
// reaches the wire twice, once as RWSet and once as the proposal response payload, each
// base64-expanded by a third. "Deduplicated" is Transaction.Bytes() as it stands now.
//
// bytes/op reports the size of the encoded transaction, so the saving is readable
// alongside the allocation cost of producing it.
func BenchmarkTransactionBytes(b *testing.B) {
	cases := []struct {
		name      string
		writes    int
		valueSize int
	}{
		// The shape the PR description quotes a saving for.
		{name: "10Writes", writes: 10, valueSize: 32},
		// The payload size the reproducer in #1599 drives, which is what #1628 is about.
		{name: "256KiB", writes: 2048, valueSize: 128},
	}

	for _, tc := range cases {
		raw := rwSetWithWrites(b, tc.writes, tc.valueSize)
		newTx := func() *Transaction {
			return &Transaction{
				TTxID:              "tx1",
				TProposalResponses: []*peer.ProposalResponse{{Payload: raw}},
			}
		}

		b.Run(tc.name, func(b *testing.B) {
			b.Run("Duplicated", func(b *testing.B) {
				tx := newTx()
				b.ReportAllocs()
				var out []byte
				for b.Loop() {
					tx.RWSet = raw
					var err error
					out, err = json.Marshal(tx)
					if err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(len(out)), "bytes/op")
			})

			b.Run("Deduplicated", func(b *testing.B) {
				tx := newTx()
				b.ReportAllocs()
				var out []byte
				for b.Loop() {
					tx.RWSet = raw
					var err error
					out, err = tx.Bytes()
					if err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(len(out)), "bytes/op")
			})
		})
	}
}

// endorsingChannel returns a channel whose metadata service accepts StoreTransient.
func endorsingChannel() *mock.Channel {
	ch := &mock.Channel{}
	ch.MetadataServiceReturns(&mock.MetadataService{})
	return ch
}

// fnsWithSigner returns a network service whose signer service hands out signer.
func fnsWithSigner(signer driver.Signer, err error) (*mock.FabricNetworkService, *mock.SignerService) {
	fns := &mock.FabricNetworkService{}
	ss := &mock.SignerService{}
	fns.SignerServiceReturns(ss)
	ss.GetSignerReturns(signer, err)
	return fns, ss
}

func TestEndorseWithSigner(t *testing.T) {
	t.Parallel()

	_, creator := mustSerializedIdentityWithRealCert(t, "Org1MSP")

	newTx := func(ch *mock.Channel) *Transaction {
		return &Transaction{
			ctx:        t.Context(),
			TTxID:      "tx1",
			TNonce:     []byte("nonce"),
			TCreator:   creator,
			TChannel:   "channel1",
			TChaincode: "cc",
			TFunction:  "invoke",
			channel:    ch,
		}
	}

	t.Run("generates a proposal and no response without rwset", func(t *testing.T) {
		t.Parallel()
		tx := newTx(endorsingChannel())

		require.NoError(t, tx.EndorseWithSigner(creator, &testSerializableSigner{creator: creator, signRes: []byte("sig")}))
		require.NotNil(t, tx.SignedProposal())
		require.NotNil(t, tx.TSignedProposal)
		require.Equal(t, []byte("sig"), tx.SignedProposal().Signature())
		require.Empty(t, tx.TProposalResponses)
	})

	t.Run("endorses one namespace per rwset namespace", func(t *testing.T) {
		t.Parallel()
		rawTx := mustRawTx(t, &applicationpb.Tx{Namespaces: []*applicationpb.TxNamespace{{NsId: "ns1"}, {NsId: "ns2"}}})
		fakeRWSet := &mock.RWSet{}
		fakeRWSet.BytesReturns(rawTx, nil)
		tx := newTx(endorsingChannel())
		tx.rwSetHandle = fakeRWSet

		require.NoError(t, tx.EndorseWithSigner(creator, &testSerializableSigner{creator: creator, signRes: []byte("sig")}))
		require.NotNil(t, tx.SignedProposal())
		require.Len(t, tx.TProposalResponses, 1)
		resp := tx.TProposalResponses[0]
		require.Equal(t, rawTx, resp.Payload)
		require.Equal(t, creator, resp.Endorsement.Endorser)

		endorsements, err := unmarshalEndorsementsFromProposalResponse(resp.Endorsement.Signature)
		require.NoError(t, err)
		require.Len(t, endorsements, 2)
		for _, e := range endorsements {
			require.Len(t, e.EndorsementsWithIdentity, 1)
			require.Equal(t, []byte("sig"), e.EndorsementsWithIdentity[0].Endorsement)
		}

		// The deferred Close terminates the simulation.
		require.Equal(t, 1, fakeRWSet.DoneCallCount())
		require.Nil(t, tx.RWS())
	})

	t.Run("wraps proposal generation failure", func(t *testing.T) {
		t.Parallel()
		tx := newTx(endorsingChannel())

		err := tx.EndorseWithSigner(creator, &testSerializableSigner{creator: creator, signErr: errors.New("boom")})
		require.ErrorContains(t, err, "generate signed proposal")
	})

	t.Run("wraps proposal response failure", func(t *testing.T) {
		t.Parallel()
		fakeRWSet := &mock.RWSet{}
		fakeRWSet.BytesReturns(nil, errors.New("boom"))
		tx := newTx(endorsingChannel())
		tx.rwSetHandle = fakeRWSet

		err := tx.EndorseWithSigner(creator, &testSerializableSigner{creator: creator, signRes: []byte("sig")})
		require.ErrorContains(t, err, "getting proposal response")
		require.Equal(t, 1, fakeRWSet.DoneCallCount())
	})

	t.Run("wraps store transient failure", func(t *testing.T) {
		t.Parallel()
		mds := &mock.MetadataService{}
		mds.StoreTransientReturns(errors.New("boom"))
		ch := &mock.Channel{}
		ch.MetadataServiceReturns(mds)
		tx := newTx(ch)

		err := tx.EndorseWithSigner(creator, &testSerializableSigner{creator: creator, signRes: []byte("sig")})
		require.ErrorContains(t, err, "failed storing transient")
	})
}

// TestEndorseDelegatesWithCreator pins that the identity-less endorse methods sign
// with the transaction creator.
func TestEndorseDelegatesWithCreator(t *testing.T) {
	t.Parallel()

	_, creator := mustSerializedIdentityWithRealCert(t, "Org1MSP")
	rawTx := mustRawTx(t, &applicationpb.Tx{Namespaces: []*applicationpb.TxNamespace{{NsId: "ns1"}}})

	tests := []struct {
		name    string
		endorse func(*Transaction) error
	}{
		{name: "Endorse", endorse: (*Transaction).Endorse},
		{name: "EndorseProposal", endorse: (*Transaction).EndorseProposal},
		{name: "EndorseProposalResponse", endorse: (*Transaction).EndorseProposalResponse},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fakeSigner := &mock.Signer{}
			fakeSigner.SignReturns([]byte("sig"), nil)
			fns, ss := fnsWithSigner(fakeSigner, nil)
			fakeRWSet := &mock.RWSet{}
			fakeRWSet.BytesReturns(rawTx, nil)
			sp, err := newSignedProposal(testSignedProposalBytes(t))
			require.NoError(t, err)

			tx := &Transaction{
				ctx:            t.Context(),
				TTxID:          "tx1",
				TNonce:         []byte("nonce"),
				TCreator:       creator,
				TChannel:       "channel1",
				TChaincode:     "cc",
				TFunction:      "invoke",
				fns:            fns,
				channel:        endorsingChannel(),
				signedProposal: sp,
				rwSetHandle:    fakeRWSet,
			}

			require.NoError(t, tc.endorse(tx))
			require.Equal(t, 1, ss.GetSignerCallCount())
			require.Equal(t, view.Identity(creator), ss.GetSignerArgsForCall(0))
		})
	}
}

func TestEndorseWithIdentityErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		fns           driver.FabricNetworkService
		expectedError string
	}{
		{
			name:          "nil fabric network service",
			expectedError: "fabric network service not initialized",
		},
		{
			name:          "nil signer service",
			fns:           &mock.FabricNetworkService{},
			expectedError: "signer service not initialized",
		},
		{
			name: "get signer fails",
			fns: func() driver.FabricNetworkService {
				fns, _ := fnsWithSigner(nil, errors.New("boom"))
				return fns
			}(),
			expectedError: "get signer identity",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tx := &Transaction{fns: tc.fns}
			require.ErrorContains(t, tx.EndorseWithIdentity(view.Identity("id")), tc.expectedError)
		})
	}
}

func TestGetProposalResponseErrors(t *testing.T) {
	t.Parallel()

	_, creator := mustSerializedIdentityWithRealCert(t, "Org1MSP")
	rawTx := mustRawTx(t, &applicationpb.Tx{Namespaces: []*applicationpb.TxNamespace{{NsId: "ns1"}}})
	sp, err := newSignedProposal(testSignedProposalBytes(t))
	require.NoError(t, err)

	tests := []struct {
		name          string
		setup         func(*Transaction)
		signer        *testSerializableSigner
		expectedError string
	}{
		{
			name: "get rwset fails",
			setup: func(tx *Transaction) {
				v := &mock.Vault{}
				v.NewRWSetReturns(nil, errors.New("boom"))
				ch := &mock.Channel{}
				ch.VaultReturns(v)
				tx.channel = ch
			},
			signer:        &testSerializableSigner{creator: creator},
			expectedError: "getting rwset for [txID=tx1]",
		},
		{
			name: "rwset bytes fails",
			setup: func(tx *Transaction) {
				rws := &mock.RWSet{}
				rws.BytesReturns(nil, errors.New("boom"))
				tx.rwSetHandle = rws
			},
			signer:        &testSerializableSigner{creator: creator},
			expectedError: "serializing rws for [txID=tx1]",
		},
		{
			name: "payload is not a tx",
			setup: func(tx *Transaction) {
				rws := &mock.RWSet{}
				rws.BytesReturns([]byte("not-a-tx"), nil)
				tx.rwSetHandle = rws
			},
			signer:        &testSerializableSigner{creator: creator},
			expectedError: "unmarshalling tx [txID=tx1]",
		},
		{
			name: "creator is not an x509 identity",
			setup: func(tx *Transaction) {
				rws := &mock.RWSet{}
				rws.BytesReturns(rawTx, nil)
				tx.rwSetHandle = rws
			},
			signer:        &testSerializableSigner{creator: []byte("not-an-identity")},
			expectedError: "converting signer identity to msp identity",
		},
		{
			name: "sign fails",
			setup: func(tx *Transaction) {
				rws := &mock.RWSet{}
				rws.BytesReturns(rawTx, nil)
				tx.rwSetHandle = rws
			},
			signer:        &testSerializableSigner{creator: creator, signErr: errors.New("boom")},
			expectedError: "signing transaction [txID=tx1] [ns=ns1]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tx := &Transaction{ctx: t.Context(), TTxID: "tx1", signedProposal: sp}
			tc.setup(tx)
			resp, err := tx.getProposalResponse(tc.signer)
			require.ErrorContains(t, err, tc.expectedError)
			require.Nil(t, resp)
		})
	}
}

// mustEndorserEnvelope builds a signed HeaderType_ENDORSER_TRANSACTION envelope with a
// single proposal response, as produced by a Fabric peer. With endorsedParams, the response
// endorses a proposal carrying those parameters instead of the envelope's own.
func mustEndorserEnvelope(t *testing.T, creator []byte, endorsedParams ...[]byte) []byte {
	t.Helper()

	signer := &testSerializableSigner{creator: creator, signRes: []byte("sig")}
	src := &Transaction{
		TTxID:             "tx-env",
		TNonce:            []byte("nonce"),
		TCreator:          creator,
		TChannel:          "channel1",
		TChaincode:        "cc",
		TChaincodeVersion: "v1",
		TFunction:         "invoke",
		TParameters:       [][]byte{[]byte("a"), []byte("b")},
	}
	require.NoError(t, src.generateProposal(signer))
	endorsed := src.TProposal
	if endorsedParams != nil {
		other := *src
		other.TParameters = endorsedParams
		require.NoError(t, other.generateProposal(signer))
		endorsed = other.TProposal
	}

	resp, err := protoutil.CreateProposalResponse(
		endorsed.Header, endorsed.Payload,
		&peer.Response{Status: 200}, []byte("results"), nil,
		&peer.ChaincodeID{Name: "cc", Version: "v1"}, signer,
	)
	require.NoError(t, err)
	env, err := protoutil.CreateSignedTx(src.TProposal, signer, resp)
	require.NoError(t, err)
	raw, err := proto.Marshal(env)
	require.NoError(t, err)
	return raw
}

func TestTransactionSetFromEnvelopeBytesEndorserTransaction(t *testing.T) {
	t.Parallel()

	creator := []byte("creator")
	raw := mustEndorserEnvelope(t, creator)

	t.Run("sets every field", func(t *testing.T) {
		t.Parallel()
		fakeFNS := &mock.FabricNetworkService{}
		ch := &mock.Channel{}
		fakeFNS.ChannelReturns(ch, nil)

		tx := &Transaction{fns: fakeFNS}
		require.NoError(t, tx.SetFromEnvelopeBytes(raw))
		require.Equal(t, "tx-env", tx.ID())
		require.Equal(t, []byte("nonce"), tx.Nonce())
		require.Equal(t, "channel1", tx.Channel())
		require.Equal(t, "cc", tx.Chaincode())
		require.Equal(t, "v1", tx.ChaincodeVersion())
		require.Equal(t, "invoke", tx.Function())
		require.Equal(t, [][]byte{[]byte("a"), []byte("b")}, tx.Parameters())
		require.Equal(t, view.Identity(creator), tx.Creator())
		require.Len(t, tx.TProposalResponses, 1)
		require.Equal(t, creator, tx.TProposalResponses[0].Endorsement.Endorser)
		require.Equal(t, "channel1", fakeFNS.ChannelArgsForCall(0))
		require.Same(t, ch, tx.channel)
	})

	t.Run("keeps an existing creator", func(t *testing.T) {
		t.Parallel()
		fakeFNS := &mock.FabricNetworkService{}
		fakeFNS.ChannelReturns(&mock.Channel{}, nil)

		tx := &Transaction{fns: fakeFNS, TCreator: view.Identity("existing")}
		require.NoError(t, tx.SetFromEnvelopeBytes(raw))
		require.Equal(t, view.Identity("existing"), tx.Creator())
	})

	t.Run("rejects arguments the endorsement does not cover", func(t *testing.T) {
		t.Parallel()
		tx := &Transaction{fns: &mock.FabricNetworkService{}}
		err := tx.SetFromEnvelopeBytes(mustEndorserEnvelope(t, creator, []byte("other")))
		require.ErrorContains(t, err, "envelope proposal hash does not match the endorsed proposal hash")
	})

	t.Run("wraps channel lookup failure", func(t *testing.T) {
		t.Parallel()
		fakeFNS := &mock.FabricNetworkService{}
		fakeFNS.ChannelReturns(nil, errors.New("boom"))

		tx := &Transaction{fns: fakeFNS}
		require.ErrorContains(t, tx.SetFromEnvelopeBytes(raw), "get channel [channel1]")
	})

	// The generic unpacker accepts only endorser transactions, so the
	// HeaderType_MESSAGE envelope this package assembles for ordering is rejected.
	t.Run("rejects an envelope from Envelope()", func(t *testing.T) {
		t.Parallel()
		src, _ := envelopeReadyTx(t)
		env, err := src.Envelope()
		require.NoError(t, err)
		envRaw, err := env.Bytes()
		require.NoError(t, err)

		tx := &Transaction{fns: &mock.FabricNetworkService{}}
		err = tx.SetFromEnvelopeBytes(envRaw)
		require.ErrorContains(t, err, "only EndorserClient Transactions are supported")
	})
}

func TestTransactionSetFromBytesSignedProposal(t *testing.T) {
	t.Parallel()

	t.Run("fills fields from the signed proposal", func(t *testing.T) {
		t.Parallel()
		raw, err := json.Marshal(&Transaction{TSignedProposal: testSignedProposalBytes(t)})
		require.NoError(t, err)
		fakeFNS := &mock.FabricNetworkService{}
		fakeFNS.ChannelReturns(&mock.Channel{}, nil)

		tx := &Transaction{fns: fakeFNS}
		require.NoError(t, tx.SetFromBytes(raw))
		require.Equal(t, "tx-signed", tx.ID())
		require.Equal(t, []byte("nonce"), tx.Nonce())
		require.Equal(t, "channel1", tx.Channel())
		require.Equal(t, "cc", tx.Chaincode())
		require.Equal(t, "v1", tx.ChaincodeVersion())
		require.Equal(t, "invoke", tx.Function())
		require.Equal(t, [][]byte{[]byte("a"), []byte("b")}, tx.Parameters())
		require.NotEmpty(t, tx.Creator())
		require.NotNil(t, tx.TProposal)
		require.Equal(t, "cc", tx.SignedProposal().ChaincodeName())
	})

	t.Run("invalid signed proposal", func(t *testing.T) {
		t.Parallel()
		raw, err := json.Marshal(&Transaction{TSignedProposal: &peer.SignedProposal{ProposalBytes: []byte("garbage")}})
		require.NoError(t, err)

		tx := &Transaction{fns: &mock.FabricNetworkService{}}
		require.ErrorContains(t, tx.SetFromBytes(raw), "unpacking proposal")
	})
}

func TestTransactionFromSignedProposal(t *testing.T) {
	t.Parallel()

	t.Run("rebuilds signed proposal", func(t *testing.T) {
		t.Parallel()
		dst := &Transaction{}
		require.NoError(t, dst.From(&Transaction{TSignedProposal: testSignedProposalBytes(t)}))
		require.NotNil(t, dst.SignedProposal())
		require.Equal(t, "cc", dst.SignedProposal().ChaincodeName())
	})

	t.Run("signed proposal cannot be unpacked", func(t *testing.T) {
		t.Parallel()
		dst := &Transaction{}
		require.Error(t, dst.From(&Transaction{TSignedProposal: &peer.SignedProposal{ProposalBytes: []byte("garbage")}}))
	})
}

func TestProposalHasBeenEndorsedBy(t *testing.T) {
	t.Parallel()

	sp, err := newSignedProposal(testSignedProposalBytes(t))
	require.NoError(t, err)
	party := view.Identity("party")

	newTx := func(verifier driver.Verifier, verifierErr error) (*Transaction, *mock.ChannelMembership) {
		cm := &mock.ChannelMembership{}
		cm.GetVerifierReturns(verifier, verifierErr)
		ch := &mock.Channel{}
		ch.ChannelMembershipReturns(cm)
		return &Transaction{channel: ch, signedProposal: sp}, cm
	}

	t.Run("valid signature", func(t *testing.T) {
		t.Parallel()
		v := &mock.Verifier{}
		tx, cm := newTx(v, nil)

		require.NoError(t, tx.ProposalHasBeenEndorsedBy(party))
		require.Equal(t, party, cm.GetVerifierArgsForCall(0))
		msg, sig := v.VerifyArgsForCall(0)
		require.Equal(t, sp.ProposalBytes(), msg)
		require.Equal(t, sp.Signature(), sig)
	})

	t.Run("get verifier fails", func(t *testing.T) {
		t.Parallel()
		tx, _ := newTx(nil, errors.New("boom"))
		require.ErrorContains(t, tx.ProposalHasBeenEndorsedBy(party), "get verifier from channel membership")
	})

	t.Run("verify fails", func(t *testing.T) {
		t.Parallel()
		v := &mock.Verifier{}
		v.VerifyReturns(errors.New("bad signature"))
		tx, _ := newTx(v, nil)
		require.ErrorContains(t, tx.ProposalHasBeenEndorsedBy(party), "bad signature")
	})

	t.Run("nil signed proposal", func(t *testing.T) {
		t.Parallel()
		tx, cm := newTx(&mock.Verifier{}, nil)
		tx.signedProposal = nil
		tx.TTxID = "tx1"

		var err error
		require.NotPanics(t, func() { err = tx.ProposalHasBeenEndorsedBy(party) })
		require.ErrorContains(t, err, "transaction [txID=tx1] has no signed proposal")
		require.Equal(t, 0, cm.GetVerifierCallCount())
	})
}

func TestTransactionSetRWSetVaultErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		tx            *Transaction
		expectedError string
	}{
		{
			name:          "from proposal response",
			tx:            &Transaction{TProposalResponses: []*peer.ProposalResponse{{Payload: []byte("malformed")}}},
			expectedError: "populate rws from proposal response",
		},
		{
			name:          "from rwset",
			tx:            &Transaction{RWSet: []byte("malformed")},
			expectedError: "populate rws from existing rws",
		},
		{
			name:          "from scratch",
			tx:            &Transaction{},
			expectedError: "create fresh rws",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v := &mock.Vault{}
			v.NewRWSetReturns(nil, errors.New("boom"))
			v.NewRWSetFromBytesReturns(nil, errors.New("boom"))
			ch := &mock.Channel{}
			ch.VaultReturns(v)
			tc.tx.ctx = t.Context()
			tc.tx.channel = ch

			require.ErrorContains(t, tc.tx.SetRWSet(), tc.expectedError)
			_, err := tc.tx.GetRWSet()
			require.ErrorContains(t, err, tc.expectedError)
			require.Nil(t, tc.tx.RWS())
		})
	}
}

func TestTransactionBytesPropagatesRWSetError(t *testing.T) {
	t.Parallel()

	for name, bytesFn := range map[string]func(*Transaction) ([]byte, error){
		"Bytes":            (*Transaction).Bytes,
		"BytesNoTransient": (*Transaction).BytesNoTransient,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rws := &mock.RWSet{}
			rws.BytesReturns(nil, errors.New("boom"))
			raw, err := bytesFn(&Transaction{rwSetHandle: rws})
			require.ErrorContains(t, err, "marshalling rws")
			require.Nil(t, raw)
		})
	}

	t.Run("BytesNoTransient invalid signed proposal", func(t *testing.T) {
		t.Parallel()
		tx := &Transaction{TSignedProposal: &peer.SignedProposal{ProposalBytes: []byte("garbage")}}
		_, err := tx.BytesNoTransient()
		require.Error(t, err)
	})
}

func TestStoreTransientErrors(t *testing.T) {
	t.Parallel()

	fmKey, err := rwset.CreateCompositeKey("field_mapping", []string{"ns", "S", "1234"})
	require.NoError(t, err)

	newTx := func(mds *mock.MetadataService, rws *mock.RWSet) *Transaction {
		ch := &mock.Channel{}
		ch.MetadataServiceReturns(mds)
		return &Transaction{
			ctx:         t.Context(),
			TTxID:       "tx1",
			channel:     ch,
			rwSetHandle: rws,
			TTransient:  driver.TransientMap{fmKey: []byte("blob")},
		}
	}

	t.Run("store transient fails", func(t *testing.T) {
		t.Parallel()
		mds := &mock.MetadataService{}
		mds.StoreTransientReturns(errors.New("boom"))
		require.ErrorContains(t, newTx(mds, &mock.RWSet{}).StoreTransient(), "boom")
		require.Equal(t, 0, mds.PutFieldMappingCallCount())
	})

	t.Run("field mapping without local write is skipped", func(t *testing.T) {
		t.Parallel()
		mds := &mock.MetadataService{}
		rws := &mock.RWSet{}
		rws.GetStateReturns(nil, nil)
		require.NoError(t, newTx(mds, rws).StoreTransient())
		require.Equal(t, 1, rws.GetStateCallCount())
		require.Equal(t, 0, mds.PutFieldMappingCallCount())
	})

	t.Run("put field mapping fails", func(t *testing.T) {
		t.Parallel()
		mds := &mock.MetadataService{}
		mds.PutFieldMappingReturns(errors.New("boom"))
		rws := &mock.RWSet{}
		rws.GetStateReturns([]byte("value"), nil)
		require.ErrorContains(t, newTx(mds, rws).StoreTransient(), "failed persisting field mapping for [ns:")
	})
}

func TestTransactionProposal(t *testing.T) {
	t.Parallel()

	tx := &Transaction{TProposal: &peer.Proposal{Header: []byte("header"), Payload: []byte("payload")}}
	p := tx.Proposal()
	require.Equal(t, []byte("header"), p.Header())
	require.Equal(t, []byte("payload"), p.Payload())
}

func TestTransactionEnvelope(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		tx, _ := envelopeReadyTx(t)
		env, err := tx.Envelope()
		require.NoError(t, err)
		require.Equal(t, "tx1", env.TxID())
		require.Equal(t, []byte("nonce"), env.Nonce())
		require.Equal(t, tx.TCreator.Bytes(), env.Creator())
		require.Nil(t, env.Results())

		raw, err := env.Bytes()
		require.NoError(t, err)
		decoded := NewEmptyEnvelope()
		require.NoError(t, decoded.FromBytes(raw))
		require.True(t, proto.Equal(env.(*Envelope).Envelope(), decoded.Envelope()))
	})

	t.Run("wraps createSCEnvelope error", func(t *testing.T) {
		t.Parallel()
		_, err := (&Transaction{TTxID: "tx1"}).Envelope()
		require.ErrorContains(t, err, "could not assemble transaction")
	})
}
