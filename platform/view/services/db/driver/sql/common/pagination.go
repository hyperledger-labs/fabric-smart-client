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

type NoPagination struct {
	pageInRange bool
}

func NewNoPagination() *NoPagination {
	return &NoPagination{pageInRange: true}
}

func (p *NoPagination) Prev() (driver.Pagination, error) {
	return &NoPagination{
		pageInRange: false,
	}, nil
}

func (p *NoPagination) Next() (driver.Pagination, error) {
	return &NoPagination{
		pageInRange: false,
	}, nil
}
func (p *NoPagination) First() (driver.Pagination, error) {
	return &NoPagination{
		pageInRange: true,
	}, nil
}

type OffsetPagination struct {
	offset      int
	pageSize    int
	pageInRange bool
}

func NewOffsetPagination(offset int, pageSize int) (*OffsetPagination, error) {
	if offset < 0 {
		return nil, fmt.Errorf("offset shoud be grater than zero. Offset: %d", offset)
	}
	if pageSize < 0 {
		return nil, fmt.Errorf("page size shoud be grater than zero. pageSize: %d", pageSize)
	}
	return &OffsetPagination{offset: offset, pageSize: pageSize, pageInRange: true}, nil
}

func (p *OffsetPagination) GoToOffset(offset int) (driver.Pagination, error) {
	if offset < 0 {
		return &OffsetPagination{
			offset:      offset,
			pageSize:    p.pageSize,
			pageInRange: false,
		}, nil
	}
	return &OffsetPagination{
		offset:      offset,
		pageSize:    p.pageSize,
		pageInRange: true,
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

func (p *OffsetPagination) Prev() (driver.Pagination, error)  { return p.GoBack(1) }
func (p *OffsetPagination) Next() (driver.Pagination, error)  { return p.GoForward(1) }
func (p *OffsetPagination) First() (driver.Pagination, error) { return p.GoToPage(0) }

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
		if pagination.pageInRange {
			return "", nil
		} else {
			return "LIMIT 0 OFFSET 0", nil
		}
	case *OffsetPagination:
		if pagination.pageInRange {
			return fmt.Sprintf("LIMIT %d OFFSET %d", pagination.pageSize, pagination.offset), nil
		} else {
			return "LIMIT 0 OFFSET 0", nil
		}
	default:
		return "", errors.Errorf("invalid pagination option %+v", pagination)
	}
}
