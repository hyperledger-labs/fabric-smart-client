/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuilderWithNilServiceProvider(t *testing.T) {
	t.Parallel()

	for name, build := range map[string]func() (*Builder, *Transaction, error){
		"NewBuilder": func() (*Builder, *Transaction, error) {
			b, err := NewBuilder(nil)
			return b, nil, err
		},
		"NewTransactionWith": func() (*Builder, *Transaction, error) {
			return NewTransactionWith(context.Background(), nil, "", "", nil)
		},
		"NewTransactionFromEnvelopeBytes": func() (*Builder, *Transaction, error) {
			return NewTransactionFromEnvelopeBytes(context.Background(), nil, []byte("raw"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			builder, tx, err := build()
			require.EqualError(t, err, "service provider must be set")
			require.Nil(t, builder)
			require.Nil(t, tx)
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
}
