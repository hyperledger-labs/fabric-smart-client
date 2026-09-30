/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
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

// TestTransaction_AppendProposalResponseNilEndorsement verifies that
// AppendProposalResponse compares endorsers without panicking when an
// existing or incoming proposal response omits Endorsement.
func TestTransaction_AppendProposalResponseNilEndorsement(t *testing.T) {
	t.Parallel()

	withEndorser := func(t *testing.T) *pb.ProposalResponse {
		t.Helper()
		pr := createValidProposalResponse(t)
		pr.Endorsement.Endorser = []byte("endorser-1")
		return pr
	}
	withoutEndorsement := func(t *testing.T) *pb.ProposalResponse {
		t.Helper()
		pr := createValidProposalResponse(t)
		pr.Endorsement = nil
		return pr
	}

	tests := []struct {
		name     string
		existing func(t *testing.T) *pb.ProposalResponse
		response func(t *testing.T) *pb.ProposalResponse
	}{
		{
			name:     "existing entry has no endorsement",
			existing: withoutEndorsement,
			response: withEndorser,
		},
		{
			name:     "incoming response has no endorsement",
			existing: withEndorser,
			response: withoutEndorsement,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mockChannelProvider := &mock.ChannelProvider{}
			mockSigService := &mock.SignerService{}
			mockChannel := &mock.Channel{}
			mockChannelProvider.ChannelReturns(mockChannel, nil)
			factory := transaction.NewEndorserTransactionFactory("network", mockChannelProvider, mockSigService)
			tx, err := factory.NewTransaction(t.Context(), "channel", []byte("nonce"), []byte("creator"), "txid", nil)
			require.NoError(t, err)

			tx.(*transaction.Transaction).TProposalResponses = []*pb.ProposalResponse{tc.existing(t)}
			dpr, err := transaction.NewProposalResponseFromResponse(tc.response(t))
			require.NoError(t, err)

			require.NotPanics(t, func() {
				err = tx.AppendProposalResponse(dpr)
			})
			require.NoError(t, err)
			require.Len(t, tx.(*transaction.Transaction).TProposalResponses, 2)
		})
	}
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

// TestTransaction_ProposalHasBeenEndorsedByWithoutSignedProposal pins that a transaction
// that was never endorsed reports the missing signed proposal instead of panicking.
func TestTransaction_ProposalHasBeenEndorsedByWithoutSignedProposal(t *testing.T) {
	t.Parallel()
	mockChannelProvider := &mock.ChannelProvider{}
	mockChannel := &mock.Channel{}
	mockMembership := &mock.ChannelMembership{}
	mockChannelProvider.ChannelReturns(mockChannel, nil)
	mockChannel.ChannelMembershipReturns(mockMembership)

	factory := transaction.NewEndorserTransactionFactory("network", mockChannelProvider, &mock.SignerService{})
	tx, err := factory.NewTransaction(t.Context(), "channel", []byte("nonce"), []byte("creator"), "txid", nil)
	require.NoError(t, err)

	require.NotPanics(t, func() { err = tx.ProposalHasBeenEndorsedBy([]byte("endorser")) })
	require.ErrorContains(t, err, "transaction [txID=txid] has no signed proposal")
	require.Equal(t, 0, mockMembership.GetVerifierCallCount())
}

// endorsableTx is a transaction wired to mocks that let it endorse itself.
type endorsableTx struct {
	tx         *transaction.Transaction
	provider   *mock.ChannelProvider
	channel    *mock.Channel
	vault      *mock.Vault
	rwset      *mock.RWSet
	metadata   *mock.MetadataService
	chaincode  *mock.Chaincode
	membership *mock.ChannelMembership
	sigService *mock.SignerService
	signer     *mock.Signer
}

