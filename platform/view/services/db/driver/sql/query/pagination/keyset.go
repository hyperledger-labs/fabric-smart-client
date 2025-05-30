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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
)

// PropertyName is the name of the field in the struct that is returned from the database
// V is the type of the field
type PropertyName[V comparable] string

// ExtractField extracts the field from the given value
func (p PropertyName[V]) ExtractField(v any) V {
	return reflect.ValueOf(v).FieldByName(string(p)).Interface().(V)
}

type keyset[I comparable, V any] struct {
	offset    int
	pageSize  int
	sqlIdName common.FieldName
	idGetter  func(V) I
	// the first and last id values in the page
	firstId, lastId I
}

// KeysetWithField creates a keyset pagination where the id has field name idFieldName
func KeysetWithField[I comparable](offset int, pageSize int, sqlIdName common.FieldName, idFieldName PropertyName[I]) (*keyset[I, any], error) {
	if strings.ToUpper(string(idFieldName[0])) != string(idFieldName[0]) {
		return nil, fmt.Errorf("must use exported field")
	}
	return Keyset(offset, pageSize, sqlIdName, idFieldName.ExtractField)
}

type id[I comparable] interface {
	Id() I
}

// KeysetWithId creates a keyset pagination where the result object implements id[I]
func KeysetWithId[I comparable, V id[I]](offset int, pageSize int, sqlIdName common.FieldName) (*keyset[I, V], error) {
	return Keyset[I, V](offset, pageSize, sqlIdName, func(v V) I { return v.Id() })
}

// Keyset creates a keyset pagination
func Keyset[I comparable, V any](offset int, pageSize int, sqlIdName common.FieldName, idGetter func(V) I) (*keyset[I, V], error) {
	if offset < 0 {
		return nil, fmt.Errorf("offset must be greater than zero. Offset: %d", offset)
	}
	if pageSize < 0 {
		return nil, fmt.Errorf("page size must be greater than zero. pageSize: %d", pageSize)
	}
	return &keyset[I, V]{
		offset:    offset,
		pageSize:  pageSize,
		sqlIdName: sqlIdName,
		idGetter:  idGetter,
		firstId:   nilElement[I](),
		lastId:    nilElement[I](),
	}, nil
}

func nilElement[I any]() I {
	var zero I
	switch any(zero).(type) {
	case int:
		return any(-1).(I)
	case string:
		return any("").(I)
	default:
		panic("unsupported type")
	}
}

func (p *keyset[I, V]) nilElement() I {
	return nilElement[I]()
}

func (p *keyset[I, V]) GoToOffset(offset int) (driver.Pagination, error) {
	if offset < 0 {
		return nil, fmt.Errorf("Offset must be greater than zero. pageSize: %d", p.pageSize)
	}
	if offset == p.offset+p.pageSize {
		return &keyset[I, V]{
			offset:    offset,
			pageSize:  p.pageSize,
			sqlIdName: p.sqlIdName,
			idGetter:  p.idGetter,
			firstId:   p.lastId,
			lastId:    p.nilElement(),
		}, nil
	}
	return &keyset[I, V]{
		offset:    offset,
		pageSize:  p.pageSize,
		sqlIdName: p.sqlIdName,
		idGetter:  p.idGetter,
		firstId:   p.nilElement(),
		lastId:    p.nilElement(),
	}, nil
}

func (p *keyset[I, V]) GoToPage(pageNum int) (driver.Pagination, error) {
	return p.GoToOffset(pageNum * p.pageSize)
}

func (p *keyset[I, V]) GoForward(numOfpages int) (driver.Pagination, error) {
	return p.GoToOffset(p.offset + (numOfpages * p.pageSize))
}

func (p *keyset[I, V]) GoBack(numOfpages int) (driver.Pagination, error) {
	return p.GoForward(-1 * numOfpages)
}

func (p *keyset[I, V]) Prev() (driver.Pagination, error) { return p.GoBack(1) }

func (p *keyset[I, V]) Next() (driver.Pagination, error) { return p.GoForward(1) }
