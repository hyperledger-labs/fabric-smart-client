/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Namespaces comes from transaction content and Filter can empty it, so At is
// reachable with an unchecked position.
func TestNamespacesAt(t *testing.T) {
	t.Parallel()

	ns := Namespaces{"iou"}
	require.Equal(t, "iou", ns.At(0))
	require.Empty(t, ns.At(1))
	require.Empty(t, ns.At(-1))

	require.Empty(t, Namespaces{}.At(0))
	require.Empty(t, Namespaces(nil).At(0))
	require.Empty(t, ns.Filter(func(string) bool { return false }).At(0))
}
