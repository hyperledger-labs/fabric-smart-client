/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package _update_test

import (
	"testing"

	. "github.com/onsi/gomega"

	q "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/query"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/query/cond"
)

func TestUpdateSimple(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	query, params := q.Update("my_table").
		Set("address", "newaddress").
		Set("name", "newname").
		Where(cond.Eq("id", 10)).
		Format(newTestInterpreter())

	Expect(query).To(Equal("UPDATE my_table SET address = $1, name = $2 WHERE id = $3"))
	Expect(params).To(ConsistOf("newaddress", "newname", 10))
}
