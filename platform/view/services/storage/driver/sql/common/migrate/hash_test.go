/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package migrate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

// Hash changes with the version, the name and any statement, and statements
// split differently do not collide.
func TestStepHash(t *testing.T) {
	t.Parallel()
	base := migrate.Step{Version: 1, Name: "create", SQL: []string{"CREATE TABLE a (id INT)", "CREATE INDEX i ON a (id)"}}
	hashes := map[string]string{}
	for name, s := range map[string]migrate.Step{
		"base":       base,
		"version":    {Version: 2, Name: base.Name, SQL: base.SQL},
		"name":       {Version: 1, Name: "create2", SQL: base.SQL},
		"statement":  {Version: 1, Name: base.Name, SQL: []string{"CREATE TABLE a (id BIGINT)", base.SQL[1]}},
		"joined":     {Version: 1, Name: base.Name, SQL: []string{base.SQL[0] + base.SQL[1]}},
		"resplit":    {Version: 1, Name: base.Name, SQL: []string{"CREATE TABLE a (id INT)C", "REATE INDEX i ON a (id)"}},
		"name+sql":   {Version: 1, Name: "createCREATE TABLE a (id INT)", SQL: base.SQL[1:]},
		"no sql":     {Version: 1, Name: base.Name},
		"empty stmt": {Version: 1, Name: base.Name, SQL: []string{""}},
	} {
		h := s.Hash()
		require.Len(t, h, 64)
		for other, oh := range hashes {
			require.NotEqual(t, oh, h, "%s and %s collide", name, other)
		}
		hashes[name] = h
	}
	require.Equal(t, base.Hash(), migrate.Step{Version: 1, Name: "create", SQL: []string{"CREATE TABLE a (id INT)", "CREATE INDEX i ON a (id)"}}.Hash(), "hash is deterministic")
}
