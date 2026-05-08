package protocol

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/examples/chaincode-to-fsc/internal/model"
)

var assets = map[string]model.Asset{
	"a1": {
		ID:    "a1",
		Owner: "alice",
	},
}

func GetAsset(id string) (model.Asset, error) {
	asset, ok := assets[id]
	if !ok {
		return model.Asset{}, fmt.Errorf("asset %s not found", id)
	}
	return asset, nil
}

func SubmitTransfer(id, newOwner string) error {
	asset, ok := assets[id]
	if !ok {
		return fmt.Errorf("asset %s not found", id)
	}
	asset.Owner = newOwner
	assets[id] = asset
	return nil
}