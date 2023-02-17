/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package streamio

import (
	"testing"
)

type MockMsgReader struct {
	messages [][]byte
}

// read each message and move it from the stack
func newMockMessageReader(messages [][]byte) *MockMsgReader {
	return &MockMsgReader{
		messages: messages,
	}
}

func (m *MockMsgReader) Read() ([]byte, error) {
	var retBuf []byte

	if len(m.messages) > 0 {
		//remove message from buffer
		retBuf = m.messages[0]
		m.messages = m.messages[1:]
		return retBuf, nil
	}

	return nil, nil

}

func TestRead(t *testing.T) {
	var buf []byte

	buf = make([]byte, 7)

	mrr := newMockMessageReader([][]byte{
		[]byte("hello"),
		[]byte("world"),
		[]byte(" my "),
		[]byte(" name "),
		[]byte(" is streamio!"),
	})

	res := []byte("helloworld my  name  is streamio!")

	reader := NewReader(mrr)

	// We are doing 4 reads of at most 7 characters
	for i := 0; i < 4; i++ {
		n, err := reader.Read(buf)

		if err != nil {
			t.Errorf("Expected err to be nil but was %v", err)
		}

		if !(n > 0 && n <= len(buf)) {
			t.Errorf("The returned length should be > 0 and <= %v, but was %v", len(buf), n)
		}

		if string(buf[:n]) != string(res[:n]) {
			t.Errorf("Wrong value read in the buffer. Expecting %v. Was %v", string(buf[:n]), string(res[:n]))
		}

		res = res[n:]
	}
}
