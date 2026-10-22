/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/configtest"
)

// TestReferenceResolvers checks the fsc.endpoint.resolvers section of docs/configuration.md
// against entry, the type ResolversLoader decodes it into.
func TestReferenceResolvers(t *testing.T) {
	t.Parallel()
	r := configtest.Load(t)
	r.DecodeEach("fsc.endpoint.resolvers", func() any { return &entry{} })
	r.RequireComplete("fsc.endpoint.resolvers")
}
