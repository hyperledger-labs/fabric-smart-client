/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txidstore

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/txidstore"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

type vcProvider struct{}

func (p *vcProvider) IsValid(code fdriver.ValidationCode) bool  { return code == fdriver.Valid }
func (p *vcProvider) ToInt32(code fdriver.ValidationCode) int32 { return int32(code) }
func (p *vcProvider) FromInt32(code int32) fdriver.ValidationCode {
	return fdriver.ValidationCode(code)
}
func (p *vcProvider) Unknown() fdriver.ValidationCode { return fdriver.Unknown }

type SimpleTXIDStore = txidstore.SimpleTXIDStore[fdriver.ValidationCode]

func NewTXIDStore(persistence driver.Persistence) (*SimpleTXIDStore, error) {
	return txidstore.NewSimpleTXIDStore[fdriver.ValidationCode](persistence, &vcProvider{})
}
