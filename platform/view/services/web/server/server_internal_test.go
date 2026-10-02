/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestServerTimeouts(t *testing.T) {
	t.Parallel()

	s := NewServer(Options{ListenAddress: "127.0.0.1:0"})
	assert.Equal(t, 10*time.Second, s.httpServer.ReadHeaderTimeout)
	assert.Equal(t, 10*time.Second, s.httpServer.ReadTimeout)
	assert.Equal(t, 2*time.Minute, s.httpServer.WriteTimeout)
	assert.Equal(t, 2*time.Minute, s.httpServer.IdleTimeout)
}
