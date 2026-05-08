/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package states defines the Asset state model used by the chaincode-to-fsc
// tutorial example.
//
// The struct preserves the exact JSON shape of the asset-transfer-basic
// chaincode at:
//
//	github.com/hyperledger/fabric-samples/blob/main/asset-transfer-basic/chaincode-go/chaincode/smartcontract.go
//
// This is deliberate — a developer migrating from chaincode to FSC views
// should be able to read the same on-chain payload before and after.
//
// FSC's state machinery requires the state object to expose a stable
// world-state key via GetLinearID(). For Asset that key is the chaincode-era
// asset ID.
package states

// Asset describes basic details of what makes up a simple asset.
//
// The field tags and field order are taken verbatim from the asset-transfer-basic
// Go chaincode so that JSON-encoded assets are byte-equivalent across the two
// programming models. This is one of the points the tutorial highlights:
// migrating to FSC-on-Fabric-X does not force a payload re-design.
type Asset struct {
	AppraisedValue int    `json:"AppraisedValue"`
	Color          string `json:"Color"`
	ID             string `json:"ID"`
	Owner          string `json:"Owner"`
	Size           int    `json:"Size"`
}

// GetLinearID returns the asset's world-state key. FSC's state package uses
// this to derive the storage key for tx.AddOutput / tx.AddInputByLinearID.
//
// Migration note: in classical chaincode the world-state key is whatever the
// developer chose to pass to ctx.GetStub().PutState(key, value). Here we make
// the key derivation a method on the state itself so that all writes — from
// every view, from every node — use the same key without copy-pasting.
func (a *Asset) GetLinearID() (string, error) {
	return a.ID, nil
}

// SetLinearID is required by the state package when an existing object is
// re-loaded inside a transaction via tx.AddInputByLinearID. The asset's ID
// is immutable once set, so this is a no-op when the ID is already present.
func (a *Asset) SetLinearID(id string) string {
	if a.ID == "" {
		a.ID = id
	}
	return a.ID
}
