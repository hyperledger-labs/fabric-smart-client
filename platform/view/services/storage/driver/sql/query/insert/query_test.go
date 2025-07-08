/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package _insert_test

import (
	"testing"

	q "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/common"
	. "github.com/onsi/gomega"
)

func TestInsertSimple(t *testing.T) {
	RegisterTestingT(t)

	query, params := q.InsertInto("my_table").
		Fields("key", "data").
		Row("val1", "val2").
		OnConflictDoNothing().
		Format()

	Expect(query).To(Equal("INSERT INTO my_table " +
		"(key, data) " +
		"VALUES ($1, $2) " +
		"ON CONFLICT DO NOTHING"))
	Expect(params).To(ConsistOf("val1", "val2"))
}

func TestInsertOnConflict(t *testing.T) {
	RegisterTestingT(t)

	query, params := q.InsertInto("my_table").
		Fields("key", "data").
		Row("val1", "val2").
		OnConflict([]common.FieldName{"key", "data"}, q.SetValue("data", "val3"), q.OverwriteValue("key")).
		Format()

	Expect(query).To(Equal("INSERT INTO my_table " +
		"(key, data) " +
		"VALUES ($1, $2) " +
		"ON CONFLICT (key, data) DO UPDATE SET " +
		"data=$3, key=excluded.key"))
	Expect(params).To(ConsistOf("val1", "val2", "val3"))
}
