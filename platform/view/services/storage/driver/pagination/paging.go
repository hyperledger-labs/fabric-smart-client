/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pagination

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/internal/storage/sqlbuild"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

// pageable is implemented by every pagination in this package. Dispatching
// through a method rather than a type switch keeps the pagination types
// unexported while covering every instantiation of the generic keyset.
type pageable interface {
	toPaging() sqlbuild.Paging
}

// Paging translates p into the SQL fragment a SELECT needs. A nil pagination
// contributes nothing.
//
// It panics for a [driver.Pagination] implemented outside this package, because
// such a pagination carries no information this package can render.
func Paging(p driver.Pagination) sqlbuild.Paging {
	if p == nil {
		return sqlbuild.Paging{}
	}
	pg, ok := p.(pageable)
	if !ok {
		panic(fmt.Sprintf("invalid pagination option %+v", p))
	}
	return pg.toPaging()
}

func (*none) toPaging() sqlbuild.Paging {
	return sqlbuild.Paging{}
}

func (*empty) toPaging() sqlbuild.Paging {
	// LIMIT 0: an empty pagination returns no rows, rather than every row.
	return sqlbuild.Paging{Limit: new(0)}
}

func (p *offset) toPaging() sqlbuild.Paging {
	return sqlbuild.Paging{Limit: new(p.PageSize), Offset: p.Offset}
}

func (k *keyset[I, V]) toPaging() sqlbuild.Paging {
	col := string(k.SQLIDName)
	pag := sqlbuild.Paging{OrderBy: col, Limit: new(k.PageSize)}
	if k.FirstID != k.zeroElement() {
		pag.Where = sqlbuild.Gt(col, k.FirstID)
	} else {
		pag.Offset = k.Offset
	}
	return pag
}
