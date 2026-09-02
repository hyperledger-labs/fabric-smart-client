/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

const tagNS = "tagns"

func TestGetFieldMappingInvalidKey(t *testing.T) {
	t.Parallel()

	tx, _, _ := newTestStateTransaction(tagNS)

	// A NUL byte is rejected by the composite-key encoding, so the mapping key
	// cannot be built. An empty key is not rejected -- see
	// TestFieldMappingKeyAcceptsEmptyKey.
	_, err := tx.getFieldMapping(tagNS, "\x00", true)
	require.Error(t, err)
	require.ErrorContains(t, err, "creating mapping key")
}

// TestGetFieldMappingNotInTransientAndNotAllowedToLookUp covers the flag=false
// short circuit: the RWSet is never consulted and a nil mapping is returned.
//
// The nil here is recorded as current behaviour, not as desirable behaviour, and it
// is asymmetric with the flag=true case (see
// TestGetFieldMappingAbsentEverywhereReturnsEmpty, which yields a non-nil empty map).
// TODO: GetInputAt passes flag=false on exactly the certifiedInputs fallback path, so
// a hash-hidden state served from there never recovers its `_root_` preimage and the
// read fails. See the TODO in namespace.go's GetInputAt.
func TestGetFieldMappingNotInTransientAndNotAllowedToLookUp(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(tagNS)
	rwset.getStateMetadataErr = errors.New("must not be called")

	mapping, err := tx.getFieldMapping(tagNS, "id-1", false)
	require.NoError(t, err)
	require.Nil(t, mapping)
}

func TestGetFieldMappingRWSetError(t *testing.T) {
	t.Parallel()

	tx, _, driverTx := newTestStateTransaction(tagNS)
	driverTx.getRWSetErr = errors.New("rwset failed")

	_, err := tx.getFieldMapping(tagNS, "id-1", true)
	require.Error(t, err)
	require.ErrorContains(t, err, "getting rw set")
}

func TestGetFieldMappingMetadataError(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(tagNS)
	rwset.getStateMetadataErr = errors.New("metadata failed")

	_, err := tx.getFieldMapping(tagNS, "id-1", true)
	require.Error(t, err)
	require.ErrorContains(t, err, "getting metadata")
}

// TestGetFieldMappingAbsentEverywhereReturnsEmpty distinguishes "no mapping" from
// an error: an absent mapping yields an empty map, not nil and not a failure.
func TestGetFieldMappingAbsentEverywhereReturnsEmpty(t *testing.T) {
	t.Parallel()

	tx, _, _ := newTestStateTransaction(tagNS)

	mapping, err := tx.getFieldMapping(tagNS, "id-1", true)
	require.NoError(t, err)
	require.NotNil(t, mapping)
	require.Empty(t, mapping)
}

// TestGetFieldMappingReadsFromStateMetadata covers the RWSet fallback: the mapping
// is absent from the transient store but present in the committed metadata.
func TestGetFieldMappingReadsFromStateMetadata(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction(tagNS)
	mappingKey, err := fieldMappingKey(tagNS, "id-1")
	require.NoError(t, err)

	raw, err := json.Marshal(map[string][]byte{"_root_": []byte("preimage")})
	require.NoError(t, err)
	require.NoError(t, rwset.SetStateMetadata(tagNS, "id-1", cdriver.Metadata{mappingKey: raw}))

	mapping, err := tx.getFieldMapping(tagNS, "id-1", true)
	require.NoError(t, err)
	require.Equal(t, []byte("preimage"), mapping["_root_"])
}

func TestGetFieldMappingCorruptPayload(t *testing.T) {
	t.Parallel()

	tx, _, _ := newTestStateTransaction(tagNS)
	mappingKey, err := fieldMappingKey(tagNS, "id-1")
	require.NoError(t, err)
	require.NoError(t, tx.SetTransient(mappingKey, []byte("{bad-json")))

	_, err = tx.getFieldMapping(tagNS, "id-1", true)
	require.Error(t, err)
	require.ErrorContains(t, err, "unmarshalling mapping")
}

