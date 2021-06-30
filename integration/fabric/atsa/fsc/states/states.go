/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package states

import (
	"encoding/json"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const (
	TypeAsset            = "A"
	TypeAssetForSale     = "S"
	TypeAssetBid         = "B"
	typeAssetSaleReceipt = "SR"
	typeAssetBuyReceipt  = "BR"
)

type AssetProperties struct {
	ObjectType string `json:"objectType"` // ObjectType is used to distinguish different object types in the same chaincode namespace
	ID         string `json:"assetID"`
	Color      string `json:"color"`
	Size       int    `json:"size"`
	Salt       []byte `json:"salt"`
}

func (ap *AssetProperties) Bytes() ([]byte, error) {
	return json.Marshal(ap)
}

type AgreementToSell struct {
	TradeID string        `json:"trade_id"`
	ID      string        `json:"asset_id"`
	Price   int           `json:"price"`
	Owner   view.Identity `json:"owner"`
}

func (a *AgreementToSell) GetLinearID() (string, error) {
	return rwset.CreateCompositeKey(TypeAssetForSale, []string{a.ID})
}

func (a *AgreementToSell) Owners() state.Identities {
	return []view.Identity{a.Owner}
}

type AgreementToBuy struct {
	TradeID string        `json:"trade_id"`
	ID      string        `json:"asset_id"`
	Price   int           `json:"price"`
	Owner   view.Identity `json:"owner"`
}

func (a *AgreementToBuy) GetLinearID() (string, error) {
	return rwset.CreateCompositeKey(TypeAssetBid, []string{a.ID})
}

func (a *AgreementToBuy) Owners() state.Identities {
	return []view.Identity{a.Owner}
}

type Asset struct {
	ObjectType        string        `json:"objectType"`
	ID                string        `json:"assetID"`
	Owner             view.Identity `json:"owner"`
	PublicDescription string        `json:"publicDescription"`
	PrivateProperties []byte        `state:"hash" json:"privateProperties"`
}

func (a *Asset) GetLinearID() (string, error) {
	return rwset.CreateCompositeKey(TypeAsset, []string{a.ID})
}

func (a *Asset) Owners() state.Identities {
	return []view.Identity{a.Owner}
}

type Receipt struct {
	Price     int
	Timestamp time.Time
}
