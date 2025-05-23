/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pagination

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
)

type Cursor string

type empty struct {
}

func Empty() *empty {
	return &empty{}
}

func (p *empty) Prev() (driver.Pagination, error) {
	return p, nil
}

func (p *empty) Next() (driver.Pagination, error) {
	return &empty{}, nil
}

type none struct {
}

func None() *none {
	return &none{}
}

func (p *none) Prev() (driver.Pagination, error) {
	return Empty(), nil
}

func (p *none) Next() (driver.Pagination, error) {
	return Empty(), nil
}

type offset struct {
	offset   int
	pageSize int
}

func Offset(os int, pageSize int) (*offset, error) {
	if os < 0 {
		return nil, fmt.Errorf("offset shoud be grater than zero. Offset: %d", os)
	}
	if pageSize < 0 {
		return nil, fmt.Errorf("page size shoud be grater than zero. pageSize: %d", pageSize)
	}
	return &offset{offset: os, pageSize: pageSize}, nil
}

func (p *offset) GoToOffset(os int) (driver.Pagination, error) {
	if os < 0 {
		return Empty(), nil
	}
	return &offset{
		offset:   os,
		pageSize: p.pageSize,
	}, nil
}

func (p *offset) GoToPage(pageNum int) (driver.Pagination, error) {
	return p.GoToOffset(pageNum * p.pageSize)
}

func (p *offset) GoForward(numOfpages int) (driver.Pagination, error) {
	return p.GoToOffset(p.offset + (numOfpages * p.pageSize))
}

func (p *offset) GoBack(numOfpages int) (driver.Pagination, error) {
	return p.GoForward(-1 * numOfpages)
}

func (p *offset) Prev() (driver.Pagination, error) { return p.GoBack(1) }
func (p *offset) Next() (driver.Pagination, error) { return p.GoForward(1) }

// PropertyName is the name of the field in the struct that is returned from the database
type PropertyName string

type keyset[I comparable] struct {
	offset      int
	pageSize    int
	sqlIdName   common.FieldName
	idFieldName PropertyName
	// the first and last id values in the page
	firstId, lastId I
}

func Keyset[I comparable](offset int, pageSize int, sqlIdName common.FieldName, idFieldName PropertyName) (*keyset[I], error) {
	if offset < 0 {
		return nil, fmt.Errorf("offset shoud be grater than zero. Offset: %d", offset)
	}
	if pageSize < 0 {
		return nil, fmt.Errorf("page size shoud be grater than zero. pageSize: %d", pageSize)
	}
	if strings.ToUpper(string(idFieldName[0])) != string(idFieldName[0]) {
		return nil, fmt.Errorf("must use exported field")
	}
	return &keyset[I]{
		offset:      offset,
		pageSize:    pageSize,
		sqlIdName:   sqlIdName,
		idFieldName: idFieldName,
	}, nil
}

func (p *keyset[I]) GoToOffset(offset int) (driver.Pagination, error) {
	if offset < 0 {
		return Empty(), nil
	}
	if offset == p.offset+p.pageSize {
		return &keyset[I]{
			offset:      offset,
			pageSize:    p.pageSize,
			sqlIdName:   p.sqlIdName,
			idFieldName: p.idFieldName,
			firstId:     p.lastId,
		}, nil
	}
	return &keyset[I]{
		offset:      offset,
		pageSize:    p.pageSize,
		sqlIdName:   p.sqlIdName,
		idFieldName: p.idFieldName,
	}, nil
}

func (p *keyset[I]) GoToPage(pageNum int) (driver.Pagination, error) {
	return p.GoToOffset(pageNum * p.pageSize)
}

func (p *keyset[I]) GoForward(numOfpages int) (driver.Pagination, error) {
	return p.GoToOffset(p.offset + (numOfpages * p.pageSize))
}

func (p *keyset[I]) GoBack(numOfpages int) (driver.Pagination, error) {
	return p.GoForward(-1 * numOfpages)
}

func (p *keyset[I]) Prev() (driver.Pagination, error) { return p.GoBack(1) }

func (p *keyset[I]) Next() (driver.Pagination, error) { return p.GoForward(1) }

func NewPage[V any](results collections.Iterator[*V], pagination driver.Pagination) (*driver.PageIterator[*V], error) {
	return NewTypedPage[string, V](results, pagination)
}

func NewTypedPage[I comparable, V any](results collections.Iterator[*V], pagination driver.Pagination) (*driver.PageIterator[*V], error) {
	if p, ok := pagination.(*keyset[I]); ok {
		items, err := collections.ReadAllValues(results)
		if err != nil {
			return nil, err
		}
		lastPageItem := reflect.ValueOf(items[len(items)-1]).Elem()
		lastId := lastPageItem.FieldByName(string(p.idFieldName))
		p.lastId = lastId.Interface().(I)
		return &driver.PageIterator[*V]{Items: collections.NewSliceIterator[*V](items), Pagination: p}, nil
	}
	return &driver.PageIterator[*V]{Items: results, Pagination: pagination}, nil
}
