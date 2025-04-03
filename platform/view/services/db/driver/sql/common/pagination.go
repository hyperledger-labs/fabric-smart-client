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

func NewNoPagination() *NoPagination {
	return &NoPagination{}
}

func (p *NoPagination) Prev() (driver.Pagination, error) {
	return NewEmptyPagination(), nil
}

func (p *NoPagination) Next() (driver.Pagination, error) {
	return NewEmptyPagination(), nil
}

type OffsetPagination struct {
	offset   int
	pageSize int
}

func NewOffsetPagination(offset int, pageSize int) (*OffsetPagination, error) {
	if offset < 0 {
		return nil, fmt.Errorf("offset shoud be grater than zero. Offset: %d", offset)
	}
	if pageSize < 0 {
		return nil, fmt.Errorf("page size shoud be grater than zero. pageSize: %d", pageSize)
	}
	return &OffsetPagination{offset: offset, pageSize: pageSize}, nil
}

func (p *OffsetPagination) GoToOffset(offset int) (driver.Pagination, error) {
	if offset < 0 {
		return NewEmptyPagination(), nil
	}
	return &OffsetPagination{
		offset:   offset,
		pageSize: p.pageSize,
	}, nil
}

func (p *OffsetPagination) GoToPage(pageNum int) (driver.Pagination, error) {
	return p.GoToOffset(pageNum * p.pageSize)
}

func (p *OffsetPagination) GoForward(numOfpages int) (driver.Pagination, error) {
	return p.GoToOffset(p.offset + (numOfpages * p.pageSize))
}

func (p *OffsetPagination) GoBack(numOfpages int) (driver.Pagination, error) {
	return (p.GoForward(-1 * numOfpages))
}

func (p *OffsetPagination) Prev() (driver.Pagination, error) { return p.GoBack(1) }
func (p *OffsetPagination) Next() (driver.Pagination, error) { return p.GoForward(1) }

type KeysetPagination struct {
	offset   int
	pageSize int
	// name of the field in the database that is a unique id of the records
	sqlIdName string
	// name of the field in the struct that is returned from the database
	idFieldName string
	// the last id value read and the offset in which it was read
	lastId     string // TODO: should this be int?
	lastOffset int
}

func NewKeysetPagination(offset int, pageSize int, sqlIdName string, idFieldName string) (*KeysetPagination, error) {
	if offset < 0 {
		return nil, fmt.Errorf("offset shoud be grater than zero. Offset: %d", offset)
	}
	if pageSize < 0 {
		return nil, fmt.Errorf("page size shoud be grater than zero. pageSize: %d", pageSize)
	}
	return &KeysetPagination{
		offset:      offset,
		pageSize:    pageSize,
		sqlIdName:   sqlIdName,
		idFieldName: idFieldName,
		lastId:      "",
		lastOffset:  -1,
	}, nil
}

func (p *KeysetPagination) GoToOffset(offset int) (driver.Pagination, error) {
	if offset < 0 {
		return NewEmptyPagination(), nil
	}
	return &KeysetPagination{
		offset:      offset,
		pageSize:    p.pageSize,
		sqlIdName:   p.sqlIdName,
		idFieldName: p.idFieldName,
		lastId:      p.lastId,
		lastOffset:  p.lastOffset,
	}, nil
}

func (p *KeysetPagination) GoToPage(pageNum int) (driver.Pagination, error) {
	return p.GoToOffset(pageNum * p.pageSize)
}

func (p *KeysetPagination) GoForward(numOfpages int) (driver.Pagination, error) {
	return p.GoToOffset(p.offset + (numOfpages * p.pageSize))
}

func (p *KeysetPagination) GoBack(numOfpages int) (driver.Pagination, error) {
	return (p.GoForward(-1 * numOfpages))
}

func (p *KeysetPagination) Prev() (driver.Pagination, error) { return p.GoBack(1) }
func (p *KeysetPagination) Next() (driver.Pagination, error) { return p.GoForward(1) }
func (p *KeysetPagination) UpdateId(id string) {
	p.lastId = id
	p.lastOffset = p.offset
}

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
		return fmt.Sprintf("LIMIT %d OFFSET %d", pagination.pageSize, pagination.offset), nil
	case *KeysetPagination:
		// TODO: add OrderBy?
		if (pagination.lastOffset != -1) && (pagination.offset == pagination.lastOffset+pagination.pageSize) {
			return fmt.Sprintf("WHERE %s>'%s' ORDER BY %s ASC LIMIT %d", pagination.sqlIdName, pagination.lastId, pagination.sqlIdName, pagination.pageSize), nil
		}
		return fmt.Sprintf("ORDER BY %s ASC LIMIT %d OFFSET %d", pagination.sqlIdName, pagination.pageSize, pagination.offset), nil
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
		record, newIt, err := collections.ReadLast(items)
		if err != nil {
			return nil, err
		}
		if record != nil {
			refRec := reflect.ValueOf(*record)
			id := refRec.FieldByName(page.idFieldName)
			page.UpdateId(id.String())
		}
		return (&driver.PageIterator[*R]{Items: newIt, Pagination: page}), nil
	default:
		return (&driver.PageIterator[*R]{Items: recs.Items, Pagination: recs.Pagination}), nil
	}
}
