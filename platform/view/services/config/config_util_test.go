/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"encoding/pem"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	koanfyaml "github.com/knadh/koanf/parsers/yaml"
	koanfbytes "github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestStruct struct {
	Slice    []string
	Size     uint32
	Content  string
	Certs    []string
	Duration time.Duration
}

func TestEnhancedExactUnmarshal(t *testing.T) {
	t.Parallel()
	// Prepare a temporary file for testing stringFromFileDecodeHook
	contentFile, err := os.CreateTemp(t.TempDir(), "test-content")
	require.NoError(t, err)
	_, err = contentFile.WriteString("hello world")
	require.NoError(t, err)
	require.NoError(t, contentFile.Close())

	// Prepare a temporary file for testing pemBlocksFromFileDecodeHook
	pemFile, err := os.CreateTemp(t.TempDir(), "test-pem")
	require.NoError(t, err)
	pemData := `
-----BEGIN CERTIFICATE-----
YmFzZTY0Cg==
-----END CERTIFICATE-----
`
	_, err = pemFile.WriteString(pemData)
	require.NoError(t, err)
	require.NoError(t, pemFile.Close())

	k := koanf.New(".")
	raw := []byte(`
test:
  slice: "[a, b, c]"
  size: 10mb
  content:
    file: ` + contentFile.Name() + `
  certs:
    File: ` + pemFile.Name() + `
  duration: 10s
`)
	err = k.Load(koanfbytes.Provider(raw), koanfyaml.Parser())
	require.NoError(t, err)

	var ts TestStruct
	err = EnhancedExactUnmarshal(k, "test", &ts)
	require.NoError(t, err)

	assert.Equal(t, []string{"a", "b", "c"}, ts.Slice)
	assert.Equal(t, uint32(10*1024*1024), ts.Size)
	assert.Equal(t, "hello world", ts.Content)
	require.Len(t, ts.Certs, 1)
	assert.Contains(t, ts.Certs[0], "BEGIN CERTIFICATE")
	assert.Equal(t, 10*time.Second, ts.Duration)

	// Test errors
	err = EnhancedExactUnmarshal(k, "test", ts) // Not a pointer
	require.Error(t, err)
}

func TestByteSizeDecodeHookExtra(t *testing.T) {
	t.Parallel()
	k := koanf.New(".")
	raw := []byte(`
test:
  size1: 1gb
  size2: 1kb
  size3: 5000000k # Too large for uint32
`)
	err := k.Load(koanfbytes.Provider(raw), koanfyaml.Parser())
	require.NoError(t, err)

	var ts struct {
		Size1 uint32
		Size2 uint32
		Size3 uint32
	}
	err = EnhancedExactUnmarshal(k, "test", &ts)
	require.Error(t, err) // size3 overflows
	assert.Contains(t, err.Error(), "overflows uint32")
}

func TestCustomDecodeHook(t *testing.T) {
	t.Parallel()
	str := reflect.TypeFor[string]()

	_, err := customDecodeHook(str, str, 42)
	require.ErrorContains(t, err, "unexpected data type")

	out, err := customDecodeHook(str, str, "[a, b ,c]")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, out)

	out, err = customDecodeHook(str, str, "[]")
	require.NoError(t, err)
	assert.Equal(t, []string{""}, out)

	out, err = customDecodeHook(str, str, "plain")
	require.NoError(t, err)
	assert.Equal(t, "plain", out)

	out, err = customDecodeHook(reflect.TypeFor[int](), str, 42)
	require.NoError(t, err)
	assert.Equal(t, 42, out)
}

func TestByteSizeDecodeHook(t *testing.T) {
	t.Parallel()

	_, err := byteSizeDecodeHook(reflect.String, reflect.Uint32, 42)
	require.ErrorContains(t, err, "unexpected data type")

	for _, in := range []string{"", "10tb", "abc"} {
		out, err := byteSizeDecodeHook(reflect.String, reflect.Uint32, in)
		require.NoError(t, err)
		assert.Equal(t, in, out)
	}

	_, err = byteSizeDecodeHook(reflect.String, reflect.Uint32, "99999999999999999999k")
	require.ErrorContains(t, err, "invalid byte size value")

	// 2^54 g wraps to 0 in uint64 when shifted, so the overflow must be caught before shifting.
	for _, in := range []string{"4g", "4096m", "4194304k", "18014398509481984g"} {
		_, err = byteSizeDecodeHook(reflect.String, reflect.Uint32, in)
		require.ErrorContains(t, err, "overflows uint32", in)
	}

	for in, want := range map[string]uint64{"1k": 1 << 10, "2MB": 2 << 20, "3g": 3 << 30, "5 kb": 5 << 10, "010k": 10 << 10, "4095m": 4095 << 20, "4194303k": 4194303 << 10} {
		out, err := byteSizeDecodeHook(reflect.String, reflect.Uint32, in)
		require.NoError(t, err, in)
		assert.Equal(t, want, out, in)
	}

	out, err := byteSizeDecodeHook(reflect.String, reflect.Uint64, "1k")
	require.NoError(t, err)
	assert.Equal(t, "1k", out)
}

