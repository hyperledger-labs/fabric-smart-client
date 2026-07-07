/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"errors"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric-x-common/api/applicationpb"
	"github.com/hyperledger/fabric-x-common/api/msppb"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/transaction/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

//go:generate counterfeiter -o mock/fabric_network_service.go --fake-name FabricNetworkService github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.FabricNetworkService
//go:generate counterfeiter -o mock/signer_service.go --fake-name SignerService github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.SignerService
//go:generate counterfeiter -o mock/signer.go --fake-name Signer github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Signer
//go:generate counterfeiter -o mock/channel_membership.go --fake-name ChannelMembership github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.ChannelMembership

// TestCreateSCEnvelopeNoProposalResponses verifies that envelope creation fails
// when the transaction carries no proposal responses at all.
func TestCreateSCEnvelopeNoProposalResponses(t *testing.T) {
	t.Parallel()
	tx := &Transaction{
		TTxID: "tx1",
	}

	_, err := tx.createSCEnvelope()
	require.Error(t, err)
	require.Contains(t, err.Error(), "number of responses must be larger than 0")
}

// TestCreateSCEnvelopeInvalidBaseTransaction verifies that envelope creation fails
// when the first proposal response does not contain a valid serialized tx payload.
func TestCreateSCEnvelopeInvalidBaseTransaction(t *testing.T) {
	t.Parallel()
	tx := &Transaction{
		TTxID: "tx1",
		TProposalResponses: []*peer.ProposalResponse{
			{
				Payload: []byte("not-a-valid-protobuf-tx"),
				Endorsement: &peer.Endorsement{
					Signature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
						sampleNamespaceEndorsements("Org1MSP", "sig-org1"),
					}),
				},
			},
		},
	}

	_, err := tx.createSCEnvelope()
	require.Error(t, err)
	require.Contains(t, err.Error(), "merge proposal response endorsements")
	require.Contains(t, err.Error(), "failed unmarshalling base tx")
}

// TestCreateSCEnvelopeMergeProposalResponsesPayloadMismatch verifies that
// envelope creation fails when proposal responses refer to different tx payloads.
func TestCreateSCEnvelopeMergeProposalResponsesPayloadMismatch(t *testing.T) {
	t.Parallel()
	tx1 := sampleTx("ns1", "key1", "value1")
	tx2 := sampleTx("ns1", "key2", "value2")

	rawTx1, err := proto.Marshal(tx1)
	require.NoError(t, err)

	rawTx2, err := proto.Marshal(tx2)
	require.NoError(t, err)

	resp1 := &peer.ProposalResponse{
		Payload: rawTx1,
		Endorsement: &peer.Endorsement{
			Signature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
				sampleNamespaceEndorsements("Org1MSP", "sig-org1"),
			}),
		},
	}
	resp2 := &peer.ProposalResponse{
		Payload: rawTx2,
		Endorsement: &peer.Endorsement{
			Signature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
				sampleNamespaceEndorsements("Org2MSP", "sig-org2"),
			}),
		},
	}

	tx := &Transaction{
		TTxID:              "tx1",
		TProposalResponses: []*peer.ProposalResponse{resp1, resp2},
	}

	_, err = tx.createSCEnvelope()
	require.Error(t, err)
	require.Contains(t, err.Error(), "merge proposal response endorsements")
	require.Contains(t, err.Error(), "content mismatch")
}

