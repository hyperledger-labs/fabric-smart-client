/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
)

func TestFinality(t *testing.T) {
	t.Parallel()

	mf := &mock.Finality{}
	finality := &Finality{finality: mf}
	ctx := t.Context()

	mf.IsFinalReturns(nil)
	err := finality.IsFinal(ctx, "txid1")
	require.NoError(t, err)
	require.Equal(t, 1, mf.IsFinalCallCount())
	callCtx, txid := mf.IsFinalArgsForCall(0)
	require.Equal(t, ctx, callCtx)
	require.Equal(t, "txid1", txid)

	mf.IsFinalReturns(errors.New("not final"))
	err = finality.IsFinal(ctx, "txid2")
	require.ErrorContains(t, err, "not final")
	require.Equal(t, 2, mf.IsFinalCallCount())
}
