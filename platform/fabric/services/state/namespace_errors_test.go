/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

const errNS = "assetns"

// seedOutput writes a state through the namespace so the RWSet has a write entry
// to read back, and returns the id it was stored under.
func seedOutput(tb testing.TB, tx *Transaction, h *House) string {
	tb.Helper()
	require.NoError(tb, tx.AddOutput(h))
	return h.LinearID
}

// requirePanicsContaining asserts f panics and that the panic value mentions want.
//
// It matches a substring rather than the whole value on purpose: several of these
// messages carry a long-standing "filed"/"failed" typo, and an exact match would
// turn a spelling fix into a test failure. Prefer the injected cause as want, since
// that is the part a behaviour change would actually drop.
func requirePanicsContaining(tb testing.TB, want string, f func()) {
	tb.Helper()
	defer func() {
		r := recover()
		require.NotNil(tb, r, "expected a panic")
		require.Contains(tb, fmt.Sprint(r), want)
	}()
	f()
}

// seedRead registers a read entry for key with the given raw value.
func seedRead(tb testing.TB, rwset *testRWSet, key string, raw []byte) {
	tb.Helper()
	require.NoError(tb, rwset.SetState(errNS, cdriver.PKey(key), raw))
	rwset.writes[errNS] = nil // keep the write list clean; we only want a read
	require.NoError(tb, rwset.AddReadAt(errNS, key, nil))
}

// TestNamespaceIndexOutOfRange checks that an index the RWSet cannot serve is
// reported rather than panicking.
//
// The three indices below all take the same `i < 0 || i >= len(...)` branch in the
// RWSet, so this is one path, not three. They are kept as a table because the
// interesting property is that GetInputAt and GetOutputAt both surface the failure
// for a negative index, one past the end, and far past the end -- a regression that
// silently clamped instead of erroring would still be caught.
func TestNamespaceIndexOutOfRange(t *testing.T) {
	t.Parallel()

	for _, index := range []int{-1, 1, 99} {
		t.Run(fmt.Sprintf("get output at %d", index), func(t *testing.T) {
			t.Parallel()
			tx, _, _ := newTestStateTransaction(errNS)
			seedOutput(t, tx, &House{Address: "one", LinearID: "id-1"})

			err := tx.GetOutputAt(index, &House{})
			require.Error(t, err)
			require.ErrorContains(t, err, "failed getting state")
		})

		t.Run(fmt.Sprintf("get input at %d", index), func(t *testing.T) {
			t.Parallel()
			tx, rwset, _ := newTestStateTransaction(errNS)
			raw, err := json.Marshal(&House{Address: "one", LinearID: "id-1"})
			require.NoError(t, err)
			seedRead(t, rwset, "id-1", raw)

			err = tx.GetInputAt(index, &House{})
			require.Error(t, err)
			require.ErrorContains(t, err, "failed getting state")
		})
	}
}

// TestNamespaceGetOutputAtLastValidIndex is the boundary companion to the
// out-of-range cases: count-1 must succeed.
func TestNamespaceGetOutputAtLastValidIndex(t *testing.T) {
	t.Parallel()

	tx, _, _ := newTestStateTransaction(errNS)
	seedOutput(t, tx, &House{Address: "first", LinearID: "id-1"})
	seedOutput(t, tx, &House{Address: "second", LinearID: "id-2"})
	require.Equal(t, 2, tx.NumOutputs())

	var got House
	require.NoError(t, tx.GetOutputAt(1, &got))
	require.Equal(t, "second", got.Address)
}

func TestNamespaceGetOutputAtUnmarshalMismatch(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(errNS)
	require.NoError(t, rwset.SetState(errNS, "id-1", []byte("{not-json")))

	err := tx.GetOutputAt(0, &House{})
	require.Error(t, err)
	require.ErrorContains(t, err, "failed unmarshalling state")
}

