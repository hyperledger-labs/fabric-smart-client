/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package _update_test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
	q "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/cond"
	. "github.com/onsi/gomega"
)

func TestUpdateSimple(t *testing.T) {
	RegisterTestingT(t)

	query, params := q.Update("my_table").
		Set("address", "newaddress").
		Set("name", "newname").
		Where(cond.Eq("id", 10)).
		Format(postgres.NewConditionInterpreter())

	Expect(query).To(Equal("UPDATE my_table SET address = $1, name = $2 WHERE id = $3"))
	Expect(params).To(ConsistOf("newaddress", "newname", 10))
}
