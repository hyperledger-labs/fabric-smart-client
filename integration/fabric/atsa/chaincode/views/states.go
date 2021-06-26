/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import "encoding/json"

// Asset struct and properties must be exported (start with capitals) to work with contract api metadata
type Asset struct {
	ObjectType        string `json:"objectType"` // ObjectType is used to distinguish different object types in the same chaincode namespace
	ID                string `json:"assetID"`
	OwnerOrg          string `json:"ownerOrg"`
	PublicDescription string `json:"publicDescription"`
}

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

type AssetPrice struct {
	AssetID string `json:"asset_id"`
	TradeID string `json:"trade_id"`
	Price   uint64 `json:"price"`
}

func (ap *AssetPrice) Bytes() ([]byte, error) {
	return json.Marshal(ap)
}
