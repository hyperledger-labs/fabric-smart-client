/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package _delete_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
	q "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/cond"
)

func TestDeleteSimple(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	query, params := q.DeleteFrom("my_table").
		Where(cond.Eq("id", 10)).
		Format(postgres.NewConditionInterpreter())

	Expect(query).To(Equal("DELETE FROM my_table WHERE id = $1"))
	Expect(params).To(ConsistOf(10))
}
