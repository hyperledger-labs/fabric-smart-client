/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"bytes"
	"encoding/gob"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/hyperledger-labs/fabric-smart-client/internal/storage/sqlbuild"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
)

func identity(k string) any { return k }

func renderCondition(c sqlbuild.Condition) (string, []sqlbuild.Param) {
	b := sqlbuild.New()
	c.WriteTo(b)
	return b.Build()
}

func TestBetweenKeys_BothBounds(t *testing.T) {
	t.Parallel()
	sql, args := renderCondition(betweenKeys("pkey", "a", "z", identity))
	require.Equal(t, "(pkey >= $1 AND pkey < $2)", sql)
	require.Equal(t, []sqlbuild.Param{"a", "z"}, args)
}

func TestBetweenKeys_OnlyStart(t *testing.T) {
	t.Parallel()
	sql, args := renderCondition(betweenKeys("pkey", "a", "", identity))
	require.Equal(t, "(pkey >= $1)", sql)
	require.Equal(t, []sqlbuild.Param{"a"}, args)
}

func TestBetweenKeys_OnlyEnd(t *testing.T) {
	t.Parallel()
	sql, args := renderCondition(betweenKeys("pkey", "", "z", identity))
	require.Equal(t, "(pkey < $1)", sql)
	require.Equal(t, []sqlbuild.Param{"z"}, args)
}

// GetStateRange(ctx, ns, "", "") relies on this rendering as a tautology
// rather than as an empty string.
func TestBetweenKeys_NoBounds(t *testing.T) {
	t.Parallel()
	sql, args := renderCondition(betweenKeys("pkey", "", "", identity))
	require.Equal(t, "(1=1)", sql)
	require.Nil(t, args)
}

// GetStateMetadata stores an empty metadata column for states written without
// metadata, so unmarshalling nothing must not be an error.
func TestUnmarshalMetadata_NoInput(t *testing.T) {
	t.Parallel()
	out, err := unmarshalMetadata(nil)
	require.NoError(t, err)
	require.Nil(t, out)

	out, err = unmarshalMetadata([]byte{})
	require.NoError(t, err)
	require.Nil(t, out)
}

func TestUnmarshalMetadata_Garbage(t *testing.T) {
	t.Parallel()
	_, err := unmarshalMetadata([]byte{0xff, 0x00, 0x42})
	require.Error(t, err)
}

// gobOtherTypes gob-encodes types this process has not encoded before, so that the type ids
// gob assigns afterwards, and with them the header of later streams, differ from a fresh
// process.
func gobOtherTypes(t *testing.T) {
	t.Helper()
	type a struct{ A, B int }
	type b struct{ C []string }
	type c map[int]a
	for _, v := range []any{a{1, 2}, b{[]string{"x"}}, c{1: {}}} {
		require.NoError(t, gob.NewEncoder(&bytes.Buffer{}).Encode(v))
	}
}

// unmarshalMetadata reads both forms the vault stores: gob, whatever its header, and
// protobuf VaultMetadata. Empty input is no metadata.
func TestUnmarshalMetadataBothForms(t *testing.T) {
	t.Parallel()
	in := map[string][]byte{"a": []byte("one"), "b": []byte("two")}

	gobOtherTypes(t)
	var buf bytes.Buffer
	require.NoError(t, gob.NewEncoder(&buf).Encode(in))
	require.NotEqual(t, byte(0x0a), buf.Bytes()[0], "gob never starts with the entries tag")
	out, err := unmarshalMetadata(buf.Bytes())
	require.NoError(t, err)
	require.Equal(t, in, out, "gob")

	raw, err := proto.Marshal(&protos.VaultMetadata{Entries: &protos.Entries{Values: in}})
	require.NoError(t, err)
	out, err = unmarshalMetadata(raw)
	require.NoError(t, err)
	require.Equal(t, in, out, "protobuf")

	for _, empty := range [][]byte{nil, {}} {
		out, err = unmarshalMetadata(empty)
		require.NoError(t, err)
		require.Empty(t, out, "empty")
	}

	_, err = unmarshalMetadata([]byte{0x0a, 0xff})
	require.Error(t, err, "truncated protobuf")
}

