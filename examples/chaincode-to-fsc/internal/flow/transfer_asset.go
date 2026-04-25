package flow

import (
	"fmt"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/examples/chaincode-to-fsc/internal/protocol"
)

func TransferAsset(assetID, newOwner string) (string, error) {
	if strings.TrimSpace(assetID) == "" {
		return "", fmt.Errorf("asset id cannot be empty")
	}
	if strings.TrimSpace(newOwner) == "" {
		return "", fmt.Errorf("new owner cannot be empty")
	}

	fmt.Println("reading asset")
	asset, err := protocol.GetAsset(assetID)
	if err != nil {
		return "", err
	}

	fmt.Println("validating transfer")
	if asset.Owner == newOwner {
		return "", fmt.Errorf("asset %s is already owned by %s", assetID, newOwner)
	}

	fmt.Println("preparing transaction")
	fmt.Println("endorsing transaction")
	if err := protocol.SubmitTransfer(assetID, newOwner); err != nil {
		return "", err
	}

	fmt.Println("submitting transaction")
	updated, err := protocol.GetAsset(assetID)
	if err != nil {
		return "", err
	}
	if updated.Owner != newOwner {
		return "", fmt.Errorf("transfer verification failed: expected owner %s, got %s", newOwner, updated.Owner)
	}

	result := fmt.Sprintf("success: asset %s now owned by %s", updated.ID, updated.Owner)
	fmt.Println(result)
	return result, nil
}