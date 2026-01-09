/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package golang

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/stretchr/testify/require"
)

func TestWriteBytesToPackage(t *testing.T) {
	tempDir := t.TempDir()

	buf := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)

	// Create a file and write it to tar writer
	filename := "test.txt"
	filecontent := "hello"
	filePath := filepath.Join(tempDir, filename)
	err := os.WriteFile(filePath, bytes.NewBufferString(filecontent).Bytes(), 0o600)
	require.NoError(t, err, "Error creating file %s", filePath)

	err = WriteBytesToPackage([]byte(filecontent), filePath, filename, tw)
	require.NoError(t, err, "Error returned by WriteFileToPackage while writing existing file")
	tw.Close()
	gw.Close()

	// Read the file from the archive and check the name and file content
	r := bytes.NewReader(buf.Bytes())
	gr, err := gzip.NewReader(r)
	require.NoError(t, err, "Error creating a gzip reader")
	defer gr.Close()

	tr := tar.NewReader(gr)
	header, err := tr.Next()
	require.NoError(t, err, "Error getting the file from the tar")
	require.Equal(t, filename, header.Name, "filename read from archive does not match what was added")
	require.Equal(t, time.Time{}, header.AccessTime, "expected zero access time")
	require.Equal(t, time.Unix(0, 0), header.ModTime, "expected zero modification time")
	require.Equal(t, time.Time{}, header.ChangeTime, "expected zero change time")
	require.Equal(t, int64(0o100644), header.Mode, "expected regular file mode")
	require.Equal(t, 500, header.Uid, "expected 500 uid")
	require.Equal(t, 500, header.Gid, "expected 500 gid")
	require.Equal(t, "", header.Uname, "expected empty user name")
	require.Equal(t, "", header.Gname, "expected empty group name")

	b := make([]byte, 5)
	n, err := tr.Read(b)
	require.Equal(t, 5, n)
	require.True(t, err == nil || err == io.EOF, "Error reading file from the archive") // go1.10 returns io.EOF
	require.Equal(t, filecontent, string(b), "file content from archive does not equal original content")

	t.Run("non existent file", func(t *testing.T) {
		tw := tar.NewWriter(&bytes.Buffer{})
		err := WriteBytesToPackage([]byte(filecontent), "missing-file", "", tw)
		require.Error(t, err, "expected error writing a non existent file")
		require.Contains(t, err.Error(), "missing-file")
	})

	t.Run("closed tar writer", func(t *testing.T) {
		tw := tar.NewWriter(&bytes.Buffer{})
		tw.Close()
		err := WriteBytesToPackage([]byte(filecontent), filePath, "test.txt", tw)
		require.EqualError(t, err, fmt.Sprintf("failed to write header for %s: archive/tar: write after close", filePath))
	})

	t.Run("stream write failure", func(t *testing.T) {
		failWriter := &failingWriter{failAt: 514}
		tw := tar.NewWriter(failWriter)
		err := WriteBytesToPackage([]byte(filecontent), filePath, "test.txt", tw)
		require.EqualError(t, err, fmt.Sprintf("failed to write %s as test.txt: failed-the-write", filePath))
	})
}

func TestWriteFileToPackage(t *testing.T) {
	tempDir := t.TempDir()

	buf := bytes.NewBuffer(nil)
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)

	// Create a file and write it to tar writer
	filename := "test.txt"
	filecontent := "hello"
	filePath := filepath.Join(tempDir, filename)
	err := os.WriteFile(filePath, bytes.NewBufferString(filecontent).Bytes(), 0o600)
	require.NoError(t, err, "Error creating file %s", filePath)

	err = WriteFileToPackage(filePath, filename, tw)
	require.NoError(t, err, "Error returned by WriteFileToPackage while writing existing file")
	tw.Close()
	gw.Close()

	// Read the file from the archive and check the name and file content
	r := bytes.NewReader(buf.Bytes())
	gr, err := gzip.NewReader(r)
	require.NoError(t, err, "Error creating a gzip reader")
	defer gr.Close()

	tr := tar.NewReader(gr)
	header, err := tr.Next()
	require.NoError(t, err, "Error getting the file from the tar")
	require.Equal(t, filename, header.Name, "filename read from archive does not match what was added")
	require.Equal(t, time.Time{}, header.AccessTime, "expected zero access time")
	require.Equal(t, time.Unix(0, 0), header.ModTime, "expected zero modification time")
	require.Equal(t, time.Time{}, header.ChangeTime, "expected zero change time")
	require.Equal(t, int64(0o100644), header.Mode, "expected regular file mode")
	require.Equal(t, 500, header.Uid, "expected 500 uid")
	require.Equal(t, 500, header.Gid, "expected 500 gid")
	require.Equal(t, "", header.Uname, "expected empty user name")
	require.Equal(t, "", header.Gname, "expected empty group name")

	b := make([]byte, 5)
	n, err := tr.Read(b)
	require.Equal(t, 5, n)
	require.True(t, err == nil || err == io.EOF, "Error reading file from the archive") // go1.10 returns io.EOF
	require.Equal(t, filecontent, string(b), "file content from archive does not equal original content")

	t.Run("non existent file", func(t *testing.T) {
		tw := tar.NewWriter(&bytes.Buffer{})
		err := WriteFileToPackage("missing-file", "", tw)
		require.Error(t, err, "expected error writing a non existent file")
		require.Contains(t, err.Error(), "missing-file")
	})

	t.Run("closed tar writer", func(t *testing.T) {
		tw := tar.NewWriter(&bytes.Buffer{})
		tw.Close()
		err := WriteFileToPackage(filePath, "test.txt", tw)
		require.EqualError(t, err, fmt.Sprintf("failed to write header for %s: archive/tar: write after close", filePath))
	})

	t.Run("stream write failure", func(t *testing.T) {
		failWriter := &failingWriter{failAt: 514}
		tw := tar.NewWriter(failWriter)
		err := WriteFileToPackage(filePath, "test.txt", tw)
		require.EqualError(t, err, fmt.Sprintf("failed to write %s as test.txt: failed-the-write", filePath))
	})
}

type failingWriter struct {
	written int
	failAt  int
}

func (f *failingWriter) Write(b []byte) (int, error) {
	f.written += len(b)
	if f.written < f.failAt {
		return len(b), nil
	}
	return 0, errors.New("failed-the-write")
}
