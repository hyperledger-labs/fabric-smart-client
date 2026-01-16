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
func WriteFile(filename string, data []byte) error {
	if filename == "" {
		return errors.New("empty filename not allowed")
	}
	path := filepath.Dir(filename)

	// create tmp file with 0o600 permissions
	tmpFile, err := os.CreateTemp(path, ".ccpackage.")
	if err != nil {
		return errors.Wrapf(err, "error creating temp file in directory '%s'", path)
	}
	defer func() {
		// we can ignore this error, note that, os.Remove will
		// throw an error if we have successfully completed
		// os.Rename as the file does not exist anymore.
		_ = os.Remove(tmpFile.Name())
	}()

	// write data into the tmp file
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

	if err := os.Rename(tmpFile.Name(), filename); err != nil {
		return errors.Wrapf(err, "error renaming temp file '%s' to '%s'", tmpFile.Name(), filename)
	}

	return nil
}