func TestNamespaceGetOutputAtMappingError(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(errNS)
	seedOutput(t, tx, &House{Address: "one", LinearID: "id-1"})
	rwset.getStateMetadataErr = errors.New("metadata failed")

	err := tx.GetOutputAt(0, &House{})
	require.Error(t, err)
	require.ErrorContains(t, err, "failed getting mapping")
}

func TestNamespaceGetInputAtUnmarshalMismatch(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(errNS)
	seedRead(t, rwset, "id-1", []byte("{not-json"))

	err := tx.GetInputAt(0, &House{})
	require.Error(t, err)
	require.ErrorContains(t, err, "failed unmarshalling state")
}

// TestNamespaceGetInputAtFallsBackToCertifiedInput covers the recovery path: when
// GetReadAt fails but the key is present in certifiedInputs, the value is served
// from there instead of surfacing the error.
func TestNamespaceGetInputAtFallsBackToCertifiedInput(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(errNS)
	raw, err := json.Marshal(&House{Address: "certified", LinearID: "id-1"})
	require.NoError(t, err)

	// A read key exists, but reading its value fails.
	require.NoError(t, rwset.AddReadAt(errNS, "id-1", nil))
	rwset.getReadAtErr = errors.New("read value unavailable")
	tx.certifiedInputs["id-1"] = raw

	var got House
	require.NoError(t, tx.GetInputAt(0, &got))
	require.Equal(t, "certified", got.Address)
}

// TestNamespaceGetInputAtCertifiedInputMissing is the same path when the key is
// absent from certifiedInputs: the original read error is returned.
func TestNamespaceGetInputAtCertifiedInputMissing(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(errNS)
	require.NoError(t, rwset.AddReadAt(errNS, "id-1", nil))
	rwset.getReadAtErr = errors.New("read value unavailable")

	err := tx.GetInputAt(0, &House{})
	require.Error(t, err)
	require.ErrorContains(t, err, "failed getting state")
}

// TestNamespaceGetInputAtBothLookupsFail exercises the joined-error branch, where
// GetReadAt and GetReadKeyAt both fail and both causes are reported.
func TestNamespaceGetInputAtBothLookupsFail(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(errNS)
	rwset.getReadAtErr = errors.New("read value unavailable")
	rwset.getReadKeyAtErr = errors.New("read key unavailable")

	err := tx.GetInputAt(0, &House{})
	require.Error(t, err)
	require.ErrorContains(t, err, "read value unavailable")
	require.ErrorContains(t, err, "read key unavailable")
}

func TestNamespaceAddInputByLinearIDGetStateError(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(errNS)
	rwset.getStateErr = errors.New("get state failed")

	err := tx.AddInputByLinearID("id-1", &House{})
	require.Error(t, err)
	require.ErrorContains(t, err, "failed getting state")
}

func TestNamespaceAddInputByLinearIDMappingError(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(errNS)
	rwset.getStateMetadataErr = errors.New("metadata failed")

	err := tx.AddInputByLinearID("id-1", &House{})
	require.Error(t, err)
	require.ErrorContains(t, err, "failed getting mapping")
}

func TestNamespaceAddInputByLinearIDUnmarshalMismatch(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(errNS)
	require.NoError(t, rwset.SetState(errNS, "id-1", []byte("{not-json")))

	err := tx.AddInputByLinearID("id-1", &House{})
	require.Error(t, err)
	require.ErrorContains(t, err, "failed unmarshalling state")
}

// TestNamespaceAddInputByLinearIDOptionError checks an option that fails aborts
// before the input is registered.
func TestNamespaceAddInputByLinearIDOptionError(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(errNS)
	raw, err := json.Marshal(&House{Address: "one", LinearID: "id-1"})
	require.NoError(t, err)
	require.NoError(t, rwset.SetState(errNS, "id-1", raw))

	optErr := errors.New("bad option")
	err = tx.AddInputByLinearID("id-1", &House{}, func(*addInputOptions) error { return optErr })
	require.ErrorIs(t, err, optErr)
	require.ErrorContains(t, err, "failed parsing opts")
}

