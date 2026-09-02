/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

func TestFieldNameValidate(t *testing.T) {
	t.Parallel()

	for _, name := range []driver.FieldName{"pos", "tx_id", "col_id", "_x", "a1"} {
		require.NoError(t, driver.FieldName(name).Validate(), name)
	}
}

// A column name reaches the statement verbatim, so anything that is not a plain
// identifier is either a bug or an injection attempt.
func TestFieldNameValidateRejectsNonIdentifiers(t *testing.T) {
	t.Parallel()

	for _, name := range []driver.FieldName{
		"",
		"1pos",
		"tx id",
		"tx_id, code",
		"tx_id ASC",
		"tx_id ASC; DROP TABLE status--",
		`"tx_id"`,
		"t.tx_id",
		"tx_id)",
	} {
		require.Error(t, name.Validate(), name)
	}
}