func newEndorsableTx(t *testing.T) *endorsableTx {
	t.Helper()
	f := &endorsableTx{
		provider:   &mock.ChannelProvider{},
		channel:    &mock.Channel{},
		vault:      &mock.Vault{},
		rwset:      &mock.RWSet{},
		metadata:   &mock.MetadataService{},
		chaincode:  &mock.Chaincode{},
		membership: &mock.ChannelMembership{},
		sigService: &mock.SignerService{},
		signer:     &mock.Signer{},
	}
	f.rwset.BytesReturns([]byte("rwset-bytes"), nil)
	f.vault.NewRWSetReturns(f.rwset, nil)
	f.vault.NewRWSetFromBytesReturns(f.rwset, nil)
	chaincodeManager := &mock.ChaincodeManager{}
	chaincodeManager.ChaincodeReturns(f.chaincode)
	f.channel.VaultReturns(f.vault)
	f.channel.MetadataServiceReturns(f.metadata)
	f.channel.ChaincodeManagerReturns(chaincodeManager)
	f.channel.ChannelMembershipReturns(f.membership)
	f.provider.ChannelReturns(f.channel, nil)
	f.signer.SignReturns([]byte("sig"), nil)
	f.sigService.GetSignerReturns(f.signer, nil)

	factory := transaction.NewEndorserTransactionFactory("network", f.provider, f.sigService)
	tx, err := factory.NewTransaction(t.Context(), "channel", []byte("nonce"), []byte("creator"), "txid", nil)
	require.NoError(t, err)
	f.tx = tx.(*transaction.Transaction)
	f.tx.SetProposal("mycc", "1.0", "invoke", "a1")
	return f
}

func TestTransaction_EndorseWithIdentitySelfEndorses(t *testing.T) {
	t.Parallel()
	f := newEndorsableTx(t)
	id := view.Identity("endorser")
	require.NoError(t, f.tx.SetRWSet())

	require.NoError(t, f.tx.EndorseWithIdentity(id))

	require.Len(t, f.tx.TProposalResponses, 1)
	pr := f.tx.TProposalResponses[0]
	require.Equal(t, []byte(id), pr.Endorsement.Endorser)
	require.Equal(t, []byte("sig"), pr.Endorsement.Signature)
	require.Equal(t, 2, f.signer.SignCallCount())
	require.Equal(t, slices.Concat(pr.Payload, []byte(id)), f.signer.SignArgsForCall(1))
	results, err := f.tx.Results()
	require.NoError(t, err)
	require.Equal(t, []byte("rwset-bytes"), results)
	require.Equal(t, 1, f.rwset.DoneCallCount())
	require.Nil(t, f.tx.RWS())
	require.Equal(t, 1, f.metadata.StoreTransientCallCount())

	// A second endorsement by the same identity is not recorded twice.
	require.NoError(t, f.tx.EndorseWithIdentity(id))
	require.Len(t, f.tx.TProposalResponses, 1)
}

func TestTransaction_EndorseWithIdentityReusesSignedProposal(t *testing.T) {
	t.Parallel()
	f := newEndorsableTx(t)
	id := view.Identity("endorser")
	require.NoError(t, f.tx.EndorseProposalWithIdentity(id))
	sp := f.tx.TSignedProposal
	require.Equal(t, 1, f.signer.SignCallCount())

	require.NoError(t, f.tx.SetRWSet())
	require.NoError(t, f.tx.EndorseWithIdentity(id))

	require.Same(t, sp, f.tx.TSignedProposal)
	require.Equal(t, 2, f.signer.SignCallCount())
	require.Len(t, f.tx.TProposalResponses, 1)
}

func TestTransaction_EndorseErrors(t *testing.T) {
	t.Parallel()
	endorsers := map[string]func(f *endorsableTx) error{
		"EndorseWithIdentity": func(f *endorsableTx) error { return f.tx.EndorseWithIdentity([]byte("endorser")) },
		"EndorseWithSigner":   func(f *endorsableTx) error { return f.tx.EndorseWithSigner([]byte("endorser"), f.signer) },
	}
	tests := []struct {
		name    string
		mutate  func(f *endorsableTx)
		wantErr string
	}{
		{
			name: "rws cannot be loaded",
			mutate: func(f *endorsableTx) {
				f.tx.RWSet = []byte("rws")
				f.vault.NewRWSetFromBytesReturns(nil, errors.New("vault down"))
			},
			wantErr: "failed getting proposal response",
		},
		{
			name:    "proposal cannot be signed",
			mutate:  func(f *endorsableTx) { f.signer.SignReturns(nil, errors.New("hsm down")) },
			wantErr: "failed setting proposal",
		},
		{
			name: "proposal response cannot be signed",
			mutate: func(f *endorsableTx) {
				f.tx.RWSet = []byte("rws")
				f.signer.SignReturnsOnCall(1, nil, errors.New("hsm down"))
			},
			wantErr: "could not sign the proposal response payload",
		},
		{
			name:    "transient cannot be stored",
			mutate:  func(f *endorsableTx) { f.metadata.StoreTransientReturns(errors.New("db down")) },
			wantErr: "failed storing transient",
		},
	}
	for name, endorse := range endorsers {
		for _, tc := range tests {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				f := newEndorsableTx(t)
				tc.mutate(f)
				require.ErrorContains(t, endorse(f), tc.wantErr)
			})
		}
	}
}

