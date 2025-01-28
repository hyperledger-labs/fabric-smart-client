/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"

type ValidationCode interface {
	comparable
}

type ValidationCodeProvider[V ValidationCode] interface {
	ToInt32(V) TxStatusCode
	FromInt32(TxStatusCode) V
}

func NewValidationCodeProvider[V ValidationCode](m map[V]TxStatusCode) ValidationCodeProvider[V] {
	return &validationCodeProvider[V]{a: m, b: collections.InverseMap(m)}
}

type validationCodeProvider[V ValidationCode] struct {
	a map[V]TxStatusCode
	b map[TxStatusCode]V
}

func (p *validationCodeProvider[V]) ToInt32(code V) TxStatusCode   { return p.a[code] }
func (p *validationCodeProvider[V]) FromInt32(code TxStatusCode) V { return p.b[code] }
