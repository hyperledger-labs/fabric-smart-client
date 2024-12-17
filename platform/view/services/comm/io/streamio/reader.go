/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package streamio

import (
	"crypto/md5"
	"encoding/base64"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

var logger = logging.MustGetLogger("view-sdk.services.comm.io")

// MsgReader wraps a message-based Read function
type MsgReader interface {
	// Returns the next message.
	// If no message available, return nil, nil
	Read() ([]byte, error)
}

// Reader implements a streaming io.Reader from a MessageReader
type Reader struct {
	mr  MsgReader
	buf []byte
}

// NewReader creates a streaming reader from a MessageReader
func NewReader(mr MsgReader) *Reader {
	r := Reader{
		mr: mr,
	}
	return &r
}

// Read implements the read function in io.Reader
func (r *Reader) Read(p []byte) (int, error) {
	var err error

	// If no leftover data in r.buf, read the next message
	if len(r.buf) == 0 {
		r.buf, err = r.mr.Read()

		if r.buf == nil || err != nil {
			r.buf = nil
			return 0, err
		}
	}

	pLen := len(p)
	rBufLen := len(r.buf)

	copy(p, r.buf)
	n := min(pLen, rBufLen)

	if pLen < rBufLen {
		r.buf = r.buf[pLen:]
	} else {
		r.buf = nil
	}

	logger.Debugf("[Reader] Read [%d][%s]\n",
		len(p), base64.StdEncoding.EncodeToString(MD5Hash(p)))

	return n, err
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func MD5Hash(in []byte) []byte {
	h := md5.New()
	h.Write(in)
	return h.Sum(nil)
}
