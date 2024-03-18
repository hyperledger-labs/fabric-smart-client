/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"bytes"
	"encoding/binary"
	"strconv"

	"github.com/pkg/errors"
)

type bufferedReader struct {
	bytes  []byte
	length int
}

func newBufferedReader() *bufferedReader {
	return &bufferedReader{
		bytes: []byte{},
	}
}

func (r *bufferedReader) Read(p []byte) (int, error) {
	r.bytes = append(r.bytes, p...)

	if r.length == 0 {
		if length, err := readLength(p); err != nil {
			return 0, err
		} else {
			r.length = length
		}
	}
	if len(r.bytes) > r.length {
		return 0, errors.New("too many elements added [" + strconv.Itoa(len(r.bytes)) + "/" + strconv.Itoa(r.length) + "]")
	}
	return len(p), nil
}

func readLength(p []byte) (int, error) {
	length, err := binary.ReadUvarint(bytes.NewReader(p))
	if err != nil {
		return 0, errors.Wrapf(err, "failed to read length ["+string(p)+"]")
	}
	logger.Debugf("Reading only size: %d + %d", length, len(p))
	return int(length) + len(p), nil
}

func (r *bufferedReader) Flush() []byte {
	if len(r.bytes) < r.length {
		return nil
	}
	res := bytes.Clone(r.bytes)
	r.bytes = []byte{}
	r.length = 0
	return res
}
