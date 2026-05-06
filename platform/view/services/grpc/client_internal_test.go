/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateSecOptsMerging(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	cert1 := []byte("cert1")
	cert2 := []byte("cert2")
	cert3 := []byte("cert3")

	certFile := filepath.Join(tempDir, "cert.pem")
	err := os.WriteFile(certFile, cert1, 0o644)
	require.NoError(t, err)

	connConfig := ConnectionConfig{
		TLSEnabled:       true,
		TLSRootCertFile:  certFile,
		TLSRootCertBytes: [][]byte{cert2, cert3},
	}

	secOpts, err := createSecOpts(connConfig, false, nil)
	require.NoError(t, err)
	require.NotNil(t, secOpts)
	require.True(t, secOpts.UseTLS)
	require.Len(t, secOpts.ServerRootCAs, 3)
	require.Equal(t, cert1, secOpts.ServerRootCAs[0])
	require.Equal(t, cert2, secOpts.ServerRootCAs[1])
	require.Equal(t, cert3, secOpts.ServerRootCAs[2])
}

func TestCreateSecOptsOnlyFile(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	cert1 := []byte("cert1")

	certFile := filepath.Join(tempDir, "cert.pem")
	err := os.WriteFile(certFile, cert1, 0o644)
	require.NoError(t, err)

	connConfig := ConnectionConfig{
		TLSEnabled:      true,
		TLSRootCertFile: certFile,
	}

	secOpts, err := createSecOpts(connConfig, false, nil)
	require.NoError(t, err)
	require.NotNil(t, secOpts)
	require.Len(t, secOpts.ServerRootCAs, 1)
	require.Equal(t, cert1, secOpts.ServerRootCAs[0])
}

func TestCreateSecOptsOnlyBytes(t *testing.T) {
	t.Parallel()
	cert2 := []byte("cert2")
	cert3 := []byte("cert3")

	connConfig := ConnectionConfig{
		TLSEnabled:       true,
		TLSRootCertBytes: [][]byte{cert2, cert3},
	}

	secOpts, err := createSecOpts(connConfig, false, nil)
	require.NoError(t, err)
	require.NotNil(t, secOpts)
	require.Len(t, secOpts.ServerRootCAs, 2)
	require.Equal(t, cert2, secOpts.ServerRootCAs[0])
	require.Equal(t, cert3, secOpts.ServerRootCAs[1])
}

func TestCreateSecOptsMissing(t *testing.T) {
	t.Parallel()
	connConfig := ConnectionConfig{
		TLSEnabled: true,
	}

	_, err := createSecOpts(connConfig, false, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing TLSRootCertFile and TLSRootCertBytes in client config")
}
