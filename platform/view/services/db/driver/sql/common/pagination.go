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
}

func (p *NoPagination) Prev() (driver.Pagination, error) {
	return nil, fmt.Errorf("%T has only one page", p)
}
func (p *NoPagination) Next() (driver.Pagination, error) {
	return nil, fmt.Errorf("%T has only one page", p)
}
func (p *NoPagination) First() (driver.Pagination, error) {
	return nil, fmt.Errorf("%T has only one page", p)
}

type OffsetPagination struct {
	Offset   int
	PageSize int
}

func (p *OffsetPagination) GoToPage(pageNum int) (driver.Pagination, error) {
	return &OffsetPagination{
		Offset:   pageNum * p.PageSize,
		PageSize: p.PageSize,
	}, nil
}
func (p *OffsetPagination) GoBack(numOfpages int) (driver.Pagination, error) {
	if (p.Offset - p.PageSize) < 0 {
		return nil, fmt.Errorf("out of pages range")
	}
	return &OffsetPagination{
		Offset:   p.Offset - p.PageSize,
		PageSize: p.PageSize,
	}, nil
}

func (p *OffsetPagination) GoForward(pages int) (driver.Pagination, error) {
	return (&OffsetPagination{
		Offset:   p.Offset + p.PageSize,
		PageSize: p.PageSize,
	}), nil
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
		return "", nil
	case *OffsetPagination:
		return fmt.Sprintf("LIMIT %d OFFSET %d", pagination.PageSize, pagination.Offset), nil
	default:
		return "", errors.Errorf("invalid pagination option %+v", pagination)
	}
}