func TestTransaction_EndorseChaincodeVersion(t *testing.T) {
	t.Parallel()
	endorse := func(t *testing.T, f *endorsableTx) error {
		t.Helper()
		require.NoError(t, f.tx.SetRWSet())
		return f.tx.EndorseWithIdentity([]byte("endorser"))
	}
	version := func(t *testing.T, f *endorsableTx) string {
		t.Helper()
		upr, err := transaction.UnpackProposalResponse(f.tx.TProposalResponses[0])
		require.NoError(t, err)
		require.Equal(t, "mycc", upr.ChaincodeAction.ChaincodeId.Name)
		return upr.ChaincodeAction.ChaincodeId.Version
	}

	t.Run("missing version is fetched from the chaincode", func(t *testing.T) {
		t.Parallel()
		f := newEndorsableTx(t)
		f.tx.SetProposal("mycc", "", "invoke")
		f.chaincode.VersionReturns("2.0", nil)

		require.NoError(t, endorse(t, f))
		require.Equal(t, "2.0", version(t, f))
		require.Equal(t, "mycc", f.channel.ChaincodeManager().(*mock.ChaincodeManager).ChaincodeArgsForCall(0))
	})

	t.Run("missing version cannot be fetched", func(t *testing.T) {
		t.Parallel()
		f := newEndorsableTx(t)
		f.tx.SetProposal("mycc", "", "invoke")
		f.chaincode.VersionReturns("", errors.New("not installed"))

		require.ErrorContains(t, endorse(t, f), "failed to get chaincode version, proposal didn't contain it")
	})

	t.Run("version set in the proposal", func(t *testing.T) {
		t.Parallel()
		f := newEndorsableTx(t)

		require.NoError(t, endorse(t, f))
		require.Equal(t, "1.0", version(t, f))
		require.Equal(t, 0, f.chaincode.VersionCallCount())
	})
}

func TestTransaction_EndorseProposalErrors(t *testing.T) {
	t.Parallel()

	t.Run("response without a signed proposal", func(t *testing.T) {
		t.Parallel()
		f := newEndorsableTx(t)
		require.EqualError(t, f.tx.EndorseProposalResponseWithIdentity([]byte("endorser")), "signed proposal is nil")
	})

	t.Run("signer not found", func(t *testing.T) {
		t.Parallel()
		f := newEndorsableTx(t)
		signerErr := errors.New("unknown identity")
		f.sigService.GetSignerReturns(nil, signerErr)
		require.ErrorIs(t, f.tx.EndorseProposalWithIdentity([]byte("endorser")), signerErr)
		require.ErrorIs(t, f.tx.EndorseProposalResponseWithIdentity([]byte("endorser")), signerErr)
	})

	t.Run("proposal cannot be signed", func(t *testing.T) {
		t.Parallel()
		f := newEndorsableTx(t)
		signErr := errors.New("hsm down")
		f.signer.SignReturns(nil, signErr)
		require.ErrorIs(t, f.tx.EndorseProposalWithIdentity([]byte("endorser")), signErr)
		require.Nil(t, f.tx.SignedProposal())
	})
}

func TestTransaction_EnvelopeRoundTrip(t *testing.T) {
	t.Parallel()
	f := newEndorsableTx(t)
	require.NoError(t, f.tx.SetRWSet())
	require.NoError(t, f.tx.EndorseWithIdentity([]byte("endorser")))

	env, err := f.tx.Envelope()
	require.NoError(t, err)
	raw, err := env.Bytes()
	require.NoError(t, err)

	upe, headerType, err := transaction.UnpackEnvelopeFromBytes(raw)
	require.NoError(t, err)
	require.Equal(t, int32(common.HeaderType_ENDORSER_TRANSACTION), headerType)
	require.Equal(t, "txid", upe.TxID)
	require.Equal(t, "channel", upe.Ch)
	require.Equal(t, "mycc", upe.ChaincodeName)
	require.Equal(t, "1.0", upe.ChaincodeVersion)
	require.Equal(t, "invoke", upe.Function)
	require.Equal(t, []string{"a1"}, upe.Args)
	require.Equal(t, []byte("rwset-bytes"), upe.Results)
	require.Len(t, upe.ProposalResponses, 1)
}