func TestNamespaceAddOutputSetStateError(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(errNS)
	rwset.setStateErr = errors.New("write rejected")

	err := tx.AddOutput(&House{Address: "one", LinearID: "id-1"})
	require.ErrorIs(t, err, rwset.setStateErr)
}

// TestNamespaceAddOutputHashHidingSetStateError covers the hash-hiding branch's
// own write, which is a separate call site from the default branch.
func TestNamespaceAddOutputHashHidingSetStateError(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(errNS)
	rwset.setStateErr = errors.New("write rejected")

	err := tx.AddOutput(&House{Address: "one", LinearID: "id-1"}, WithHashHiding())
	require.ErrorIs(t, err, rwset.setStateErr)
}

// TestNamespaceAddOutputHashHidingStoresHashAndPreimage asserts the visible state
// is the digest while the preimage is kept in the field mapping.
func TestNamespaceAddOutputHashHidingStoresHashAndPreimage(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(errNS)
	h := &House{Address: "secret", LinearID: "id-1"}
	require.NoError(t, tx.AddOutput(h, WithHashHiding()))

	stored, err := rwset.GetState(errNS, "id-1")
	require.NoError(t, err)

	// NotEmpty first: without it, a missing key would also satisfy NotContains and
	// the test would pass for the wrong reason.
	require.NotEmpty(t, stored, "the key must hold the digest")
	require.Len(t, stored, sha256.Size, "the stored value is a sha256 digest")
	require.NotContains(t, string(stored), "secret", "the raw state must not be written in the clear")

	var got House
	require.NoError(t, tx.GetOutputAt(0, &got))
	require.Equal(t, "secret", got.Address, "the preimage is recovered from the mapping")
}

func TestNamespaceAddOutputOptionError(t *testing.T) {
	t.Parallel()

	tx, _, _ := newTestStateTransaction(errNS)
	optErr := errors.New("bad option")

	err := tx.AddOutput(&House{LinearID: "id-1"}, func(*addOutputOptions) error { return optErr })
	require.ErrorIs(t, err, optErr)
	require.ErrorContains(t, err, "failed parsing opts")
}

// TestNamespaceAddOutputMetaHandlerError covers the setMeta step of AddOutput: a
// failing meta handler aborts the call after the state has been written.
func TestNamespaceAddOutputMetaHandlerError(t *testing.T) {
	t.Parallel()

	tx, _, _ := newTestStateTransaction(errNS)
	tx.metaHandlers = []MetaHandler{&testMetaHandler{err: errors.New("meta failed")}}

	err := tx.AddOutput(&House{Address: "one", LinearID: "id-1"})
	require.Error(t, err)
	require.ErrorContains(t, err, "failed setting metadata")
}

func TestNamespaceDeleteSetStateError(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(errNS)
	rwset.setStateErr = errors.New("delete rejected")

	err := tx.Delete(&House{LinearID: "id-1"})
	require.ErrorIs(t, err, rwset.setStateErr)
}

// TestNamespaceDeleteWritesNilValue records that a delete is modelled as a write
// of a nil value, which is what Outputs() reads back as a delete marker.
func TestNamespaceDeleteWritesNilValue(t *testing.T) {
	t.Parallel()

	tx, _, _ := newTestStateTransaction(errNS)
	require.NoError(t, tx.Delete(&House{LinearID: "id-1"}))
	require.Equal(t, 1, tx.NumOutputs())

	outputs := tx.Outputs()
	require.Equal(t, 1, outputs.Count())
	require.True(t, outputs.At(0).IsDelete(), "a deleted key is reported as a delete")
}

func TestNamespaceDeleteAbsentKeyIsAccepted(t *testing.T) {
	t.Parallel()

	tx, _, _ := newTestStateTransaction(errNS)
	require.NoError(t, tx.Delete(&House{LinearID: "never-written"}),
		"deleting a key that was never written is not an error")
}

