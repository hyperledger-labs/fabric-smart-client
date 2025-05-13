/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cond_test

import (
	"testing"

	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/cond"
	. "github.com/onsi/gomega"
)

type testCase struct {
	condition      cond.Condition
	expectedQuery  string
	expectedParams []common.Param
}

var testMatrix = []testCase{
	{
		condition:      cond.Cmp(common.NewAliasedTable("tab1").Field("id"), ">", common.NewAliasedTable("tab2").Field("id2")),
		expectedQuery:  "tab1.id > tab2.id2",
		expectedParams: []common.Param{},
	},
	{
		condition:      cond.CmpVal(common.NewAliasedTable("tab").Field("id"), "=", 10),
		expectedQuery:  "tab.id = $0",
		expectedParams: []common.Param{10},
	},
	{
		condition: cond.And(
			cond.Cmp(common.NewAliasedTable("tab1").Field("id"), ">", common.NewAliasedTable("tab2").Field("id2")),
			cond.CmpVal(common.NewAliasedTable("tab").Field("id"), "=", 10),
		),
		expectedQuery:  "(tab1.id > tab2.id2) AND (tab.id = $0)",
		expectedParams: []common.Param{10},
	},
	{
		condition:      cond.InTuple([]common.Serializable{common.NewAliasedTable("tab").Field("id")}, []cond.Tuple{{10}, {20}, {30}}),
		expectedQuery:  "((tab.id = $0)) OR ((tab.id = $1)) OR ((tab.id = $2))",
		expectedParams: []common.Param{10, 20, 30},
	},
	{
		condition:      cond.InTuple([]common.Serializable{common.NewAliasedTable("tab").Field("id"), common.NewAliasedTable("tab").Field("id2")}, []cond.Tuple{{10, "a"}, {20, "b"}, {30, "c"}}),
		expectedQuery:  "((tab.id = $0) AND (tab.id2 = $1)) OR ((tab.id = $2) AND (tab.id2 = $3)) OR ((tab.id = $4) AND (tab.id2 = $5))",
		expectedParams: []common.Param{10, "a", 20, "b", 30, "c"},
	},
}

func TestConditions(t *testing.T) {
	RegisterTestingT(t)

	for _, tc := range testMatrix {
		query, params := common.NewBuilderWithOffset(common2.CopyPtr(0)).
			WriteConditionSerializable(tc.condition, postgres.NewConditionInterpreter()).
			Build()

		Expect(query).To(Equal(tc.expectedQuery))
		Expect(params).To(ConsistOf(tc.expectedParams...))
	}

}