func TestTransaction_SetRWSetSources(t *testing.T) {
	t.Parallel()
	vaultErr := errors.New("vault down")
	tests := []struct {
		name      string
		mutate    func(t *testing.T, f *endorsableTx)
		wantBytes []byte
		wantErr   string
		wantCause error
	}{
		{
			name:      "from the stored rws",
			mutate:    func(_ *testing.T, f *endorsableTx) { f.tx.RWSet = []byte("rws") },
			wantBytes: []byte("rws"),
		},
		{
			name: "stored rws cannot be loaded",
			mutate: func(_ *testing.T, f *endorsableTx) {
				f.tx.RWSet = []byte("rws")
				f.vault.NewRWSetFromBytesReturns(nil, vaultErr)
			},
			wantErr:   "failed to populate rws from existing rws",
			wantCause: vaultErr,
		},
		{
			name: "from the proposal response",
			mutate: func(t *testing.T, f *endorsableTx) {
				t.Helper()
				f.tx.RWSet = []byte("rws")
				f.tx.TProposalResponses = []*pb.ProposalResponse{createValidProposalResponse(t)}
			},
			wantBytes: []byte("results"),
		},
		{
			name: "proposal response rws cannot be loaded",
			mutate: func(t *testing.T, f *endorsableTx) {
				t.Helper()
				f.tx.TProposalResponses = []*pb.ProposalResponse{createValidProposalResponse(t)}
				f.vault.NewRWSetFromBytesReturns(nil, vaultErr)
			},
			wantErr:   "failed to populate rws from proposal response",
			wantCause: vaultErr,
		},
		{
			name: "proposal response cannot be decoded",
			mutate: func(_ *testing.T, f *endorsableTx) {
				f.tx.TProposalResponses = []*pb.ProposalResponse{{Payload: []byte("invalid")}}
			},
			wantErr: "failed to get rws from proposal response",
		},
		{
			name:      "fresh rws cannot be created",
			mutate:    func(_ *testing.T, f *endorsableTx) { f.vault.NewRWSetReturns(nil, vaultErr) },
			wantErr:   "failed to create fresh rws",
			wantCause: vaultErr,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newEndorsableTx(t)
			tc.mutate(t, f)

			err := f.tx.SetRWSet()
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				if tc.wantCause != nil {
					require.ErrorIs(t, err, tc.wantCause)
				}
				require.Nil(t, f.tx.RWS())
				return
			}
			require.NoError(t, err)
			require.Same(t, f.rwset, f.tx.RWS())
			require.Equal(t, 1, f.vault.NewRWSetFromBytesCallCount())
			_, txID, raw := f.vault.NewRWSetFromBytesArgsForCall(0)
			require.Equal(t, "txid", txID)
			require.Equal(t, tc.wantBytes, raw)
		})
	}
}

func TestTransaction_SerializeActiveRWSet(t *testing.T) {
	t.Parallel()

	t.Run("raw keeps the simulation open", func(t *testing.T) {
		t.Parallel()
		f := newEndorsableTx(t)
		require.NoError(t, f.tx.SetRWSet())

		raw, err := f.tx.Raw()
		require.NoError(t, err)
		decoded := &transaction.Transaction{}
		require.NoError(t, json.Unmarshal(raw, decoded))
		require.Equal(t, []byte("rwset-bytes"), decoded.RWSet)
		require.Equal(t, 0, f.rwset.DoneCallCount())
	})

	serializers := map[string]func(tx *transaction.Transaction) error{
		"Raw":              func(tx *transaction.Transaction) error { _, err := tx.Raw(); return err },
		"Bytes":            func(tx *transaction.Transaction) error { _, err := tx.Bytes(); return err },
		"BytesNoTransient": func(tx *transaction.Transaction) error { _, err := tx.BytesNoTransient(); return err },
		"Done":             func(tx *transaction.Transaction) error { return tx.Done() },
	}
	for name, serialize := range serializers {
		t.Run(name+" surfaces rws marshalling errors", func(t *testing.T) {
			t.Parallel()
			f := newEndorsableTx(t)
			require.NoError(t, f.tx.SetRWSet())
			bytesErr := errors.New("marshal failed")
			f.rwset.BytesReturns(nil, bytesErr)

			require.ErrorIs(t, serialize(f.tx), bytesErr)
		})
	}
}