// TestSetFieldMappingEmptyIsNoOp records that an empty mapping is skipped rather
// than written, so no transient entry is created.
func TestSetFieldMappingEmptyIsNoOp(t *testing.T) {
	t.Parallel()

	tx, _, _ := newTestStateTransaction(tagNS)
	require.NoError(t, tx.setFieldMapping(tagNS, "id-1", map[string][]byte{}))

	mappingKey, err := fieldMappingKey(tagNS, "id-1")
	require.NoError(t, err)
	require.Empty(t, tx.GetTransient(mappingKey))
}

func TestSetFieldMappingRoundTrip(t *testing.T) {
	t.Parallel()

	tx, _, _ := newTestStateTransaction(tagNS)
	want := map[string][]byte{"_root_": []byte("value"), "other": []byte("x")}
	require.NoError(t, tx.setFieldMapping(tagNS, "id-1", want))

	got, err := tx.getFieldMapping(tagNS, "id-1", true)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestFieldMappingKeyRejectsNUL(t *testing.T) {
	t.Parallel()

	_, err := fieldMappingKey(tagNS, "\x00")
	require.Error(t, err)
	require.ErrorContains(t, err, "U+0000")
}

// TestFieldMappingKeyAcceptsEmptyKey records that an empty key is not an error:
// the composite-key encoding only rejects U+0000 and U+10FFFF, so an empty
// attribute produces a valid -- if degenerate -- key.
func TestFieldMappingKeyAcceptsEmptyKey(t *testing.T) {
	t.Parallel()

	key, err := fieldMappingKey(tagNS, "")
	require.NoError(t, err)
	require.NotEmpty(t, key)
}

// TestMarshalTagsHashOnStringFailsClosed pins a deliberate refusal: hash-hiding a
// string field would emit a hash whose preimage is never retained, so it could be
// neither recovered nor verified. Both directions reject it rather than emitting
// an unverifiable value.
func TestMarshalTagsHashOnStringFailsClosed(t *testing.T) {
	t.Parallel()

	type stringTagged struct {
		Secret string `state:"hash"`
	}

	n := &Namespace{}
	_, _, err := n.marshalTags(nil, &stringTagged{Secret: "s"})
	require.Error(t, err)
	require.ErrorContains(t, err, "not supported for string field")

	err = n.unmarshalTags(nil, &stringTagged{Secret: "s"}, map[string][]byte{})
	require.Error(t, err)
	require.ErrorContains(t, err, "not supported for string field")
}

// TestTagsHashRejectsUnsupportedKinds checks that state:"hash" on a field that
// cannot be hashed is rejected.
//
// Previously an unhandled kind fell through the switch silently, so the value was
// written in the clear while the caller believed it was hidden, and a non-byte slice
// panicked inside reflect.Value.Bytes.
//
// marshalTags now refuses all of them. unmarshalTags refuses the slice kinds
// unconditionally -- marshalTags panicked on those before it could write anything, so
// no committed state can hold one -- but for the remaining kinds it refuses only when
// a preimage is present, because a release that skipped the kind silently left
// readable state behind. See TestUnmarshalTagsPassesThroughUnhashedUnsupportedKind.
func TestTagsHashRejectsUnsupportedKinds(t *testing.T) {
	t.Parallel()

	type intTagged struct {
		Count int `state:"hash"`
	}
	type stringSliceTagged struct {
		Names []string `state:"hash"`
	}
	type intSliceTagged struct {
		Sizes []int `state:"hash"`
	}
	type mapTagged struct {
		Attrs map[string]string `state:"hash"`
	}

	for _, tc := range []struct {
		name  string
		field string
		state any
		// readRejectsWithoutPreimage marks the kinds handled by the slice guard, which
		// cannot have produced legacy data and so fails closed either way.
		readRejectsWithoutPreimage bool
	}{
		{name: "int", field: "Count", state: &intTagged{Count: 7}},
		{name: "map", field: "Attrs", state: &mapTagged{Attrs: map[string]string{"k": "v"}}},
		{
			name: "string slice", field: "Names",
			state:                      &stringSliceTagged{Names: []string{"a", "b"}},
			readRejectsWithoutPreimage: true,
		},
		{
			name: "int slice", field: "Sizes",
			state:                      &intSliceTagged{Sizes: []int{1, 2}},
			readRejectsWithoutPreimage: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			n := &Namespace{}
			_, _, err := n.marshalTags(nil, tc.state)
			require.Error(t, err, "marshalTags must refuse a field it cannot hash")
			require.ErrorContains(t, err, "is not supported for field")

			// A preimage means something hashed the field and this code cannot verify
			// it, so the read side always refuses.
			err = n.unmarshalTags(nil, tc.state, map[string][]byte{tc.field: []byte("preimage")})
			require.Error(t, err, "unmarshalTags must refuse a hashed field it cannot verify")
			require.ErrorContains(t, err, "is not supported for field")

			err = n.unmarshalTags(nil, tc.state, map[string][]byte{})
			if tc.readRejectsWithoutPreimage {
				require.Error(t, err, "a slice of this shape could never have been committed")
				require.ErrorContains(t, err, "is not supported for field")
			} else {
				require.NoError(t, err, "an unhashed legacy field must stay readable")
			}
		})
	}
}

