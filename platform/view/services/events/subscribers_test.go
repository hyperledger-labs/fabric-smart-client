/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package events_test

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/stretchr/testify/assert"
	"testing"
)

type A struct {
	Name string
}

type B struct {
	Age int
}

func TestSubscribers(t *testing.T) {
	s := events.NewSubscribers()
	a := &A{Name: "a"}
	a2 := &A{Name: "a"}
	b := &B{Age: 1}
	s.Store("0", a, b)
	s.Store("0", a2, b)
	s.Delete("0", a2)
	s.Store("1", b, a)

	b1, ok := s.Load("0", a)
	assert.True(t, ok)
	assert.Equal(t, b, b1)

	b1, ok = s.Load("0", b)
	assert.False(t, ok)
	assert.Nil(t, b1)
}
