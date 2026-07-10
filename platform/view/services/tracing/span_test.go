/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

func TestNewLabels_Empty(t *testing.T) {
	t.Parallel()

	l := tracing.NewLabels([]string{})
	require.Empty(t, l.ToLabels())
}

func TestNewLabels_WithKeys(t *testing.T) {
	t.Parallel()

	l := tracing.NewLabels([]string{"key1", "key2"})
	labels := l.ToLabels()

	// Each key contributes 2 entries (key + empty value) to the flattened slice.
	require.ElementsMatch(t, []string{"key1", "", "key2", ""}, labels)
}

func TestLabels_Append(t *testing.T) {
	t.Parallel()

	l := tracing.NewLabels([]string{"color"})
	l.Append(attribute.String("color", "blue"))

	require.ElementsMatch(t, []string{"color", "blue"}, l.ToLabels())
}

func TestLabels_Append_InvalidKey_IsNoOp(t *testing.T) {
	t.Parallel()

	l := tracing.NewLabels([]string{"color"})

	before := l.ToLabels()
	l.Append(attribute.String("", "ignored"))
	after := l.ToLabels()

	// An attribute with an empty key must not modify the labels map.
	require.ElementsMatch(t, before, after)
}

func TestLabels_ToLabels_MultipleKeys(t *testing.T) {
	t.Parallel()

	l := tracing.NewLabels([]string{"a", "b", "c"})
	l.Append(
		attribute.String("a", "1"),
		attribute.String("b", "2"),
		attribute.String("c", "3"),
	)

	require.ElementsMatch(t, []string{"a", "1", "b", "2", "c", "3"}, l.ToLabels())
}