func TestTransaction_BytesNoTransient(t *testing.T) {
	t.Parallel()

	t.Run("drops the transient only", func(t *testing.T) {
		t.Parallel()
		f := newEndorsableTx(t)
		f.tx.TTransient = map[string][]byte{"k": []byte("v")}
		require.NoError(t, f.tx.EndorseProposalWithIdentity([]byte("creator")))

		raw, err := f.tx.BytesNoTransient()
		require.NoError(t, err)
		decoded := &transaction.Transaction{}
		require.NoError(t, json.Unmarshal(raw, decoded))
		require.Empty(t, decoded.TTransient)
		require.True(t, proto.Equal(f.tx.TSignedProposal, decoded.TSignedProposal))
		require.Equal(t, []byte("v"), f.tx.TTransient["k"])
	})

	t.Run("undecodable signed proposal", func(t *testing.T) {
		t.Parallel()
		f := newEndorsableTx(t)
		f.tx.TSignedProposal = &pb.SignedProposal{ProposalBytes: []byte("invalid")}

		_, err := f.tx.BytesNoTransient()
		require.ErrorContains(t, err, "error unmarshalling Proposal")
	})
}

type foreignProposalResponse struct {
	driver.ProposalResponse
}

func TestTransaction_RejectsForeignInput(t *testing.T) {
	t.Parallel()

	t.Run("From another transaction type", func(t *testing.T) {
		t.Parallel()
		f := newEndorsableTx(t)
		require.ErrorContains(t, f.tx.From(&mock.Transaction{}), "unexpected transaction type [*mock.Transaction]")
	})

	t.Run("From an undecodable signed proposal", func(t *testing.T) {
		t.Parallel()
		f := newEndorsableTx(t)
		src := &transaction.Transaction{TSignedProposal: &pb.SignedProposal{ProposalBytes: []byte("invalid")}}
		require.ErrorContains(t, f.tx.From(src), "error unmarshalling Proposal")
	})

	t.Run("AppendProposalResponse of another type", func(t *testing.T) {
		t.Parallel()
		f := newEndorsableTx(t)
		err := f.tx.AppendProposalResponse(foreignProposalResponse{})
		require.ErrorContains(t, err, "unexpected proposal response type [transaction_test.foreignProposalResponse]")
		require.Empty(t, f.tx.TProposalResponses)
	})

	t.Run("Results of an undecodable response", func(t *testing.T) {
		t.Parallel()
		f := newEndorsableTx(t)
		f.tx.TProposalResponses = []*pb.ProposalResponse{{Payload: []byte("invalid")}}
		_, err := f.tx.Results()
		require.ErrorContains(t, err, "error unmarshalling ProposalResponsePayload")
	})

	t.Run("NewProcessedTransaction with an undecodable envelope payload", func(t *testing.T) {
		t.Parallel()
		raw := mustMarshal(t, &pb.ProcessedTransaction{TransactionEnvelope: &common.Envelope{Payload: []byte("invalid")}})
		pt, err := transaction.NewProcessedTransaction(raw)
		require.Error(t, err)
		require.Nil(t, pt)
	})
}

func TestTransaction_ChannelErrors(t *testing.T) {
	t.Parallel()

	t.Run("ProposalHasBeenEndorsedBy without a verifier", func(t *testing.T) {
		t.Parallel()
		f := newEndorsableTx(t)
		require.NoError(t, f.tx.EndorseProposal())
		verifierErr := errors.New("unknown party")
		f.membership.GetVerifierReturns(nil, verifierErr)

		require.ErrorIs(t, f.tx.ProposalHasBeenEndorsedBy([]byte("endorser")), verifierErr)
	})

	t.Run("SetFromEnvelopeBytes without a channel", func(t *testing.T) {
		t.Parallel()
		f := newEndorsableTx(t)
		channelErr := errors.New("unknown channel")
		f.provider.ChannelReturns(nil, channelErr)

		require.ErrorIs(t, f.tx.SetFromEnvelopeBytes(mustMarshal(t, createValidEnvelope(t))), channelErr)
	})
}
