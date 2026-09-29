/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

func TestBuilderWithNilServiceProvider(t *testing.T) {
	t.Parallel()

	for name, newBuilder := range map[string]func(services.Provider) (*Builder, error){
		"NewBuilder":                    NewBuilder,
		"NewBuilderWithServiceProvider": NewBuilderWithServiceProvider,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			builder, err := newBuilder(nil)
			require.EqualError(t, err, "service provider must be set")
			require.Nil(t, builder)
		})
	}

	t.Run("zero value Builder", func(t *testing.T) {
		t.Parallel()
		var b Builder
		for name, build := range map[string]func() (*Transaction, error){
			"NewTransaction":          func() (*Transaction, error) { return b.NewTransaction(context.Background()) },
			"NewTransactionFromBytes": func() (*Transaction, error) { return b.NewTransactionFromBytes([]byte("raw")) },
			"NewTransactionFromEnvelopeBytes": func() (*Transaction, error) {
				return b.NewTransactionFromEnvelopeBytes(context.Background(), []byte("raw"))
			},
			"NewTransactionWithIdentity": func() (*Transaction, error) { return b.NewTransactionWithIdentity(nil) },
		} {
			tx, err := build()
			require.EqualError(t, err, "service provider must be set", name)
			require.Nil(t, tx, name)
		}
	})

	t.Run("NewTransactionWith", func(t *testing.T) {
		t.Parallel()
		builder, tx, err := NewTransactionWith(context.Background(), nil, "", "", nil)
		require.EqualError(t, err, "service provider must be set")
		require.Nil(t, builder)
		require.Nil(t, tx)
	})

	t.Run("NewTransactionFromEnvelopeBytes", func(t *testing.T) {
		t.Parallel()
		builder, tx, err := NewTransactionFromEnvelopeBytes(context.Background(), nil, []byte("raw"))
		require.EqualError(t, err, "service provider must be set")
		require.Nil(t, builder)
		require.Nil(t, tx)
	})
}
