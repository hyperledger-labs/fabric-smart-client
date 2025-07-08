/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cond_test

import (
	"testing"
	"time"

	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/common"
	cond2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/cond"
	. "github.com/onsi/gomega"
)

type testCase struct {
	condition      cond2.Condition
	expectedQuery  string
	expectedParams []common3.Param
}

var testMatrix = []testCase{
	{
		condition:      cond2.And(cond2.AlwaysTrue, cond2.AlwaysTrue),
		expectedQuery:  "1 = 1",
		expectedParams: []common3.Param{},
	},
	{
		condition:      cond2.Or(cond2.AlwaysFalse, cond2.Eq("field", 1)),
		expectedQuery:  "(field = $0)",
		expectedParams: []common3.Param{1},
	},
	{
		condition:      cond2.Cmp(common3.NewTable("tab1").Field("id"), ">", common3.NewTable("tab2").Field("id2")),
		expectedQuery:  "tab1.id > tab2.id2",
		expectedParams: []common3.Param{},
	},
	{
		condition:      cond2.CmpVal(common3.NewTable("tab").Field("id"), "=", 10),
		expectedQuery:  "tab.id = $0",
		expectedParams: []common3.Param{10},
	},
	{
		condition: cond2.And(
			cond2.Cmp(common3.NewTable("tab1").Field("id"), ">", common3.NewTable("tab2").Field("id2")),
			cond2.CmpVal(common3.NewTable("tab").Field("id"), "=", 10),
		),
		expectedQuery:  "(tab1.id > tab2.id2) AND (tab.id = $0)",
		expectedParams: []common3.Param{10},
	},
	{
		condition:      cond2.InTuple([]common3.Serializable{common3.NewTable("tab").Field("id")}, []cond2.Tuple{{10}, {20}, {30}}),
		expectedQuery:  "((tab.id = $0)) OR ((tab.id = $1)) OR ((tab.id = $2))",
		expectedParams: []common3.Param{10, 20, 30},
	},
	{
		condition:      cond2.InTuple([]common3.Serializable{common3.NewTable("tab").Field("id"), common3.NewTable("tab").Field("id2")}, []cond2.Tuple{{10, "a"}, {20, "b"}, {30, "c"}}),
		expectedQuery:  "((tab.id = $0) AND (tab.id2 = $1)) OR ((tab.id = $2) AND (tab.id2 = $3)) OR ((tab.id = $4) AND (tab.id2 = $5))",
		expectedParams: []common3.Param{10, "a", 20, "b", 30, "c"},
	},
	{
		condition:      cond2.OlderThan(common3.FieldName("field"), 5*time.Minute),
		expectedQuery:  "field < NOW() - INTERVAL '300 seconds'",
		expectedParams: []common3.Param{},
	},
	{
		condition:      cond2.AfterNext(common3.FieldName("field"), 10*time.Minute),
		expectedQuery:  "field > NOW() + INTERVAL '600 seconds'",
		expectedParams: []common3.Param{},
	},
	{
		condition:      cond2.InPast(common3.FieldName("field")),
		expectedQuery:  "field < NOW()",
		expectedParams: []common3.Param{},
	},
}

func TestConditions(t *testing.T) {
	RegisterTestingT(t)

	for _, tc := range testMatrix {
		query, params := common3.NewBuilderWithOffset(common2.CopyPtr(0)).
			WriteConditionSerializable(tc.condition, postgres.NewConditionInterpreter()).
			Build()

		Expect(query).To(Equal(tc.expectedQuery))
		Expect(params).To(ConsistOf(tc.expectedParams...))
	}

}