// TestNamespaceInterleavedAddDeleteRead walks a mixed sequence and checks the
// input/output counts stay consistent throughout.
//
// Each key is written exactly once, deliberately. The real vault.WriteSet
// de-duplicates by key -- only a key's first write extends OrderedWrites (see
// WriteSet.Add in platform/common/core/generic/vault/rwset.go) -- whereas testRWSet
// appends every call. A sequence that re-wrote a key would assert counts that hold
// for the fake and not for the framework, so this walks distinct keys and the
// overwrite case is left unasserted rather than asserted wrongly.
func TestNamespaceInterleavedAddDeleteRead(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(errNS)
	require.Zero(t, tx.NumOutputs())
	require.Zero(t, tx.NumInputs())

	seedOutput(t, tx, &House{Address: "first", LinearID: "id-1"})
	require.Equal(t, 1, tx.NumOutputs())

	require.NoError(t, tx.Delete(&House{LinearID: "id-2"}))
	require.Equal(t, 2, tx.NumOutputs(), "deleting a fresh key is one more write")

	seedOutput(t, tx, &House{Address: "third", LinearID: "id-3"})
	require.Equal(t, 3, tx.NumOutputs())

	require.NoError(t, rwset.AddReadAt(errNS, "id-3", nil))
	require.Equal(t, 1, tx.NumInputs())

	var got House
	require.NoError(t, tx.GetOutputAt(2, &got))
	require.Equal(t, "third", got.Address)

	outputs := tx.Outputs()
	require.Equal(t, 3, outputs.Count())
	require.True(t, outputs.At(1).IsDelete(), "the deleted key is reported as a delete")
}

func TestNamespaceNumInputsAndOutputsPanicOnRWSetError(t *testing.T) {
	t.Parallel()

	t.Run("num inputs", func(t *testing.T) {
		t.Parallel()
		tx, _, driverTx := newTestStateTransaction(errNS)
		driverTx.getRWSetErr = errors.New("rwset failed")
		requirePanicsContaining(t, "rwset failed", func() { _ = tx.NumInputs() })
	})

	t.Run("num outputs", func(t *testing.T) {
		t.Parallel()
		tx, _, driverTx := newTestStateTransaction(errNS)
		driverTx.getRWSetErr = errors.New("rwset failed")
		requirePanicsContaining(t, "rwset failed", func() { _ = tx.NumOutputs() })
	})
}

func TestNamespaceStreamsPanicOnRWSetError(t *testing.T) {
	t.Parallel()

	t.Run("outputs", func(t *testing.T) {
		t.Parallel()
		tx, _, driverTx := newTestStateTransaction(errNS)
		driverTx.getRWSetErr = errors.New("rwset failed")
		require.Panics(t, func() { _ = tx.Outputs() })
	})

	t.Run("inputs", func(t *testing.T) {
		t.Parallel()
		tx, _, driverTx := newTestStateTransaction(errNS)
		driverTx.getRWSetErr = errors.New("rwset failed")
		require.Panics(t, func() { _ = tx.Inputs() })
	})
}

// TestNamespaceOutputsPanicOnWriteLookupError covers the panic inside the loop,
// which is a different call site from the RWSet lookup above.
func TestNamespaceOutputsPanicOnWriteLookupError(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(errNS)
	seedOutput(t, tx, &House{Address: "one", LinearID: "id-1"})
	rwset.getWriteAtErr = errors.New("write lookup failed")

	require.Panics(t, func() { _ = tx.Outputs() })
}

func TestNamespaceInputsPanicOnReadKeyLookupError(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(errNS)
	require.NoError(t, rwset.AddReadAt(errNS, "id-1", nil))
	rwset.getReadKeyAtErr = errors.New("read key lookup failed")

	require.Panics(t, func() { _ = tx.Inputs() })
}

