/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package io

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const (
	DefaultTimeout = time.Millisecond * 10 // time.Second * 240
)

type commSCCMsgConn struct {
	ctx     context.Context
	index   int
	session view.Session
	ch      <-chan *view.Message
	timeout time.Duration

	writeCounter uint64
	readCounter  uint64
}

// NewMsgConn returns a MsgConn that writes each message with session.Send and reads the
// payloads of the messages session receives. A nil ctx is replaced by context.Background.
func NewMsgConn(ctx context.Context, index int, session view.Session) (MsgConn, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	conn := &commSCCMsgConn{
		ctx:          ctx,
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
	if logger.IsEnabledFor(logging.DebugLevel) {
		logger.Debugf("[commSCCMsgConn] Write [%d][%s]", len(data), logging.SHA256Base64(data))
	}

	c.writeCounter++
	if err := c.session.Send(c.ctx, data); err != nil {
		// Info copies the fields under the session's lock; formatting the session itself would
		// read them while the node updates them.
		return 0, errors.Wrapf(err, "failed sending message on session [%s]", c.session.Info().ID)
	}

	return len(data), nil
}

func (c *commSCCMsgConn) Read() ([]byte, error) {
	c.readCounter++
	if logger.IsEnabledFor(logging.DebugLevel) {
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
		return nil, errors.New("received empty payload")
	}

	if logger.IsEnabledFor(logging.DebugLevel) {
		logger.Debugf("[commSCCMsgConn] [%d] Read [%d][%s]", c.readCounter, len(msg.Payload), logging.SHA256Base64(msg.Payload))
	}
	return msg.Payload, nil
}

func (*commSCCMsgConn) Flush() error {
	return nil
}