// An empty map has a protobuf form, 0a 00, which reads back as an empty map, not as no metadata.
func TestMetadataEmptyMapProtobuf(t *testing.T) {
	t.Parallel()
	for _, m := range []map[string][]byte{{}, nil} {
		raw, err := marshallMetadata(m)
		require.NoError(t, err)
		require.Equal(t, []byte{0x0a, 0x00}, raw)
		out, err := unmarshalMetadata(raw)
		require.NoError(t, err)
		require.NotNil(t, out)
		require.Empty(t, out)
	}
}

// marshallMetadata writes protobuf VaultMetadata, deterministically, and refuses a key that is
// not valid UTF-8, naming it.
func TestMarshallMetadata(t *testing.T) {
	t.Parallel()
	in := map[string][]byte{"a": []byte("one"), "b": []byte("two"), "c": []byte("three")}
	raw, err := marshallMetadata(in)
	require.NoError(t, err)
	want, err := proto.MarshalOptions{Deterministic: true}.Marshal(&protos.VaultMetadata{Entries: &protos.Entries{Values: in}})
	require.NoError(t, err)
	require.Equal(t, want, raw)
	out, err := unmarshalMetadata(raw)
	require.NoError(t, err)
	require.Equal(t, in, out)

	_, err = marshallMetadata(map[string][]byte{"a": nil, "\xff": []byte("v")})
	require.EqualError(t, err, `metadata key ["\xff"] is not valid UTF-8`)
}

// The store cannot tell the platform of a flat transaction without a channel key, so such a
// transaction stays JSON and the step counts it apart.
func TestToEnvelopeKeylessFlat(t *testing.T) {
	t.Parallel()
	_, err := toEnvelope(keylessFlatTxJSON)
	require.ErrorIs(t, err, protos.ErrNoChannelKey)
	_, err = toEnvelope(endorseTxJSON(t, 0))
	require.NoError(t, err, "a keyless Fabric transaction is wrapped, so it converts")
}

// A transaction of a platform without a registered format stays JSON; the step counts it apart.
func TestToEnvelopeUnregisteredPlatform(t *testing.T) {
	t.Parallel()
	_, err := toEnvelope(unregisteredTxJSON)
	require.ErrorIs(t, err, protos.ErrNoFormat)
	require.ErrorContains(t, err, "[unregistered]")
}

// toEnvelope converts JSON only if its envelope reads back as the same transaction: the
// differences a round trip makes by design do not count, any other difference does.
func TestToEnvelopeComparesRoundTrip(t *testing.T) {
	t.Parallel()
	for i := range 2 {
		raw := bytesTxJSON(t, unsignedBytesTx(i))
		env, err := toEnvelope(raw)
		require.NoError(t, err, "Bytes row %d", i)
		back, err := protos.JSONFromEnvelope(env)
		require.NoError(t, err)
		require.NotEqual(t, raw, back, "row %d differs in the ignored fields only", i)
	}

	_, err := toEnvelope(lossyTxJSON(t))
	require.ErrorIs(t, err, errRoundTrip)
}

// bytesTxJSON gives the bytes the Fabric transaction core gives for the same transaction.
func TestBytesTxJSONMatchesTransactionBytes(t *testing.T) {
	t.Parallel()
	for i := range 2 {
		want := unsignedBytesTx(i)
		tx := &transaction.Transaction{
			TCreator: want.TCreator, TNonce: want.TNonce, TTxID: want.TTxID, TNetwork: want.TNetwork,
			TChannel: want.TChannel, TChaincode: want.TChaincode, TFunction: want.TFunction,
			RWSet: want.RWSet, TTransient: want.TTransient,
		}
		raw, err := (&transaction.WrappedTransaction{Transaction: tx, TransactionType: fdriver.EndorserTransaction}).Bytes()
		require.NoError(t, err)
		require.Equal(t, string(raw), string(bytesTxJSON(t, want)), "row %d", i)
	}
}
