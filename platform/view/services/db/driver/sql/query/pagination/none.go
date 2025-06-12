/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pagination

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"

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
