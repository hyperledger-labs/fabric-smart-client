/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"errors"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/transaction/mocks"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric-x-common/api/applicationpb"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mocks/fabric_network_service.go --fake-name FakeFabricNetworkService github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.FabricNetworkService
//go:generate counterfeiter -o mocks/signer_service.go --fake-name FakeSignerService github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.SignerService
//go:generate counterfeiter -o mocks/signer.go --fake-name FakeSigner github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Signer

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

	fakeFNS := &mocks.FakeFabricNetworkService{}
	fakeSignerService := &mocks.FakeSignerService{}

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
// is created and signed successfully.
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

	fakeFNS := &mocks.FakeFabricNetworkService{}
	fakeSignerService := &mocks.FakeSignerService{}
	fakeSigner := &mocks.FakeSigner{}

	fakeFNS.SignerServiceReturns(fakeSignerService)
	fakeSignerService.GetSignerReturns(fakeSigner, nil)
	fakeSigner.SignReturns([]byte("envelope-signature"), nil)

	tx := &Transaction{
		TTxID:              "tx1",
		TNonce:             []byte("nonce"),
		TCreator:           view.Identity([]byte("creator")),
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
}

func mustRawTx(t *testing.T, tx *applicationpb.Tx) []byte {
	t.Helper()

	raw, err := proto.Marshal(tx)
	require.NoError(t, err)
	return raw
}
