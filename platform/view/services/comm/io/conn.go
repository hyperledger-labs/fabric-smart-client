/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package io

import (
	"encoding/base64"
	"io"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/io/streamio"
)

type commSCCConn struct {
	msgConn MsgConn
	r       io.Reader
}

// NewConn creates a new connection conn backed by the comm scc
func NewConn(index int, session view.Session) (Conn, error) {
	msgConn, err := NewMsgConn(index, session)
	if err != nil {
		return nil, err
	}

	return &commSCCConn{
		msgConn: msgConn,
		r:       streamio.NewReader(msgConn),
	}, nil
}

type Comm struct {
	Channels map[uint]Conn
	MyPID    int
}

func (c *commSCCConn) Write(data []byte) (n int, err error) {
	logger.Debugf("Write [%d][%s]", len(data), base64.StdEncoding.EncodeToString(MD5Hash(data)))
	defer logger.Debugf("Write [%d][%s] done", len(data), base64.StdEncoding.EncodeToString(MD5Hash(data)))
	return c.msgConn.Write(data)
}

func (c *commSCCConn) Read(p []byte) (n int, err error) {
	logger.Debugf("Reading...")
	n, err = c.r.Read(p)
	logger.Debugf("Read [%d,%d][%s]", n, len(p), base64.StdEncoding.EncodeToString(MD5Hash(p[:n])))
	return
}

func (c *commSCCConn) Flush() error {
	return c.msgConn.Flush()
}
