/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pagination

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

type offset struct {
	offset   int
	pageSize int
}

// Offset creates a pagination using OFFSET
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