// TestUnmarshalTagsPassesThroughUnhashedUnsupportedKind covers the upgrade path: a
// release that silently skipped an unsupported kind wrote the value in the clear and
// recorded no preimage, so reading that state back has to keep working -- otherwise
// upgrading strands already-committed data with no way to migrate it. A fixed-size
// byte array is the realistic shape.
func TestUnmarshalTagsPassesThroughUnhashedUnsupportedKind(t *testing.T) {
	t.Parallel()

	type arrayTagged struct {
		Fingerprint [4]byte `state:"hash"`
	}

	n := &Namespace{}
	state := &arrayTagged{Fingerprint: [4]byte{1, 2, 3, 4}}

	// The write side refuses, so no new state of this shape can be produced.
	_, _, err := n.marshalTags(nil, state)
	require.Error(t, err)
	require.ErrorContains(t, err, "of kind [array]")

	// The read side leaves the legacy value alone.
	require.NoError(t, n.unmarshalTags(nil, state, map[string][]byte{}))
	require.Equal(t, [4]byte{1, 2, 3, 4}, state.Fingerprint, "the committed value is untouched")

	// A preimage, though, means it was hashed and cannot be verified here.
	err = n.unmarshalTags(nil, state, map[string][]byte{"Fingerprint": []byte("preimage")})
	require.Error(t, err)
	require.ErrorContains(t, err, "is not supported for field")
}

// TestTagsHashRejectsNamedByteElementSlice guards the hole an element-kind check
// leaves open: []MyByte has element Kind Uint8, so `Elem().Kind() != reflect.Uint8`
// admits it, reflect.Value.Bytes then succeeds, and writing the digest back panics in
// reflect.Value.Set because []byte is not assignable to []MyByte. Assignability is
// the check that actually closes it.
func TestTagsHashRejectsNamedByteElementSlice(t *testing.T) {
	t.Parallel()

	type namedByte byte
	type namedElemTagged struct {
		Data []namedByte `state:"hash"`
	}

	n := &Namespace{}
	state := &namedElemTagged{Data: []namedByte{1, 2, 3}}

	require.NotPanics(t, func() {
		_, _, err := n.marshalTags(nil, state)
		require.Error(t, err, "marshalTags must refuse a slice it cannot write back")
		require.ErrorContains(t, err, "is not supported for field")
	})

	// A matching digest, so the hash check would pass and reach the field.Set that
	// used to panic.
	sum := sha256.Sum256([]byte{1, 2, 3})
	hashed := make([]namedByte, len(sum))
	for i, b := range sum {
		hashed[i] = namedByte(b)
	}
	require.NotPanics(t, func() {
		err := n.unmarshalTags(nil, &namedElemTagged{Data: hashed},
			map[string][]byte{"Data": {1, 2, 3}})
		require.Error(t, err, "unmarshalTags must refuse it too")
		require.ErrorContains(t, err, "is not supported for field")
	})
}

