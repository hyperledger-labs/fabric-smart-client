/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoutil_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	"github.com/stretchr/testify/require"
)

func TestComputeProposalTxID(t *testing.T) {
	txid := protoutil.ComputeTxID([]byte{1}, []byte{1})

	// Compute the function computed by ComputeTxID,
	// namely, base64(sha256(nonce||creator))
	hf := sha256.New()
	hf.Write([]byte{1})
	hf.Write([]byte{1})
	hashOut := hf.Sum(nil)
	txid2 := hex.EncodeToString(hashOut)

	t.Logf("%x\n", hashOut)
	t.Logf("%s\n", txid)
	t.Logf("%s\n", txid2)

	require.Equal(t, txid, txid2)
}
