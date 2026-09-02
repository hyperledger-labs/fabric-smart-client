/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

const trustedNS = "trustedns"

// TestTrustedReadCertifierCertifyInputIsNoOp pins the documented behaviour: every
// node self-reads the committed value through GetReadAt, so no per-input proof is
// produced and certifiedInputs stays empty.
func TestTrustedReadCertifierCertifyInputIsNoOp(t *testing.T) {
	t.Parallel()

	tx, _, _ := newTestStateTransaction(trustedNS)
	certifier := &TrustedReadCertifier{}

	require.NoError(t, certifier.CertifyInput(tx.Namespace, "id-1"))
	require.Empty(t, tx.certifiedInputs,
		"the trusted-read certifier produces no certification to store")
}

func TestTrustedReadCertifierVerifyRWSetError(t *testing.T) {
	t.Parallel()

	tx, _, driverTx := newTestStateTransaction(trustedNS)
	injected := errors.New("rwset failed")
	driverTx.getRWSetErr = injected

	err := (&TrustedReadCertifier{}).VerifyInputCertificationAt(tx.Namespace, 0, "id-1")
	require.ErrorIs(t, err, injected, "the underlying cause must be preserved")
	require.ErrorContains(t, err, "failed getting rw set")
}

func TestTrustedReadCertifierVerifyReadError(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(trustedNS)
	injected := errors.New("read failed")
	rwset.getReadAtErr = injected

	err := (&TrustedReadCertifier{}).VerifyInputCertificationAt(tx.Namespace, 0, "id-1")
	require.ErrorIs(t, err, injected, "the underlying cause must be preserved")
	require.ErrorContains(t, err, "failed reading committed state")
}

func TestTrustedReadCertifierVerifyFieldMappingError(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(trustedNS)
	require.NoError(t, rwset.SetState(trustedNS, "id-1", []byte("committed")))
	require.NoError(t, rwset.AddReadAt(trustedNS, "id-1", nil))
	injected := errors.New("metadata failed")
	rwset.getStateMetadataErr = injected

	err := (&TrustedReadCertifier{}).VerifyInputCertificationAt(tx.Namespace, 0, "id-1")
	require.ErrorIs(t, err, injected, "the underlying cause must be preserved")
	require.ErrorContains(t, err, "failed getting field mapping")
}
