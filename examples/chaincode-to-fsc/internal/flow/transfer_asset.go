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
		return "", fmt.Errorf("get asset failed: %w", err)
	}

	fmt.Println("validating transfer")
	if asset.Owner == newOwner {
		return "", fmt.Errorf("asset %s is already owned by %s", assetID, newOwner)
	}

	fmt.Println("preparing transaction")
	fmt.Println("endorsing transaction")
	fmt.Println("submitting transaction")
	if err := protocol.SubmitTransfer(assetID, newOwner); err != nil {
		return "", fmt.Errorf("submit transfer failed: %w", err)
	}

	fmt.Println("verifying state")
	updated, err := protocol.GetAsset(assetID)
	if err != nil {
		return "", fmt.Errorf("verify asset failed: %w", err)
	}
	if updated.Owner != newOwner {
		return "", fmt.Errorf("transfer verification failed: expected owner %s, got %s", newOwner, updated.Owner)
	}

	result := fmt.Sprintf("success: asset %s now owned by %s", updated.ID, updated.Owner)
	return result, nil
}