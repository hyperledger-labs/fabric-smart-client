/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package io_test

import (
	"sync"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/io"
	"github.com/stretchr/testify/assert"
)

func TestSessionTwoParties(t *testing.T) {
	network, err := comm.NewVirtualNetwork(12345, 2)
	assert.NoError(t, err)
	network.Start()

	session01, err := network[0].Node.NewSessionWithID(
		"session_id", "context_id", "bob", []byte(network[1].ID), nil, nil)
	assert.NoError(t, err)
	conn01, err := io.NewConn(0, session01)
	assert.NoError(t, err)

	session10, err := network[1].Node.NewSessionWithID(
		"session_id", "context_id", "alice", []byte(network[0].ID), nil, nil)
	assert.NoError(t, err)
	conn10, err := io.NewConn(1, session10)
	assert.NoError(t, err)

	num := 10000
	waitGroup := &sync.WaitGroup{}
	waitGroup.Add(4)
	payload1 := []byte("Message in a bottle")
	payload2 := []byte("bottle in a message")

	go send(t, conn01, waitGroup, num, payload1)
	go receive(t, conn10, waitGroup, num, payload1)

	go send(t, conn10, waitGroup, num, payload2)
	go receive(t, conn01, waitGroup, num, payload2)
	waitGroup.Wait()
}

func receive(t *testing.T, session io.Conn, waitGroup *sync.WaitGroup, num int, payload []byte) {
	for i := 0; i < num; i++ {
		msg := make([]byte, len(payload))
		n, err := session.Read(msg)
		assert.NoError(t, err)
		assert.Equal(t, n, len(payload))

		//log.Printf("[%s][%s]\n", msg, payload)
		assert.Equal(t, payload, msg)
	}
	waitGroup.Done()
}

func send(t *testing.T, session io.Conn, waitGroup *sync.WaitGroup, num int, payload []byte) {
	for i := 0; i < num; i++ {
		n, err := session.Write(payload)
		assert.NoError(t, err)
		assert.Equal(t, n, len(payload))
	}
	waitGroup.Done()
}
