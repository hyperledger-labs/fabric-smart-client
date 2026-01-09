/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package golang

import (
	"archive/tar"
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

// WriteBytesToPackage writes a file to a tar stream with the contents as provided via the raw parameter.
func WriteBytesToPackage(raw []byte, localpath string, packagepath string, tw *tar.Writer) error {
	fd, err := os.Open(localpath)
	if err != nil {
		return fmt.Errorf("%s: %s", localpath, err)
	}
	defer utils.IgnoreErrorFunc(fd.Close)

	fi, err := fd.Stat()
	if err != nil {
		return fmt.Errorf("%s: %s", localpath, err)
	}

	header, err := tar.FileInfoHeader(fi, localpath)
	if err != nil {
		return fmt.Errorf("failed calculating FileInfoHeader: %s", err)
	}

	// Take the variance out of the tar by using zero time and fixed uid/gid.
	var zeroTime time.Time
	header.AccessTime = zeroTime
	header.ModTime = zeroTime
	header.ChangeTime = zeroTime
	header.Name = packagepath
	header.Mode = 0o100644
	header.Uid = 500
	header.Gid = 500
	header.Uname = ""
	header.Gname = ""
	header.Size = int64(len(raw))

	err = tw.WriteHeader(header)
	if err != nil {
		return fmt.Errorf("failed to write header for %s: %s", localpath, err)
	}

	_, err = io.Copy(tw, bytes.NewBuffer(raw))
	if err != nil {
		return fmt.Errorf("failed to write %s as %s: %s", localpath, packagepath, err)
	}

	return nil
}

// WriteFileToPackage writes a file to a tar stream.
func WriteFileToPackage(localpath string, packagepath string, tw *tar.Writer) error {
	fd, err := os.Open(localpath)
	if err != nil {
		return fmt.Errorf("%s: %s", localpath, err)
	}
	defer fd.Close()

	fi, err := fd.Stat()
	if err != nil {
		return fmt.Errorf("%s: %s", localpath, err)
	}

	header, err := tar.FileInfoHeader(fi, localpath)
	if err != nil {
		return fmt.Errorf("failed calculating FileInfoHeader: %s", err)
	}

	// Take the variance out of the tar by using zero time and fixed uid/gid.
	var zeroTime time.Time
	header.AccessTime = zeroTime
	header.ModTime = zeroTime
	header.ChangeTime = zeroTime
	header.Name = packagepath
	header.Mode = 0o100644
	header.Uid = 500
	header.Gid = 500
	header.Uname = ""
	header.Gname = ""

	err = tw.WriteHeader(header)
	if err != nil {
		return fmt.Errorf("failed to write header for %s: %s", localpath, err)
	}

	is := bufio.NewReader(fd)
	_, err = io.Copy(tw, is)
	if err != nil {
		return fmt.Errorf("failed to write %s as %s: %s", localpath, packagepath, err)
	}

	return nil
}
