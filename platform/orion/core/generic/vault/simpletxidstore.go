/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/txidstore"
	odriver "github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

type vcProvider struct{}

func (p *vcProvider) IsValid(code odriver.ValidationCode) bool  { return code == odriver.Valid }
func (p *vcProvider) ToInt32(code odriver.ValidationCode) int32 { return int32(code) }
func (p *vcProvider) FromInt32(code int32) odriver.ValidationCode {
	return odriver.ValidationCode(code)
}
func (p *vcProvider) Unknown() odriver.ValidationCode { return odriver.Unknown }

type SimpleTXIDStore = txidstore.SimpleTXIDStore[odriver.ValidationCode]

func NewSimpleTXIDStore(persistence driver.Persistence) (*SimpleTXIDStore, error) {
	return txidstore.NewSimpleTXIDStore[odriver.ValidationCode](persistence, &vcProvider{})
}
