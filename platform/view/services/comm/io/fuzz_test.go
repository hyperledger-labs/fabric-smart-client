/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package io

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// FuzzVarintReaderReadData fuzzes varintReader.ReadData with arbitrary wire bytes.
// This is the framing reader that ingests raw network socket bytes directly before
// authentication or signature verification on inbound peer streams.
func FuzzVarintReaderReadData(f *testing.F) {
	// 1. Valid length-prefixed payload
	msg := []byte("valid message payload for fuzz testing")
	lenBuf := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(lenBuf, uint64(len(msg)))
	validFramed := append(append([]byte{}, lenBuf[:n]...), msg...)
	f.Add(validFramed)

	// 2. Empty payload (len = 0)
	lenBuf0 := make([]byte, binary.MaxVarintLen64)
	n0 := binary.PutUvarint(lenBuf0, 0)
	f.Add(lenBuf0[:n0])

	// 3. Multiple framed messages back-to-back
	var multi []byte
	for _, m := range []string{"alpha", "beta", "gamma"} {
		lBuf := make([]byte, binary.MaxVarintLen64)
		ln := binary.PutUvarint(lBuf, uint64(len(m)))
		multi = append(multi, lBuf[:ln]...)
		multi = append(multi, []byte(m)...)
	}
	f.Add(multi)

	// 4. Oversized claimed length prefix (OOM boundary test)
	hugeBuf := make([]byte, binary.MaxVarintLen64)
	hugeN := binary.PutUvarint(hugeBuf, 1024*1024*1024)
	f.Add(hugeBuf[:hugeN])

	// 5. Max 10-byte varint overflow
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01})

	// 6. Partial varint and truncated payload boundaries
	f.Add([]byte{0x05, 'a', 'b'}) // claims 5 bytes, provides 2
	f.Add([]byte{0x80})           // incomplete varint continuation bit
	f.Add([]byte(nil))
	f.Add([]byte(""))
	f.Add([]byte("not a varint message"))

	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("varintReader.ReadData panicked on input %q: %v", data, r)
			}
		}()

		r := newVarintReader(bytes.NewReader(data), testBufferSize, testMaxMessageSize)
		buf, err := r.ReadData()
		if err == nil && len(buf) > 0 {
			// If first message succeeded, attempt second read to exercise reader state continuation
			_, _ = r.ReadData()
		}
	})
}
