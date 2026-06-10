/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ledgerrwset "github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset/kvrwset"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
	gproto "google.golang.org/protobuf/proto"

	pkgproto "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	rwsetfake "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset/fake"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
)

//go:generate counterfeiter -o mock/transaction_manager.go --fake-name TransactionManager github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.TransactionManager
//go:generate counterfeiter -o mock/rwset_inspector.go --fake-name RWSetInspector github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.RWSetInspector
//go:generate counterfeiter -o mock/rwset_payload_handler.go --fake-name RWSetPayloadHandler github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.RWSetPayloadHandler
//go:generate counterfeiter -o mock/rwset_loader.go --fake-name RWSetLoader github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.RWSetLoader
//go:generate counterfeiter -o mock/processor.go --fake-name Processor github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Processor

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

	signer := &rwsetfake.SignerSerializer{Serialized: creator, Signature: []byte("signature")}

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
