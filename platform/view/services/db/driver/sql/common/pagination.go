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

// Implements No Pagination
type NoPagination struct {
}

func (p *NoPagination) Prev() driver.Pagination  { return &NoPagination{} /* TBD */ }
func (p *NoPagination) Next() driver.Pagination  { return &NoPagination{} /* TBD */ }
func (p *NoPagination) Last() driver.Pagination  { return &NoPagination{} /* TBD */ }
func (p *NoPagination) First() driver.Pagination { return &NoPagination{} /* TBD */ }

// Implements Offset Pagination
type OffsetPagination struct {
	Offset   int
	PageSize int
}

func (p *OffsetPagination) GoToPage(pageNum int) driver.Pagination {
	return &OffsetPagination{
		Offset:   pageNum * p.PageSize,
		PageSize: p.PageSize,
	}
}
func (p *OffsetPagination) GoBack(numOfpages int) driver.Pagination {
	return &OffsetPagination{
		Offset:   p.Offset - p.PageSize,
		PageSize: p.PageSize,
	}
}

func (p *OffsetPagination) GoForward(pages int) driver.Pagination {
	return &OffsetPagination{
		Offset:   p.Offset + p.PageSize,
		PageSize: p.PageSize,
	}
}

func (p *OffsetPagination) Prev() driver.Pagination  { return p.GoBack(1) }
func (p *OffsetPagination) Next() driver.Pagination  { return p.GoForward(1) }
func (p *OffsetPagination) Last() driver.Pagination  { return nil /* TBD */ }
func (p *OffsetPagination) First() driver.Pagination { return p.GoToPage(0) }

func NewPaginationInterpreter() *paginationInterpreter {
	return &paginationInterpreter{}
}

type PaginationInterpreter interface {
	Interpret(p driver.Pagination) (string, error)
}

// Follow the condiaticondition

type paginationInterpreter struct{}

func (i *paginationInterpreter) Interpret(p driver.Pagination) (string, error) {
	switch pagination := p.(type) {
	case *NoPagination:
		return "", nil
	case *OffsetPagination:
		return fmt.Sprintf(" OFFSET %d LIMIT %d", pagination.Offset, pagination.PageSize), nil
	default:
		return "", errors.Errorf("Invalid pagination option %+v", pagination)
	}
}
