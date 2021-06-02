/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package io

import (
	"bufio"
	"io"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/io/streamio"
)

const defaultBufferSize = 65536 // FIXME

type commSCCBufConn struct {
	msgConn    MsgConn
	r          io.Reader
	w          *bufio.Writer
	sessionID  string
	targetPeer string
}

// NewBufConn creates a new connection conn backed by the comm scc
// with a buffered writer to improve efficiency
func NewBufConn(session view.Session) (Conn, error) {
	msgConn, err := NewMsgConn(0, session)
	if err != nil {
		return nil, err
	}
	return &commSCCBufConn{
		msgConn: msgConn,
		r:       streamio.NewReader(msgConn),
		w:       bufio.NewWriterSize(msgConn, defaultBufferSize),
	}, nil
}

func (c *commSCCBufConn) Write(data []byte) (n int, err error) {
	logger.Debugf("commSCCBufConn.Write %v bytes (%v - %v)...\n", len(data), c.targetPeer, c.sessionID)
	return c.w.Write(data)
}

func (c *commSCCBufConn) Read(p []byte) (n int, err error) {
	logger.Debugf("commSCCBufConn.Read %v bytes (%v - %v)...\n", len(p), c.targetPeer, c.sessionID)
	// Flush before read
	c.Flush()
	return c.r.Read(p)
}

func (c *commSCCBufConn) Flush() error {
	logger.Debugf("commSCCBufConn.Flush (%v - %v)...\n", c.targetPeer, c.sessionID)
	err := c.w.Flush()
	if err != nil {
		return err
	}
	return c.msgConn.Flush()
}