// TestCreateSCEnvelopeSignerNotFound verifies that envelope creation fails
// after marshaling the merged tx if the signer service cannot provide a signer
// for the transaction creator.
func TestCreateSCEnvelopeSignerNotFound(t *testing.T) {
	t.Parallel()
	rawTx := mustRawTx(t, sampleTx("ns1", "key1", "value1"))

	resp := &peer.ProposalResponse{
		Payload: rawTx,
		Endorsement: &peer.Endorsement{
			Signature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
				sampleNamespaceEndorsements("Org1MSP", "sig-org1"),
			}),
		},
	}

	fakeFNS := &mock.FabricNetworkService{}
	fakeSignerService := &mock.SignerService{}

	fakeFNS.SignerServiceReturns(fakeSignerService)
	fakeSignerService.GetSignerReturns(nil, errors.New("boom"))

	tx := &Transaction{
		TTxID:              "tx1",
		TNonce:             []byte("nonce"),
		TCreator:           view.Identity([]byte("creator")),
		TChannel:           "testchannel",
		TProposalResponses: []*peer.ProposalResponse{resp},
		fns:                fakeFNS,
	}

	_, err := tx.createSCEnvelope()
	require.Error(t, err)
	require.Contains(t, err.Error(), "signer not found")
	require.Contains(t, err.Error(), "creating tx envelope for ordering")
}

// TestCreateSCEnvelopeSuccess verifies the happy path:
// proposal responses are merged, the merged tx is marshaled, and the envelope
// is created and signed successfully. The SignatureHeader.Creator in the resulting
// envelope must use the cert-ID (cached identity) format.
func TestCreateSCEnvelopeSuccess(t *testing.T) {
	t.Parallel()
	rawTx := mustRawTx(t, sampleTx("ns1", "key1", "value1"))

	resp1 := &peer.ProposalResponse{
		Payload: rawTx,
		Endorsement: &peer.Endorsement{
			Signature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
				sampleNamespaceEndorsements("Org2MSP", "sig-org2"),
			}),
		},
	}
	resp2 := &peer.ProposalResponse{
		Payload: rawTx,
		Endorsement: &peer.Endorsement{
			Signature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
				sampleNamespaceEndorsements("Org1MSP", "sig-org1"),
			}),
		},
	}

	fakeFNS := &mock.FabricNetworkService{}
	fakeSignerService := &mock.SignerService{}
	fakeSigner := &mock.Signer{}
	fakeChannel := &mock.Channel{}
	fakeMembership := &mock.ChannelMembership{}

	fakeFNS.SignerServiceReturns(fakeSignerService)
	fakeSignerService.GetSignerReturns(fakeSigner, nil)
	fakeSigner.SignReturns([]byte("envelope-signature"), nil)
	fakeChannel.ChannelMembershipReturns(fakeMembership)
	fakeFNS.ChannelReturns(fakeChannel, nil)

	_, creatorBytes := mustSerializedIdentityWithRealCert(t, "Org1MSP")

	tx := &Transaction{
		TTxID:              "tx1",
		TNonce:             []byte("nonce"),
		TCreator:           view.Identity(creatorBytes),
		TChannel:           "testchannel",
		TProposalResponses: []*peer.ProposalResponse{resp1, resp2},
		fns:                fakeFNS,
	}

	env, err := tx.createSCEnvelope()
	require.NoError(t, err)
	require.NotNil(t, env)

	require.Equal(t, 1, fakeFNS.SignerServiceCallCount())
	require.Equal(t, 1, fakeSignerService.GetSignerCallCount())
	require.Equal(t, 1, fakeSigner.SignCallCount())

	// verify the Creator in the envelope's SignatureHeader uses cert-ID format
	var payload cb.Payload
	require.NoError(t, proto.Unmarshal(env.Payload, &payload))
	var sigHeader cb.SignatureHeader
	require.NoError(t, proto.Unmarshal(payload.Header.SignatureHeader, &sigHeader))
	var creator msppb.Identity
	require.NoError(t, proto.Unmarshal(sigHeader.Creator, &creator))
	require.NotEmpty(t, creator.GetCertificateId())
	require.Empty(t, creator.GetCertificate())
}

func mustRawTx(t *testing.T, tx *applicationpb.Tx) []byte {
	t.Helper()

	raw, err := proto.Marshal(tx)
	require.NoError(t, err)
	return raw
}
