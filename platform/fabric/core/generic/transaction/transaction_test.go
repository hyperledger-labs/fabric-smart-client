/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction_test

import (
	"encoding/json"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

//go:generate counterfeiter -o mock/chaincode.go -fake-name Chaincode github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Chaincode
//go:generate counterfeiter -o mock/chaincode_manager.go -fake-name ChaincodeManager github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.ChaincodeManager
//go:generate counterfeiter -o mock/channel.go -fake-name Channel github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Channel
//go:generate counterfeiter -o mock/channel_membership.go -fake-name ChannelMembership github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.ChannelMembership
//go:generate counterfeiter -o mock/channel_provider.go -fake-name ChannelProvider github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.ChannelProvider
//go:generate counterfeiter -o mock/endorse_tx_store.go -fake-name EndorseTxStore github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.EndorseTxStore
//go:generate counterfeiter -o mock/envelope_store.go -fake-name EnvelopeStore github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.EnvelopeStore
//go:generate counterfeiter -o mock/identity.go -fake-name Identity github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp.Identity
//go:generate counterfeiter -o mock/identity_deserializer.go -fake-name IdentityDeserializer github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp.IdentityDeserializer
//go:generate counterfeiter -o mock/metadata_service.go -fake-name MetadataService github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.MetadataService
//go:generate counterfeiter -o mock/metadata_store.go -fake-name MetadataStore github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.MetadataStore
//go:generate counterfeiter -o mock/rwset.go -fake-name RWSet github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.RWSet
//go:generate counterfeiter -o mock/signer.go -fake-name Signer github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Signer
//go:generate counterfeiter -o mock/signer_service.go -fake-name SignerService github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.SignerService
//go:generate counterfeiter -o mock/transaction.go -fake-name Transaction github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Transaction
//go:generate counterfeiter -o mock/transaction_factory.go -fake-name TransactionFactory github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.TransactionFactory
//go:generate counterfeiter -o mock/vault.go -fake-name Vault github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Vault
//go:generate counterfeiter -o mock/verifier.go -fake-name Verifier github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Verifier
//go:generate counterfeiter -o mock/verifier_provider.go -fake-name VerifierProvider github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.VerifierProvider

func TestTransaction_GettersAndSetters(t *testing.T) {
	t.Parallel()
	mockChannelProvider := &mock.ChannelProvider{}
	mockSigService := &mock.SignerService{}
	mockChannel := &mock.Channel{}
	mockChannelProvider.ChannelReturns(mockChannel, nil)

	factory := transaction.NewEndorserTransactionFactory("network", mockChannelProvider, mockSigService)
	tx, err := factory.NewTransaction(t.Context(), "channel", []byte("nonce"), []byte("creator"), "txid", nil)
	require.NoError(t, err)

	require.Equal(t, "txid", tx.ID())
	require.Equal(t, "network", tx.Network())
	require.Equal(t, "channel", tx.Channel())
	require.Equal(t, []byte("nonce"), tx.Nonce())
	require.Equal(t, view.Identity([]byte("creator")), tx.Creator())

	// SetProposal
	tx.SetProposal("chaincode", "1.0", "function", "arg1", "arg2")
	require.Equal(t, "chaincode", tx.Chaincode())
	require.Equal(t, "1.0", tx.ChaincodeVersion())
	require.Equal(t, "function", tx.Function())

	params := tx.Parameters()
	require.Len(t, params, 2)
	require.Equal(t, []byte("arg1"), params[0])

	f, fParams := tx.FunctionAndParameters()
	require.Equal(t, "function", f)
	require.Equal(t, []string{"arg1", "arg2"}, fParams)

	// AppendParameter
	tx.AppendParameter([]byte("arg3"))
	require.Len(t, tx.Parameters(), 3)

	// SetParameterAt
	err = tx.SetParameterAt(1, []byte("arg2_new"))
	require.NoError(t, err)
	require.Equal(t, []byte("arg2_new"), tx.Parameters()[1])

	err = tx.SetParameterAt(10, []byte("arg10"))
	require.Error(t, err)

	// Transient
	require.NotNil(t, tx.Transient())
	tx.ResetTransient()
	require.Empty(t, tx.Transient())
}

func TestTransaction_Bytes(t *testing.T) {
	t.Parallel()
	mockChannelProvider := &mock.ChannelProvider{}
	mockSigService := &mock.SignerService{}
	mockChannel := &mock.Channel{}
	mockChannelProvider.ChannelReturns(mockChannel, nil)

	factory := transaction.NewEndorserTransactionFactory("network", mockChannelProvider, mockSigService)
	tx, err := factory.NewTransaction(t.Context(), "channel", []byte("nonce"), []byte("creator"), "txid", nil)
	require.NoError(t, err)

	b, err := tx.Bytes()
	require.NoError(t, err)
	require.NotEmpty(t, b)

	bnt, err := tx.BytesNoTransient()
	require.NoError(t, err)
	require.NotEmpty(t, bnt)

	raw, err := tx.Raw()
	require.NoError(t, err)
	require.NotEmpty(t, raw)
}

func TestTransaction_RWSet(t *testing.T) {
	t.Parallel()
	mockChannelProvider := &mock.ChannelProvider{}
	mockSigService := &mock.SignerService{}
	mockChannel := &mock.Channel{}
	mockVault := &mock.Vault{}
	mockRWSet := &mock.RWSet{}

	mockChannelProvider.ChannelReturns(mockChannel, nil)
	mockChannel.VaultReturns(mockVault)
	mockVault.NewRWSetReturns(mockRWSet, nil)
	mockRWSet.BytesReturns([]byte("rwsetbytes"), nil)

	factory := transaction.NewEndorserTransactionFactory("network", mockChannelProvider, mockSigService)
	tx, err := factory.NewTransaction(t.Context(), "channel", []byte("nonce"), []byte("creator"), "txid", nil)
	require.NoError(t, err)

	// GetRWSet -> populates from scratch
	rwset, err := tx.GetRWSet()
	require.NoError(t, err)
	require.NotNil(t, rwset)
	require.Equal(t, rwset, tx.RWS())

	// Done
	err = tx.Done()
	require.NoError(t, err)

	// Close
	tx.Close()
	require.Nil(t, tx.RWS())
}

// TestTransaction_ResultsReturnsErrorOnEmptyProposalResponses demonstrates that
// Transaction.Results() now rejects an empty TProposalResponses slice with an error
// instead of indexing t.TProposalResponses[0] and panicking with an
// index-out-of-range runtime error.
//
// TProposalResponses is an exported field and is populated directly from
// attacker-controlled envelope bytes by SetFromEnvelopeBytes (which sets
// t.TProposalResponses = upe.ProposalResponses with no length check), so a
// transaction reconstructed from a crafted envelope with zero proposal responses
// carries an empty TProposalResponses.
func TestTransaction_ResultsReturnsErrorOnEmptyProposalResponses(t *testing.T) {
	t.Parallel()

	tx := &transaction.Transaction{}
	require.Empty(t, tx.TProposalResponses)

	_, err := tx.Results()
	require.Error(t, err, "Results() must reject an empty TProposalResponses slice")
}

func TestProcessedTransaction(t *testing.T) {
	t.Parallel()
	env := createValidEnvelope(t)
	envBytes, err := proto.Marshal(env)
	require.NoError(t, err)

	pt, ht, err := transaction.NewProcessedTransactionFromEnvelopePayload(env.Payload)
	require.NoError(t, err)
	require.NotNil(t, pt)
	require.Equal(t, int32(common.HeaderType_ENDORSER_TRANSACTION), ht) // ENDORSER_TRANSACTION = 3

	pt2, err := transaction.NewProcessedTransactionFromEnvelopeRaw(envBytes)
	require.NoError(t, err)
	require.NotNil(t, pt2)

	require.Equal(t, "txid", pt2.TxID())
	require.Equal(t, []byte("results"), pt2.Results())
	require.Equal(t, envBytes, pt2.Envelope())

	// NewProcessedTransaction
	rawPt := &pb.ProcessedTransaction{
		ValidationCode:      int32(pb.TxValidationCode_VALID),
		TransactionEnvelope: env,
	}
	rawPtBytes, err := proto.Marshal(rawPt)
	require.NoError(t, err)

	pt3, err := transaction.NewProcessedTransaction(rawPtBytes)
	require.NoError(t, err)
	require.NotNil(t, pt3)

	require.True(t, pt3.IsValid())
	require.Equal(t, int32(pb.TxValidationCode_VALID), pt3.ValidationCode()) // VALID = 0
}

func TestTransaction_Endorse(t *testing.T) {
	t.Parallel()
	mockChannelProvider := &mock.ChannelProvider{}
	mockSigService := &mock.SignerService{}
	mockChannel := &mock.Channel{}
	mockSigner := &mock.Signer{}

	mockChannelProvider.ChannelReturns(mockChannel, nil)
	mockSigService.GetSignerReturns(mockSigner, nil)
	mockSigner.SignReturns([]byte("signature"), nil)

	factory := transaction.NewEndorserTransactionFactory("network", mockChannelProvider, mockSigService)
	tx, err := factory.NewTransaction(t.Context(), "channel", []byte("nonce"), []byte("creator"), "txid", nil)
	require.NoError(t, err)

	tx.SetProposal("chaincode", "1.0", "function")

	// StoreTransient mock
	mockMetadataService := &mock.MetadataService{}
	mockChannel.MetadataServiceReturns(mockMetadataService)
	mockMetadataService.StoreTransientReturns(nil)

	err = tx.Endorse()
	require.NoError(t, err)

	require.NotNil(t, tx.Proposal())
	require.NotNil(t, tx.SignedProposal())

	// EndorseProposal
	err = tx.EndorseProposal()
	require.NoError(t, err)
}

func TestTransaction_SetFromBytes(t *testing.T) {
	t.Parallel()
	mockChannelProvider := &mock.ChannelProvider{}
	mockSigService := &mock.SignerService{}
	mockChannel := &mock.Channel{}
	mockChannelProvider.ChannelReturns(mockChannel, nil)

	factory := transaction.NewEndorserTransactionFactory("network", mockChannelProvider, mockSigService)
	tx, err := factory.NewTransaction(t.Context(), "channel", []byte("nonce"), []byte("creator"), "txid", nil)
	require.NoError(t, err)

	b, err := tx.Bytes()
	require.NoError(t, err)

	// Unmarshal
	tx2, err := factory.NewTransaction(t.Context(), "channel", nil, nil, "", nil)
	require.NoError(t, err)

	err = tx2.SetFromBytes(b)
	require.NoError(t, err)
	require.Equal(t, "txid", tx2.ID())

	// Test TSignedProposal unmarshal failure
	txWithSigProp := &transaction.Transaction{}
	txWithSigProp.TSignedProposal = &pb.SignedProposal{
		ProposalBytes: []byte("invalid proposal bytes"),
	}
	bWithSigProp, _ := json.Marshal(txWithSigProp)
	err = tx2.SetFromBytes(bWithSigProp)
	require.ErrorContains(t, err, "failed unpacking proposal")

	// mock channel error
	mockChannelProvider.ChannelReturns(nil, contextError("channel fail"))
	err = tx2.SetFromBytes(b)
	require.ErrorContains(t, err, "channel fail")
}

// TestTransaction_SetFromBytesReturnsErrorOnEmptyChaincodeArgs demonstrates that
// Transaction.SetFromBytes now rejects a TSignedProposal whose ChaincodeInput.Args is
// empty with an error, instead of indexing Args[0] and panicking with an
// index-out-of-range runtime error.
//
// This is directly attacker-reachable: platform/fabric/services/endorser/flow.go's
// receiveTransactionView.Call and platform/fabric/services/state/transaction.go's
// receiveTransactionView.Call both read a raw []byte payload off an inbound P2P
// session and pass it straight to Builder.NewTransactionFromBytes ->
// Manager.NewTransactionFromBytes -> Transaction.SetFromBytes. A remote peer sending
// a transaction payload whose embedded proposal has zero chaincode arguments must get
// a rejected transaction, not a crashed responder goroutine.
func TestTransaction_SetFromBytesReturnsErrorOnEmptyChaincodeArgs(t *testing.T) {
	t.Parallel()
	mockChannelProvider := &mock.ChannelProvider{}
	mockSigService := &mock.SignerService{}
	mockChannel := &mock.Channel{}
	mockChannelProvider.ChannelReturns(mockChannel, nil)

	factory := transaction.NewEndorserTransactionFactory("network", mockChannelProvider, mockSigService)
	tx2, err := factory.NewTransaction(t.Context(), "channel", nil, nil, "", nil)
	require.NoError(t, err)

	maliciousSignedProposal := createSignedProposalWithArgs(t, [][]byte{})

	txWithEmptyArgs := &transaction.Transaction{}
	txWithEmptyArgs.TSignedProposal = &pb.SignedProposal{
		ProposalBytes: maliciousSignedProposal.ProposalBytes,
		Signature:     maliciousSignedProposal.Signature,
	}
	raw, err := json.Marshal(txWithEmptyArgs)
	require.NoError(t, err)

	err = tx2.SetFromBytes(raw)
	require.Error(t, err, "SetFromBytes must reject a zero-argument chaincode input")
}

func TestTransaction_SetFromEnvelopeBytes(t *testing.T) {
	t.Parallel()
	mockChannelProvider := &mock.ChannelProvider{}
	mockSigService := &mock.SignerService{}
	mockChannel := &mock.Channel{}
	mockChannelProvider.ChannelReturns(mockChannel, nil)

	env := createValidEnvelope(t)
	envBytes, err := proto.Marshal(env)
	require.NoError(t, err)

	factory := transaction.NewEndorserTransactionFactory("network", mockChannelProvider, mockSigService)
	tx, err := factory.NewTransaction(t.Context(), "channel", nil, nil, "", nil)
	require.NoError(t, err)

	err = tx.SetFromEnvelopeBytes(envBytes)
	require.NoError(t, err)
	require.Equal(t, "txid", tx.ID())
}

func TestTransaction_EndorsementAndProposal(t *testing.T) {
	t.Parallel()
	mockChannelProvider := &mock.ChannelProvider{}
	mockSigService := &mock.SignerService{}
	mockChannel := &mock.Channel{}
	mockSigner := &mock.Signer{}
	mockVault := &mock.Vault{}
	mockRWSet := &mock.RWSet{}
	mockMetadata := &mock.MetadataService{}

	mockChannelProvider.ChannelReturns(mockChannel, nil)
	mockChannel.VaultReturns(mockVault)
	mockChannel.MetadataServiceReturns(mockMetadata)
	mockVault.NewRWSetFromBytesReturns(mockRWSet, nil)
	mockRWSet.BytesReturns([]byte("rwset"), nil)
	mockSigService.GetSignerReturns(mockSigner, nil)
	mockSigner.SignReturns([]byte("signature"), nil)

	factory := transaction.NewEndorserTransactionFactory("network", mockChannelProvider, mockSigService)
	tx, err := factory.NewTransaction(t.Context(), "channel", []byte("nonce"), []byte("creator"), "txid", nil)
	require.NoError(t, err)

	tx.SetProposal("chaincode", "1.0", "function")

	// AppendProposalResponse
	pr := createValidProposalResponse(t)
	dpr, err := transaction.NewProposalResponseFromResponse(pr)
	require.NoError(t, err)
	err = tx.AppendProposalResponse(dpr)
	require.NoError(t, err)

	mockMembership := &mock.ChannelMembership{}
	mockVerifier := &mock.Verifier{}
	mockChannel.ChannelMembershipReturns(mockMembership)
	mockMembership.GetVerifierReturns(mockVerifier, nil)
	mockVerifier.VerifyReturnsOnCall(0, nil)
	mockVerifier.VerifyReturnsOnCall(1, errors.New("error"))

	// EndorseWithSigner
	err = tx.EndorseWithSigner(view.Identity([]byte("id")), mockSigner)
	require.NoError(t, err)

	err = tx.ProposalHasBeenEndorsedBy([]byte("endorser"))
	require.NoError(t, err)

	err = tx.ProposalHasBeenEndorsedBy([]byte("other"))
	require.Error(t, err)

	prs, err := tx.ProposalResponses()
	require.NoError(t, err)
	require.Len(t, prs, 2)

	// SetFromBytes empty
	err = tx.SetFromBytes(nil)
	require.Error(t, err)

	// EndorseProposalResponse
	err = tx.EndorseProposalResponse()
	require.NoError(t, err)

	// EndorseProposalResponseWithIdentity
	err = tx.EndorseProposalResponseWithIdentity(view.Identity([]byte("id")))
	require.NoError(t, err)
}

func TestTransaction_LifecycleAndFormatting(t *testing.T) {
	t.Parallel()
	mockChannelProvider := &mock.ChannelProvider{}
	mockSigService := &mock.SignerService{}
	mockChannel := &mock.Channel{}
	mockSigner := &mock.Signer{}
	mockVault := &mock.Vault{}
	mockRWSet := &mock.RWSet{}
	mockMetadata := &mock.MetadataService{}
	mockChaincodeManager := &mock.ChaincodeManager{}
	mockChaincode := &mock.Chaincode{}

	mockChannelProvider.ChannelReturns(mockChannel, nil)
	mockChannel.VaultReturns(mockVault)
	mockChannel.MetadataServiceReturns(mockMetadata)
	mockChannel.ChaincodeManagerReturns(mockChaincodeManager)
	mockChaincodeManager.ChaincodeReturns(mockChaincode)
	mockChaincode.VersionReturns("1.0", nil)

	mockVault.NewRWSetFromBytesReturns(mockRWSet, nil)
	mockRWSet.BytesReturns([]byte("rwset"), nil)

	factory := transaction.NewEndorserTransactionFactory("network", mockChannelProvider, mockSigService)
	tx, err := factory.NewTransaction(t.Context(), "channel", []byte("nonce"), []byte("creator"), "txid", nil)
	require.NoError(t, err)

	tx.SetProposal("chaincode", "", "function")

	// Raw
	b, err := tx.Raw()
	require.NoError(t, err)
	require.NotEmpty(t, b)

	// BytesNoTransient
	bnt, err := tx.BytesNoTransient()
	require.NoError(t, err)
	require.NotEmpty(t, bnt)

	// EndorseWithIdentity
	// EndorseWithIdentity takes view.Identity
	// But it actually uses the SignerService to get the signer for that identity.
	mockSigService.GetSignerReturns(mockSigner, nil)
	mockSigner.SignReturns([]byte("sig"), nil)
	err = tx.EndorseWithIdentity(view.Identity([]byte("id")))
	require.NoError(t, err)

	// EndorseProposalWithIdentity
	err = tx.EndorseProposalWithIdentity(view.Identity([]byte("id")))
	require.NoError(t, err)

	// Proposal, SignedProposal, GetRWSet, Bytes, Done, Close, AppendParameter, SetParameterAt, ResetTransient
	require.NotNil(t, tx.Proposal())
	require.NotNil(t, tx.SignedProposal())

	tx.AppendParameter([]byte("param2"))
	err = tx.SetParameterAt(0, []byte("param0"))
	require.NoError(t, err)

	err = tx.SetParameterAt(100, []byte("error"))
	require.Error(t, err)

	_, err = tx.GetRWSet()
	require.NoError(t, err)

	_, err = tx.Bytes()
	require.NoError(t, err)

	tx.ResetTransient()
	err = tx.Done()
	require.NoError(t, err)

	tx.Close()

	// EndorseProposal
	err = tx.EndorseProposal()
	require.NoError(t, err)
}

func TestTransaction_ProcessedTransactionValidation(t *testing.T) {
	t.Parallel()
	e := createValidEnvelope(t)
	raw, err := proto.Marshal(e)
	require.NoError(t, err)

	// NewProcessedTransactionFromEnvelopePayload
	pt1, ht, err := transaction.NewProcessedTransactionFromEnvelopePayload(e.Payload)
	require.NoError(t, err)
	require.NotNil(t, pt1)
	require.Equal(t, int32(common.HeaderType_ENDORSER_TRANSACTION), ht)

	// NewProcessedTransactionFromEnvelopeRaw
	pt2, err := transaction.NewProcessedTransactionFromEnvelopeRaw(raw)
	require.NoError(t, err)
	require.NotNil(t, pt2)
	require.Equal(t, raw, pt2.Envelope())

	// NewProcessedTransaction
	ptRaw, err := proto.Marshal(&pb.ProcessedTransaction{
		TransactionEnvelope: e,
		ValidationCode:      0,
	})
	require.NoError(t, err)

	pt3, err := transaction.NewProcessedTransaction(ptRaw)
	require.NoError(t, err)
	require.NotNil(t, pt3)
	require.Equal(t, int32(pb.TxValidationCode_VALID), pt3.ValidationCode())
	require.True(t, pt3.IsValid())
	require.NotEmpty(t, pt3.Results())
	require.Equal(t, "txid", pt3.TxID())
}

func TestTransaction_ErrorHandling(t *testing.T) {
	t.Parallel()
	mockChannelProvider := &mock.ChannelProvider{}
	mockSigService := &mock.SignerService{}
	mockChannel := &mock.Channel{}
	mockSigner := &mock.Signer{}

	mockChannelProvider.ChannelReturns(mockChannel, nil)
	mockSigService.GetSignerReturns(mockSigner, nil)
	mockSigner.SignReturns([]byte("signature"), nil)

	factory := transaction.NewEndorserTransactionFactory("network", mockChannelProvider, mockSigService)
	tx, err := factory.NewTransaction(t.Context(), "channel", []byte("nonce"), []byte("creator"), "txid", nil)
	require.NoError(t, err)

	// Envelope empty
	env, err := tx.Envelope()
	require.Error(t, err)
	require.Nil(t, env)

	// ProposalResponse empty
	pr, err := tx.ProposalResponse()
	require.NoError(t, err)
	require.Nil(t, pr)

	// EndorseWithIdentity (error path)
	// Without setting up a proper vault/metadata service, it will error in getting RWSet
	err = tx.EndorseWithIdentity(view.Identity([]byte("id")))
	require.Error(t, err)

	tx2, err := factory.NewTransaction(t.Context(), "channel", []byte("nonce"), []byte("creator"), "txid", nil)
	require.NoError(t, err)
	tx2.SetProposal("chaincode", "1.0", "function")

	mockMetadataService := &mock.MetadataService{}
	mockChannel.MetadataServiceReturns(mockMetadataService)
	mockMetadataService.StoreTransientReturns(nil)

	// Proposal methods
	err = tx2.Endorse()
	require.NoError(t, err)

	p := tx2.Proposal()
	require.NotNil(t, p)

	sp := tx2.SignedProposal()
	require.NotNil(t, sp)

	// If it's a known implementation type, we can cast it
	// driver interfaces might have these
	type proser interface {
		Header() []byte
		Payload() []byte
	}
	if prop, ok := p.(proser); ok {
		prop.Header()
		prop.Payload()
	}

	type signedProser interface {
		Internal() any
	}
	if sprop, ok := sp.(signedProser); ok {
		sprop.Internal()
	}

	// SetFromBytes error path
	err = tx2.SetFromBytes([]byte("invalid"))
	require.Error(t, err)
}

func TestTransaction_ProcessedTransactionErrors(t *testing.T) {
	t.Parallel()
	// Test error paths in NewProcessedTransaction...
	_, _, err := transaction.NewProcessedTransactionFromEnvelopePayload([]byte("invalid"))
	require.Error(t, err)

	_, err = transaction.NewProcessedTransactionFromEnvelopeRaw([]byte("invalid"))
	require.Error(t, err)

	_, err = transaction.NewProcessedTransaction([]byte("invalid"))
	require.Error(t, err)

	// Transaction empty edge cases
	factory := transaction.NewEndorserTransactionFactory("network", &mock.ChannelProvider{}, &mock.SignerService{})
	tx, err := factory.NewTransaction(t.Context(), "channel", nil, nil, "txid", nil)
	require.NoError(t, err)

	_, err = tx.Raw()
	require.NoError(t, err)

	_, err = tx.BytesNoTransient()
	require.NoError(t, err)

	// Test SetFromBytes
	txBytes, err := tx.Bytes()
	require.NoError(t, err)

	tx2, err := factory.NewTransaction(t.Context(), "channel", []byte("nonce"), []byte("creator"), "txid", nil)
	require.NoError(t, err)
	err = tx2.SetFromBytes(txBytes)
	require.NoError(t, err)

	err = tx2.SetFromBytes([]byte("invalid"))
	require.Error(t, err)
}

func TestTransaction_EnvelopeErrors(t *testing.T) {
	t.Parallel()
	mockChannelProvider := &mock.ChannelProvider{}
	mockSigService := &mock.SignerService{}
	mockChannel := &mock.Channel{}
	mockChannelProvider.ChannelReturns(mockChannel, nil)
	factory := transaction.NewEndorserTransactionFactory("network", mockChannelProvider, mockSigService)
	tx, err := factory.NewTransaction(t.Context(), "channel", []byte("nonce"), []byte("creator"), "txid", nil)
	require.NoError(t, err)

	// Error getting signer
	mockSigService.GetSignerReturns(nil, contextError("signer err"))
	_, err = tx.Envelope()
	require.ErrorContains(t, err, "signer not found")

	// Error Getting ProposalResponses
	mockSigService.GetSignerReturns(&mock.Signer{}, nil)
	invalidPR := &pb.ProposalResponse{
		Payload: []byte("invalid"),
	}
	tx.(*transaction.Transaction).TProposalResponses = append(tx.(*transaction.Transaction).TProposalResponses, invalidPR)
	_, err = tx.Envelope()
	require.ErrorContains(t, err, "failed getting proposalResponses")
}

func TestTransaction_EndorseWithIdentityErrors(t *testing.T) {
	t.Parallel()
	mockChannelProvider := &mock.ChannelProvider{}
	mockSigService := &mock.SignerService{}
	mockChannel := &mock.Channel{}
	mockMetadata := &mock.MetadataService{}
	mockChannel.MetadataServiceReturns(mockMetadata)
	mockChannelProvider.ChannelReturns(mockChannel, nil)
	factory := transaction.NewEndorserTransactionFactory("network", mockChannelProvider, mockSigService)
	tx, err := factory.NewTransaction(t.Context(), "channel", []byte("nonce"), []byte("creator"), "txid", nil)
	require.NoError(t, err)

	mockSigService.GetSignerReturns(nil, contextError("signer err"))
	err = tx.EndorseWithIdentity([]byte("id"))
	require.Error(t, err)

	tx.SetProposal("chaincode", "1.0", "function")

	// EndorseProposalResponseWithIdentity error (GetRWSet fails)
	mockSigService.GetSignerReturns(&mock.Signer{}, nil)
	mockVault := &mock.Vault{}
	mockChannel.VaultReturns(mockVault)
	mockVault.NewRWSetReturns(nil, contextError("rwset err"))
	err = tx.EndorseProposalResponseWithIdentity([]byte("id"))
	require.ErrorContains(t, err, "rwset err")

	// Test rwset.Bytes() failure handling
	mockRWSet := &mock.RWSet{}
	mockVault.NewRWSetReturns(mockRWSet, nil)
	mockRWSet.BytesReturns(nil, contextError("bytes err"))
	err = tx.EndorseProposalResponseWithIdentity([]byte("id"))
	require.ErrorContains(t, err, "bytes err")
}
