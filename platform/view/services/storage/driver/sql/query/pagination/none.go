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

func (self *empty) Equal(other driver.Pagination) bool {
	_, ok := other.(*empty)
	if !ok {
		return false
	}
	return true
}

func (k *empty) Serialize() ([]byte, error) {
	return []byte{}, nil
}

func EmptyFromRaw(raw []byte) (*empty, error) {
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

func (self *none) Equal(other driver.Pagination) bool {
	_, ok := other.(*none)
	if !ok {
		return false
	}
	return true
}

func (k *none) Serialize() ([]byte, error) {
	return []byte{}, nil
}

func NoneFromRaw(raw []byte) (*none, error) {
	return &none{}, nil
}
