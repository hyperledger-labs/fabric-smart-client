/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package io

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"go.uber.org/zap/zapcore"
)

const (
	DefaultTimeout = time.Millisecond * 10 // time.Second * 240
)

type commSCCMsgConn struct {
	index   int
	session view.Session
	ch      <-chan *view.Message
	timeout time.Duration

	writeCounter uint64
	readCounter  uint64
}

func NewMsgConn(index int, session view.Session) (MsgConn, error) {
	conn := &commSCCMsgConn{
		session:      session,
		index:        index,
		ch:           session.Receive(),
		timeout:      DefaultTimeout,
		writeCounter: 0,
		readCounter:  0,
	}

	return conn, nil
}

func (c *commSCCMsgConn) Write(data []byte) (n int, err error) {
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("[commSCCMsgConn] Write [%d][%s]\n", len(data), logging.SHA256Base64(data))
	}

	c.writeCounter++
	err = c.session.Send(data)
	if err != nil {
		errMsg := errors.Errorf("failed sending message to [%s]: [%s]", c.session, err)
		return 0, errMsg
	}

	return len(data), nil
}

func (c *commSCCMsgConn) Read() ([]byte, error) {
	c.readCounter++
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("[commSCCMsgConn] Reading at counter [%d]", c.readCounter)
	}
	msg, ok := <-c.ch
	if !ok {
		return nil, errors.New("channel closed")
	}
	if msg == nil {
		return nil, errors.New("received nil message")
	}
	if msg.Status == view.ERROR {
		return nil, errors.New(string(msg.Payload))
	}

	if len(msg.Payload) == 0 {
		logger.Error("failed receiving message [%s][%s]", "", "")
		errMsg := errors.Errorf("failed receiving message [%s][%s]", "", "")
		return nil, errMsg
	}

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("[commSCCMsgConn] [%d] Read [%d][%s]\n", c.readCounter, len(msg.Payload), logging.SHA256Base64(msg.Payload))
	}
	return msg.Payload, nil
}

func (c *commSCCMsgConn) Flush() error {
	return nil
}
