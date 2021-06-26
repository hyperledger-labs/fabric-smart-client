/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package io

import (
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
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
	logger.Debugf("[commSCCMsgConn] Write [%d][%s]\n", len(data), base64.StdEncoding.EncodeToString(MD5Hash(data)))

	c.writeCounter++
	err = c.session.Send(data)
	if err != nil {
		errMsg := fmt.Errorf("failed sending message to [%s]: [%s]", c.session, err)
		return 0, errMsg
	}

	return len(data), nil
}

func (c *commSCCMsgConn) Read() ([]byte, error) {
	c.readCounter++
	logger.Debugf("[commSCCMsgConn] Reading at counter [%d]", c.readCounter)
	select {
	case msg := <-c.ch:
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}

		if len(msg.Payload) == 0 {
			logger.Error("failed receiving message [%s][%s]", "", "")
			errMsg := fmt.Errorf("failed receiving message [%s][%s]", "", "")
			return nil, errMsg
		}

		logger.Debugf("[commSCCMsgConn] [%d] Read [%d][%s]\n", c.readCounter, len(msg.Payload), base64.StdEncoding.EncodeToString(MD5Hash(msg.Payload)))
		return msg.Payload, nil
	}
}

func (c *commSCCMsgConn) Flush() error {
	return nil
}

func MD5Hash(in []byte) []byte {
	h := md5.New()
	h.Write(in)
	return h.Sum(nil)
}
