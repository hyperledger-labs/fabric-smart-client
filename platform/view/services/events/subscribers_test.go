/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package events_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
)

type A struct {
	Name string
}

type B struct {
	Age int
}

func TestSubscribers(t *testing.T) {
	t.Parallel()
	s := events.NewSubscribers()
	a := &A{Name: "a"}
	a2 := &A{Name: "a"}
	b := &B{Age: 1}
	s.Set("0", a, b)
	s.Set("0", a2, b)
	s.Delete("0", a2)
	s.Set("1", b, a)

	b1, ok := s.Get("0", a)
	require.True(t, ok)
	require.Equal(t, b, b1)

	b1, ok = s.Get("0", b)
	require.False(t, ok)
	require.Nil(t, b1)
}