// TestTagsHashAcceptsNamedByteSliceType is the counterpart: a named slice type whose
// underlying type is []byte *is* assignable from []byte, so the tighter check must not
// start rejecting it.
func TestTagsHashAcceptsNamedByteSliceType(t *testing.T) {
	t.Parallel()

	type hashDigest []byte
	type namedSliceTagged struct {
		Data hashDigest `state:"hash"`
	}

	n := &Namespace{}
	hashed, mapping, err := n.marshalTags(nil, &namedSliceTagged{Data: hashDigest("secret")})
	require.NoError(t, err, "a named []byte type must still be hashable")
	require.Equal(t, []byte("secret"), mapping["Data"], "the preimage is retained")

	digest := sha256.Sum256([]byte("secret"))
	require.Equal(t, hashDigest(digest[:]), hashed.(*namedSliceTagged).Data,
		"the field holds the digest")

	require.NoError(t, n.unmarshalTags(nil, hashed, mapping))
	require.Equal(t, hashDigest("secret"), hashed.(*namedSliceTagged).Data,
		"the preimage is restored")
}

// TestTagsHashOnNonByteSliceDoesNotPanic is the regression guard for the panic
// this used to cause: reflect.Value.Bytes panics on a slice whose elements are not
// bytes, so the kind check has to run before it.
func TestTagsHashOnNonByteSliceDoesNotPanic(t *testing.T) {
	t.Parallel()

	type stringSliceTagged struct {
		Names []string `state:"hash"`
	}

	n := &Namespace{}
	require.NotPanics(t, func() {
		_, _, _ = n.marshalTags(nil, &stringSliceTagged{Names: []string{"a"}})
	})
	require.NotPanics(t, func() {
		_ = n.unmarshalTags(nil, &stringSliceTagged{Names: []string{"a"}}, map[string][]byte{})
	})
}

// TestMarshalTagsIgnoresUnknownTagValue records that an unrecognised state tag is
// ignored, so adding one is not a compile- or run-time error.
func TestMarshalTagsIgnoresUnknownTagValue(t *testing.T) {
	t.Parallel()

	type oddlyTagged struct {
		Data []byte `state:"encrypt"`
	}

	n := &Namespace{}
	dest, mapping, err := n.marshalTags(nil, &oddlyTagged{Data: []byte("x")})
	require.NoError(t, err)
	require.Empty(t, mapping)
	require.Equal(t, []byte("x"), dest.(*oddlyTagged).Data)
}

// TestUnmarshalTagsMissingMappingEntry checks a hash-tagged field with no preimage
// is rejected: without it the hash cannot be verified, so it fails closed.
func TestUnmarshalTagsMissingMappingEntry(t *testing.T) {
	t.Parallel()

	n := &Namespace{}
	hashed, mapping, err := n.marshalTags(nil, &Asset{ID: "a1", PrivateProperties: []byte("private")})
	require.NoError(t, err)
	require.NotEmpty(t, mapping)

	err = n.unmarshalTags(nil, hashed, map[string][]byte{})
	require.Error(t, err)
	require.ErrorContains(t, err, "mapping not found")
}

// TestUnmarshalTagsHashMismatch covers the integrity check: a preimage that does
// not hash to the stored digest is rejected.
func TestUnmarshalTagsHashMismatch(t *testing.T) {
	t.Parallel()

	n := &Namespace{}
	hashed, _, err := n.marshalTags(nil, &Asset{ID: "a1", PrivateProperties: []byte("private")})
	require.NoError(t, err)

	err = n.unmarshalTags(nil, hashed, map[string][]byte{"PrivateProperties": []byte("tampered")})
	require.Error(t, err)
	require.ErrorContains(t, err, "failed checking hash")
}

// TestMarshalUnmarshalTagsRoundTrip is the success counterpart: a matching
// preimage restores the original value.
func TestMarshalUnmarshalTagsRoundTrip(t *testing.T) {
	t.Parallel()

	n := &Namespace{}
	original := &Asset{ID: "a1", PrivateProperties: []byte("private")}
	hashed, mapping, err := n.marshalTags(nil, original)
	require.NoError(t, err)
	require.NotEqual(t, original.PrivateProperties, hashed.(*Asset).PrivateProperties,
		"the field is replaced by its digest")

	require.NoError(t, n.unmarshalTags(nil, hashed, mapping))
	require.Equal(t, []byte("private"), hashed.(*Asset).PrivateProperties)
}
