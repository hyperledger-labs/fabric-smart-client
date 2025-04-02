/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"fmt"
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/pkg/errors"
)

type Cursor string

type EmptyPagination struct {
}

func NewEmptyPagination() *EmptyPagination {
	return &EmptyPagination{}
}

func (p *EmptyPagination) Prev() (driver.Pagination, error) {
	return p, nil
}

func (p *EmptyPagination) Next() (driver.Pagination, error) {
	return &EmptyPagination{}, nil
}

type NoPagination struct {
}

func (p *NoPagination) Prev() driver.Pagination  { return nil }
func (p *NoPagination) Next() driver.Pagination  { return nil }
func (p *NoPagination) Last() driver.Pagination  { return nil }
func (p *NoPagination) First() driver.Pagination { return nil }

type OffsetPagination struct {
	offset   int
	pageSize int
}

func NewOffsetPagination(offset int, pageSize int) (*OffsetPagination, error) {
	if offset < 0 {
		return nil, fmt.Errorf("offset shoud be grater than zero. Offset: %d", offset)
	}
}
func (p *OffsetPagination) GoBack(numOfpages int) driver.Pagination {
	if (p.Offset - p.PageSize) < 0 {
		return nil // TBD will be addressed when we will add order by feature
	}
	return &OffsetPagination{
		Offset:   p.Offset - p.PageSize,
		PageSize: p.PageSize,
	}
	return &OffsetPagination{offset: offset, pageSize: pageSize}, nil
}

func (p *OffsetPagination) GoForward(pages int) driver.Pagination {
	// TBD we need to address the case when we go fowroward more than the table size,
	// this case will be addressed when we will add order by feature
	return &OffsetPagination{
		Offset:   p.Offset + p.PageSize,
		PageSize: p.PageSize,
	}
	return &OffsetPagination{
		offset:   offset,
		pageSize: p.pageSize,
	}, nil
}

/* TBD Last, Prev, and Next will be addressed when we will add order by feature*/
func (p *OffsetPagination) Prev() driver.Pagination  { return p.GoBack(1) }
func (p *OffsetPagination) Next() driver.Pagination  { return p.GoForward(1) }
func (p *OffsetPagination) Last() driver.Pagination  { return nil }
func (p *OffsetPagination) First() driver.Pagination { return p.GoToPage(0) }

func NewPaginationInterpreter() *paginationInterpreter {
	return &paginationInterpreter{}
}

type PaginationInterpreter interface {
	Interpret(p driver.Pagination) (string, error)
}

type paginationInterpreter struct{}

func (i *paginationInterpreter) Interpret(p driver.Pagination) (string, error) {
	switch pagination := p.(type) {
	case *NoPagination:
		return "", nil
	case *OffsetPagination:
		// TBD need to add order by feature
		return fmt.Sprintf("LIMIT %d OFFSET %d", pagination.pageSize, pagination.offset), nil
	case *KeysetPagination:
		// TODO: add OrderBy?
		if (pagination.lastOffset != -1) && (pagination.offset == pagination.lastOffset+pagination.pageSize) {
			return fmt.Sprintf("WHERE %s>%s LIMIT %d", pagination.idFieldName, pagination.lastId, pagination.pageSize), nil
		}
		return fmt.Sprintf("LIMIT %d OFFSET %d", pagination.pageSize, pagination.offset), nil
	case *EmptyPagination:
		return "LIMIT 0 OFFSET 0", nil
	default:
		return "", errors.Errorf("invalid pagination option %+v", pagination)
	}
}

type PaginationUpdater[R comparable] interface {
	Update(recs driver.PageIterator[R]) driver.PageIterator[R]
}

type paginationUpdater[R comparable] struct{}

func NewPaginationUpdater[R comparable]() *paginationUpdater[R] {
	return &paginationUpdater[R]{}
}

func (i *paginationUpdater[R]) Update(recs *driver.PageIterator[*R]) (*driver.PageIterator[*R], error) {
	switch page := recs.Pagination.(type) {
	case *KeysetPagination:
		items := recs.Items
		record, err := collections.ReadLast(items)
		if record == nil || err != nil {
			return nil, nil
		}
		refRec := reflect.ValueOf(*record)
		id := refRec.FieldByName(page.idFieldName)
		page.UpdateId(id.String())
		return (&driver.PageIterator[*R]{Items: recs.Items, Pagination: page}), nil
	default:
		return (&driver.PageIterator[*R]{Items: recs.Items, Pagination: recs.Pagination}), nil
	}
}
