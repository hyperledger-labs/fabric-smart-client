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
	sqlIdName FieldName
	// name of the field in the struct that is returned from the database
	idFieldName string
	// the first and last id values in the page
	firstId string
	lastId  string
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
	}, nil
}

func (p *KeysetPagination) GoToOffset(offset int) (driver.Pagination, error) {
	if offset < 0 {
		return NewEmptyPagination(), nil
	}
	if offset == p.offset+p.pageSize {
		return &KeysetPagination{
			offset:      offset,
			pageSize:    p.pageSize,
			sqlIdName:   p.sqlIdName,
			idFieldName: p.idFieldName,
			firstId:     p.lastId,
		}, nil
	} else {
		return &KeysetPagination{
			offset:      offset,
			pageSize:    p.pageSize,
			sqlIdName:   p.sqlIdName,
			idFieldName: p.idFieldName,
		}, nil
	}
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
}

func NewPaginationInterpreter() *paginationInterpreter {
	return &paginationInterpreter{}
}

type PaginationInterpreter interface {
	Interpret(p driver.Pagination, sql driver.SqlQuery) (driver.SqlQuery, error)
}

type paginationInterpreter struct{}

func (i *paginationInterpreter) Interpret(p driver.Pagination, sql driver.SqlQuery) (driver.SqlQuery, error) {
	switch pagination := p.(type) {
	case *NoPagination:
		return sql, nil
	case *OffsetPagination:
		sql.SetLimit(pagination.pageSize)
		sql.SetOffset(pagination.offset)
		return sql, nil
	case *KeysetPagination:
		sql.SetOrder(fmt.Sprintf("%s ASC", pagination.sqlIdName))
		sql.SetLimit(pagination.pageSize)
		if pagination.firstId != "" {
			lastId := sql.AddParam(pagination.firstId)
			sql.AddWhere(fmt.Sprintf("%s>$%d", pagination.sqlIdName, lastId))
		} else {
			sql.SetOffset(pagination.offset)
		}
		return sql, nil
	case *EmptyPagination:
		sql.SetLimit(0)
		sql.SetOffset(0)
		return sql, nil
	default:
		return sql, errors.Errorf("invalid pagination option %+v", pagination)
	}
}

type PaginationUpdater[R comparable] interface {
	Update(recs driver.PageIterator[R]) driver.PageIterator[R]
}

type paginationUpdater[R comparable] struct{}

func NewPaginationUpdater[R comparable]() *paginationUpdater[R] {
	return &paginationUpdater[R]{}
}

func (i *paginationUpdater[R]) Update(recs driver.PageIterator[*R]) (driver.PageIterator[*R], error) {
	switch page := recs.Pagination.(type) {
	case *KeysetPagination:
		items := recs.Items
		record, newIt, err := collections.ReadLast(items)
		if err != nil {
			return recs, err
		}
		if record != nil {
			refRec := reflect.ValueOf(*record)
			id := refRec.FieldByName(page.idFieldName)
			page.UpdateId(id.String())
		}
		return (driver.PageIterator[*R]{Items: newIt, Pagination: page}), nil
	default:
		return (driver.PageIterator[*R]{Items: recs.Items, Pagination: recs.Pagination}), nil
	}
}
