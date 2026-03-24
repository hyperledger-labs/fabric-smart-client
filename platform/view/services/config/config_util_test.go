/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"os"
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
	assert.Len(t, ts.Certs, 1)
	assert.Contains(t, ts.Certs[0], "BEGIN CERTIFICATE")
	assert.Equal(t, 10*time.Second, ts.Duration)

	// Test errors
	err = EnhancedExactUnmarshal(k, "test", ts) // Not a pointer
	require.Error(t, err)
}

func TestByteSizeDecodeHookExtra(t *testing.T) {
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
