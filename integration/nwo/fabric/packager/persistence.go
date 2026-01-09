/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package packager

import (
	"os"
	"path/filepath"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// WriteFile writes a file to the filesystem; it does so atomically
// by first writing to a temp file and then renaming the file so that
// if the operation crashes midway we're not stuck with a bad package
func WriteFile(path, name string, data []byte) error {
	if path == "" {
		return errors.New("empty path not allowed")
	}
	tmpFile, err := os.CreateTemp(path, ".ccpackage.")
	if err != nil {
		return errors.Wrapf(err, "error creating temp file in directory '%s'", path)
	}
	defer os.Remove(tmpFile.Name())

	if n, err := tmpFile.Write(data); err != nil || n != len(data) {
		if err == nil {
			err = errors.Errorf(
				"failed to write the entire content of the file, expected %d, wrote %d",
				len(data), n)
		}
		return errors.Wrapf(err, "error writing to temp file '%s'", tmpFile.Name())
	}

	if err := tmpFile.Close(); err != nil {
		return errors.Wrapf(err, "error closing temp file '%s'", tmpFile.Name())
	}

	if err := os.Rename(tmpFile.Name(), filepath.Join(path, name)); err != nil {
		return errors.Wrapf(err, "error renaming temp file '%s'", tmpFile.Name())
	}

	return nil
}
