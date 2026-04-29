/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pagination_test

import (
	"testing"

	sq "github.com/Masterminds/squirrel"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/pagination"
)

func baseQuery() sq.SelectBuilder {
	return sq.Select("tx_id", "code").From("status")
}

func TestApplyToSquirrel_None(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	q, args, err := pagination.ApplyToSquirrel(pagination.None(), baseQuery()).ToSql()
	Expect(err).ToNot(HaveOccurred())
	Expect(q).To(Equal("SELECT tx_id, code FROM status"))
	Expect(args).To(BeEmpty())
}

func TestApplyToSquirrel_PageSizeOnly(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	p, err := pagination.Offset(0, 10)
	Expect(err).ToNot(HaveOccurred())

	q, args, err := pagination.ApplyToSquirrel(p, baseQuery()).ToSql()
	Expect(err).ToNot(HaveOccurred())
	Expect(q).To(Equal("SELECT tx_id, code FROM status LIMIT 10"))
	Expect(args).To(BeEmpty())
}

func TestApplyToSquirrel_PageSizeAndOffset(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	p, err := pagination.Offset(5, 10)
	Expect(err).ToNot(HaveOccurred())

	q, args, err := pagination.ApplyToSquirrel(p, baseQuery()).ToSql()
	Expect(err).ToNot(HaveOccurred())
	Expect(q).To(Equal("SELECT tx_id, code FROM status LIMIT 10 OFFSET 5"))
	Expect(args).To(BeEmpty())
}

func TestApplyToSquirrel_Nil(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	q, args, err := pagination.ApplyToSquirrel(nil, baseQuery()).ToSql()
	Expect(err).ToNot(HaveOccurred())
	Expect(q).To(Equal("SELECT tx_id, code FROM status"))
	Expect(args).To(BeEmpty())
}