func TestNamespaceCommandsEmptyAndPopulated(t *testing.T) {
	t.Parallel()

	t.Run("no parameters", func(t *testing.T) {
		t.Parallel()
		tx, _, _ := newTestStateTransaction(errNS)
		require.Zero(t, tx.Commands().Count())
	})

	t.Run("appends to existing header", func(t *testing.T) {
		t.Parallel()
		tx, _, _ := newTestStateTransaction(errNS)
		require.NoError(t, tx.AddCommand("create"))
		require.NoError(t, tx.AddCommand("transfer"))

		commands := tx.Commands()
		require.Equal(t, 2, commands.Count())
		require.Equal(t, "create", commands.At(0).Name)
		require.Equal(t, "transfer", commands.At(1).Name)
	})
}

// failingLinearState is an AutoLinearState whose id cannot be derived, so
// AddOutput and Delete fail before touching the RWSet.
type failingLinearState struct {
	err error
}

func (f *failingLinearState) GetLinearID() (string, error) { return "", f.err }

func TestNamespaceStateIDError(t *testing.T) {
	t.Parallel()

	idErr := errors.New("cannot derive id")

	t.Run("add output", func(t *testing.T) {
		t.Parallel()
		tx, rwset, _ := newTestStateTransaction(errNS)
		err := tx.AddOutput(&failingLinearState{err: idErr})
		require.ErrorIs(t, err, idErr)
		require.Zero(t, rwset.NumWrites(errNS), "nothing is written when the id cannot be derived")
	})

	t.Run("delete", func(t *testing.T) {
		t.Parallel()
		tx, rwset, _ := newTestStateTransaction(errNS)
		err := tx.Delete(&failingLinearState{err: idErr})
		require.ErrorIs(t, err, idErr)
		require.Zero(t, rwset.NumWrites(errNS))
	})
}

// TestNamespaceAddOutputEmbeddingStateDerivesInnerID covers the EmbeddingState
// branch of getStateID, which recurses into the wrapped state.
func TestNamespaceAddOutputEmbeddingStateDerivesInnerID(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(errNS)
	inner := &House{Address: "wrapped", Valuation: 11, LinearID: "inner-id"}
	require.NoError(t, tx.AddOutput(&embeddingHouse{Inner: inner}))

	// The write is keyed by the embedded state's linear id, and holds the encoded
	// wrapper, so assert the exact bytes rather than merely that something landed.
	stored, err := rwset.GetState(errNS, "inner-id")
	require.NoError(t, err)

	var got embeddingHouse
	require.NoError(t, json.Unmarshal(stored, &got))
	require.Equal(t, inner, got.Inner)

	// Nothing is written under the wrapper's own generated id.
	require.Equal(t, 1, tx.NumOutputs())
	key, _, err := rwset.GetWriteAt(errNS, 0)
	require.NoError(t, err)
	require.Equal(t, "inner-id", string(key))
}

type embeddingHouse struct {
	Inner *House
}

func (e *embeddingHouse) GetState() any { return e.Inner }

// TestNamespaceAddInputByLinearIDReadsHashHiddenPreimage exercises the `_root_`
// mapping branch on the input path: the stored value is a digest, and the real
// state has to come from the mapping.
func TestNamespaceAddInputByLinearIDReadsHashHiddenPreimage(t *testing.T) {
	t.Parallel()

	tx, _, _ := newTestStateTransaction(errNS)
	original := &House{Address: "hidden", Valuation: 7, LinearID: "id-1"}
	require.NoError(t, tx.AddOutput(original, WithHashHiding()))

	var got House
	require.NoError(t, tx.AddInputByLinearID("id-1", &got))
	require.Equal(t, "hidden", got.Address)
	require.Equal(t, uint64(7), got.Valuation)
}

func TestNamespacePresentReflectsWrites(t *testing.T) {
	t.Parallel()

	tx, _, _ := newTestStateTransaction(errNS)
	require.False(t, tx.Present(), "an untouched namespace is not present")

	seedOutput(t, tx, &House{Address: "one", LinearID: "id-1"})
	require.True(t, tx.Present())
}