func TestStringFromFileDecodeHook(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "content")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o600))

	for _, key := range []string{"File", "file"} {
		out, err := stringFromFileDecodeHook(reflect.Map, reflect.String, map[string]any{key: path})
		require.NoError(t, err)
		assert.Equal(t, "hello", out)
	}

	_, err := stringFromFileDecodeHook(reflect.Map, reflect.String, map[string]any{"File": nil})
	require.ErrorContains(t, err, "value of File: was nil")

	_, err = stringFromFileDecodeHook(reflect.Map, reflect.String, map[string]any{"File": 42})
	require.ErrorContains(t, err, "unexpected File value type")

	_, err = stringFromFileDecodeHook(reflect.Map, reflect.String, map[string]any{"File": filepath.Join(t.TempDir(), "missing")})
	require.ErrorIs(t, err, fs.ErrNotExist)

	_, err = stringFromFileDecodeHook(reflect.Map, reflect.String, map[string]string{"File": path})
	require.ErrorContains(t, err, "unexpected data type")

	noKey := map[string]any{"other": path}
	out, err := stringFromFileDecodeHook(reflect.Map, reflect.String, noKey)
	require.NoError(t, err)
	assert.Equal(t, noKey, out)

	out, err = stringFromFileDecodeHook(reflect.String, reflect.String, path)
	require.NoError(t, err)
	assert.Equal(t, path, out)

	out, err = stringFromFileDecodeHook(reflect.Map, reflect.Int, noKey)
	require.NoError(t, err)
	assert.Equal(t, noKey, out)
}

func TestPemBlocksFromFileDecodeHook(t *testing.T) {
	t.Parallel()
	cert := &pem.Block{Type: "CERTIFICATE", Bytes: []byte("cert")}
	path := filepath.Join(t.TempDir(), "blocks.pem")
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(f, cert))
	require.NoError(t, pem.Encode(f, &pem.Block{Type: "PRIVATE KEY", Bytes: []byte("key")}))
	require.NoError(t, pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Headers: map[string]string{"k": "v"}, Bytes: []byte("hdr")}))
	require.NoError(t, f.Close())
	want := []string{string(pem.EncodeToMemory(cert))}

	for _, in := range []any{
		map[string]string{"File": path},
		map[string]string{"file": path},
		map[string]any{"File": path},
		map[string]any{"file": path},
	} {
		out, err := pemBlocksFromFileDecodeHook(reflect.Map, reflect.Slice, in)
		require.NoError(t, err)
		assert.Equal(t, want, out)
	}

	_, err = pemBlocksFromFileDecodeHook(reflect.Map, reflect.Slice, map[string]any{"File": ""})
	require.ErrorContains(t, err, "value of File: was nil")

	_, err = pemBlocksFromFileDecodeHook(reflect.Map, reflect.Slice, map[string]any{"File": filepath.Join(t.TempDir(), "missing")})
	require.ErrorIs(t, err, fs.ErrNotExist)

	in := map[string]any{"File": path}
	out, err := pemBlocksFromFileDecodeHook(reflect.Map, reflect.String, in)
	require.NoError(t, err)
	assert.Equal(t, in, out)

	out, err = pemBlocksFromFileDecodeHook(reflect.String, reflect.Slice, path)
	require.NoError(t, err)
	assert.Equal(t, path, out)
}

func TestStrictUnmarshalSubtreeNonPointer(t *testing.T) {
	t.Parallel()
	var out struct{ A string }
	err := StrictUnmarshalSubtree(map[string]any{"a": "b"}, out)
	require.ErrorContains(t, err, "failed building strict decoder")
}
