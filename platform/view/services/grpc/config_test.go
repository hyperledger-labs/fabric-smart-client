/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

func TestServerKeepaliveOptions(t *testing.T) {
	t.Parallel()

	kap := keepalive.ServerParameters{
		Time:    4 * time.Second,
		Timeout: 2 * time.Second,
	}
	kep := keepalive.EnforcementPolicy{
		MinTime:             10 * time.Second,
		PermitWithoutStream: true,
	}
	expectedOpts := []grpc.ServerOption{
		grpc.KeepaliveParams(kap),
		grpc.KeepaliveEnforcementPolicy(kep),
	}
	opts := ServerKeepaliveOptions(&ServerKeepAliveConfig{
		Time:    4 * time.Second,
		Timeout: 2 * time.Second,
		EnforcementPolicy: &ServerKeepAliveEnforcementPolicyConfig{
			MinTime:             10 * time.Second,
			PermitWithoutStream: true,
		},
	})
	assert.ObjectsAreEqual(expectedOpts, opts)
}

func TestClientKeepaliveOptions(t *testing.T) {
	t.Parallel()

	kap := keepalive.ClientParameters{
		Time:                1 * time.Second,
		Timeout:             3 * time.Second,
		PermitWithoutStream: true,
	}
	expectedOpts := []grpc.DialOption{grpc.WithKeepaliveParams(kap)}
	opts := ClientKeepaliveOptions(&ClientKeepAliveConfig{
		Time:    1 * time.Second,
		Timeout: 3 * time.Second,
	})
	assert.ObjectsAreEqual(expectedOpts, opts)
}

func TestClientConfigClone(t *testing.T) {
	origin := ClientConfig{
		KeepAliveConfig: &ClientKeepAliveConfig{
			Time: time.Second,
		},
		SecOpts: SecureOptions{
			Key: []byte{1, 2, 3},
		},
		Timeout: time.Second,
	}

	clone := origin.Clone()

	// Same content, different inner fields references.
	assert.Equal(t, origin, clone)

	// We change the contents of the fields and ensure it doesn't
	// propagate across instances.
	origin.KeepAliveConfig.Time = time.Hour
	origin.SecOpts.Certificate = []byte{1, 2, 3}
	origin.SecOpts.Key = []byte{5, 4, 6}
	origin.Timeout = time.Second * 2

	clone.SecOpts.UseTLS = true
	clone.KeepAliveConfig.Time = time.Hour

	expectedOriginState := ClientConfig{
		KeepAliveConfig: &ClientKeepAliveConfig{
			Time: time.Hour,
		},
		SecOpts: SecureOptions{
			Key:         []byte{5, 4, 6},
			Certificate: []byte{1, 2, 3},
		},
		Timeout: time.Second * 2,
	}

	expectedCloneState := ClientConfig{
		KeepAliveConfig: &ClientKeepAliveConfig{
			Time: time.Hour,
		},
		SecOpts: SecureOptions{
			Key:    []byte{1, 2, 3},
			UseTLS: true,
		},
		Timeout: time.Second,
	}

	assert.Equal(t, expectedOriginState, origin)
	assert.Equal(t, expectedCloneState, clone)
}
