/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
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
	case *EmptyPagination:
		return "LIMIT 0 OFFSET 0", nil
	default:
		return "", errors.Errorf("invalid pagination option %+v", pagination)
	}
}
