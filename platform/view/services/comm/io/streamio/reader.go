/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package streamio

import (
	"go.uber.org/zap/zapcore"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

var logger = logging.MustGetLogger()

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

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("[Reader] Read [%d][%s]\n", n, logging.SHA256Base64(p[:n]))
	}

	return n, err
}
