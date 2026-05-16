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

type testSerializable struct {
	raw    []byte
	err    error
	setRaw []byte
	setErr error
}

func (t *testSerializable) Bytes() ([]byte, error) {
	if t.err != nil {
		return nil, t.err
	}
	return t.raw, nil
}

func (t *testSerializable) SetFromBytes(raw []byte) error {
	t.setRaw = append([]byte(nil), raw...)
	return t.setErr
}

type testUnmarshaller struct {
	err     error
	lastRaw []byte
	lastV   interface{}
}

func (t *testUnmarshaller) Unmarshal(raw []byte, v interface{}) error {
	t.lastRaw = append([]byte(nil), raw...)
	t.lastV = v
	return t.err
}

func TestCreateCompositeKeyAndRangeKeys(t *testing.T) {
	t.Parallel()

	key, err := CreateCompositeKey("asset", []string{"type", "id"})
	require.NoError(t, err)
	require.Equal(t, "\x00asset\x00type\x00id\x00", key)

	start, end, err := CreateRangeKeysForPartialCompositeKey("asset", []string{"type"})
	require.NoError(t, err)
	require.Equal(t, "\x00asset\x00type\x00", start)
	require.Equal(t, start+string(MaxUnicodeRuneValue), end)
}

func TestCreateCompositeKeyValidationErrors(t *testing.T) {
	t.Parallel()

	_, err := CreateCompositeKey("asset\x00type", []string{"id"})
	require.Error(t, err)
	require.ErrorContains(t, err, "U+0000")

	_, err = CreateCompositeKey("asset", []string{"\xff\xfe"})
	require.Error(t, err)
	require.ErrorContains(t, err, "not a valid utf8 string")

	err = validateCompositeKeyAttribute(string([]rune{'a', MaxUnicodeRuneValue}))
	require.Error(t, err)
	require.ErrorContains(t, err, "U+10FFFF")
}

func TestCompileServiceOptions(t *testing.T) {
	t.Parallel()

	opts, err := CompileServiceOptions(
		WithNetwork("network1"),
		WithChannel("channel1"),
		WithIdentity("alice"),
	)
	require.NoError(t, err)
	require.Equal(t, "network1", opts.Network)
	require.Equal(t, "channel1", opts.Channel)
	require.Equal(t, "alice", opts.Identity)

	expectedErr := errors.New("option failed")
	_, err = CompileServiceOptions(func(*ServiceOptions) error { return expectedErr })
	require.ErrorIs(t, err, expectedErr)
}

func TestOutputAndInputOptions(t *testing.T) {
	t.Parallel()

	out := &addOutputOptions{}
	require.NoError(t, WithContract("c1")(out))
	require.NoError(t, WithHashHiding()(out))
	require.NoError(t, WithStateBasedEndorsement()(out))
	require.Equal(t, "c1", out.contract)
	require.True(t, out.hashHiding)
	require.True(t, out.sbe)

	in := &addInputOptions{}
	require.NoError(t, WithCertification()(in))
	require.True(t, in.certification)
}

func TestUnmarshalDelegatesToUnmarshaller(t *testing.T) {
	t.Parallel()

	u := &testUnmarshaller{}
	dst := &struct{}{}
	raw := []byte("payload")

	err := Unmarshal(u, raw, dst)
	require.NoError(t, err)
	require.Equal(t, raw, u.lastRaw)
	require.Same(t, dst, u.lastV)
}

func TestJSONCodecMarshal(t *testing.T) {
	t.Parallel()

	codec := &JSONCodec{}

	t.Run("serializable", func(t *testing.T) {
		t.Parallel()
		s := &testSerializable{raw: []byte("serialized")}
		raw, err := codec.Marshal(s)
		require.NoError(t, err)
		require.Equal(t, []byte("serialized"), raw)
	})

	t.Run("serializable error", func(t *testing.T) {
		t.Parallel()
		s := &testSerializable{err: errors.New("marshal failed")}
		_, err := codec.Marshal(s)
		require.EqualError(t, err, "marshal failed")
	})

	t.Run("json fallback", func(t *testing.T) {
		t.Parallel()
		raw, err := codec.Marshal(map[string]string{"k": "v"})
		require.NoError(t, err)
		require.JSONEq(t, `{"k":"v"}`, string(raw))
	})
}

func TestJSONCodecUnmarshal(t *testing.T) {
	t.Parallel()

	codec := &JSONCodec{}

	t.Run("state", func(t *testing.T) {
		t.Parallel()
		s := &testSerializable{}
		err := codec.Unmarshal([]byte("serialized"), s)
		require.NoError(t, err)
		require.Equal(t, []byte("serialized"), s.setRaw)
	})

	t.Run("state error", func(t *testing.T) {
		t.Parallel()
		s := &testSerializable{setErr: errors.New("set failed")}
		err := codec.Unmarshal([]byte("serialized"), s)
		require.EqualError(t, err, "set failed")
	})

	t.Run("json fallback", func(t *testing.T) {
		t.Parallel()
		var dst struct {
			Key string `json:"key"`
		}
		err := codec.Unmarshal([]byte(`{"key":"value"}`), &dst)
		require.NoError(t, err)
		require.Equal(t, "value", dst.Key)
	})
}

func TestCreateNonceFunctions(t *testing.T) {
	t.Parallel()

	nonce, err := CreateNonce()
	require.NoError(t, err)
	require.Len(t, nonce, 24)

	nonceOrPanic := CreateNonceOrPanic()
	require.Len(t, nonceOrPanic, 24)
}
