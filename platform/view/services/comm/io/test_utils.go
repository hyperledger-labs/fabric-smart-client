/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package io

import (
	"context"
	"sync"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
)

type networkNode interface {
	NewSessionWithID(sessionID, contextID, endpoint string, pkid []byte, caller view.Identity, msg *view.Message) (view.Session, error)
	Start(ctx context.Context)
	ID() string
}

func SessionTwoParties(t *testing.T, network ...networkNode) {
	ctx := context.Background()
	for _, node := range network {
		node.Start(ctx)
	}

	session01, err := network[0].NewSessionWithID(
		"session_id", "context_id", "", []byte(network[1].ID()), nil, nil)
	assert.NoError(t, err)
	conn01, err := NewConn(0, session01)
	assert.NoError(t, err)

	session10, err := network[1].NewSessionWithID(
		"session_id", "context_id", "", []byte(network[0].ID()), nil, nil)
	assert.NoError(t, err)
	conn10, err := NewConn(1, session10)
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

func receive(t *testing.T, session Conn, waitGroup *sync.WaitGroup, num int, payload []byte) {
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

func send(t *testing.T, session Conn, waitGroup *sync.WaitGroup, num int, payload []byte) {
	for i := 0; i < num; i++ {
		n, err := session.Write(payload)
		assert.NoError(t, err)
		assert.Equal(t, n, len(payload))
	}
	waitGroup.Done()
}
