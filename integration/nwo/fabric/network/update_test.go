/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"testing"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

func TestNextChaincodeCarriesUntouchedFields(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)

	old := &topology.ChannelChaincode{
		Chaincode: topology.Chaincode{
			Name:            "events",
			Label:           "events",
			Version:         "Version-0.0",
			Sequence:        "1",
			Image:           "fsc-cc/events:latest",
			Ctor:            `{"Args":["init"]}`,
			InitRequired:    true,
			SignaturePolicy: "OR ('Org1MSP.member')",
			PackageID:       "events:abc",
			PackageFile:     "/tmp/old.tar.gz",
		},
		Channel: "testchannel",
		Peers:   []string{"org1_peer"},
	}

	next := nextChaincode(old, "Version-1.0",
		topology.WithContainerImage("fsc-cc/events2:latest"))

	require.Equal(t, "Version-1.0", next.Chaincode.Version)
	require.Equal(t, "2", next.Chaincode.Sequence)
	require.Equal(t, "fsc-cc/events2:latest", next.Chaincode.Image)

	require.Empty(t, next.Chaincode.PackageID, "a new package means a new id")
	require.Empty(t, next.Chaincode.PackageFile)

	require.Equal(t, "events", next.Chaincode.Name)
	require.Equal(t, "events", next.Chaincode.Label)
	require.Equal(t, "OR ('Org1MSP.member')", next.Chaincode.SignaturePolicy)
	require.Equal(t, "testchannel", next.Channel)
	require.Equal(t, []string{"org1_peer"}, next.Peers)

	require.Equal(t, "Version-0.0", old.Chaincode.Version, "the original is untouched")
}

// TestNextChaincodeCarriesEverythingWhenNoOptionApplies guards against the
// regression the old hand-rolled UpdateChaincode had: it silently dropped
// Image. ApplyOptions also recomputes InitRequired from Ctor, so a
// regression that dropped Ctor would disable init while every other test
// here (which always touches Image) kept passing.
func TestNextChaincodeCarriesEverythingWhenNoOptionApplies(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)

	old := &topology.ChannelChaincode{
		Chaincode: topology.Chaincode{
			Name:            "iou",
			Label:           "iou-label",
			Version:         "Version-0.0",
			Sequence:        "1",
			Image:           "fsc-cc/iou:latest",
			Ctor:            `{"Args":["init"]}`,
			InitRequired:    true,
			SignaturePolicy: "AND ('Org1MSP.member')",
			PackageID:       "iou:abc",
			PackageFile:     "/tmp/old.tar.gz",
		},
		Channel: "testchannel",
		Peers:   []string{"org1_peer", "org2_peer"},
	}

	next := nextChaincode(old, "Version-1.0")

	require.Equal(t, "Version-1.0", next.Chaincode.Version)
	require.Equal(t, "2", next.Chaincode.Sequence)
	require.Empty(t, next.Chaincode.PackageID)
	require.Empty(t, next.Chaincode.PackageFile)

	require.Equal(t, "fsc-cc/iou:latest", next.Chaincode.Image, "Image must survive")
	require.Equal(t, `{"Args":["init"]}`, next.Chaincode.Ctor, "Ctor must survive")
	require.True(t, next.Chaincode.InitRequired, "InitRequired must survive")
	require.Empty(t, next.Chaincode.Lang, "Lang must survive")
	require.Empty(t, next.Chaincode.Path, "Path must survive")
	require.Equal(t, "AND ('Org1MSP.member')", next.Chaincode.SignaturePolicy,
		"SignaturePolicy must survive")
	require.Equal(t, "iou", next.Chaincode.Name, "Name must survive")
	require.Equal(t, "iou-label", next.Chaincode.Label, "Label must survive")
	require.Equal(t, "testchannel", next.Channel, "Channel must survive")
	require.Equal(t, []string{"org1_peer", "org2_peer"}, next.Peers, "Peers must survive")
}

func TestNextChaincodeCanSwitchToLegacy(t *testing.T) { //nolint:paralleltest
	gomega.RegisterTestingT(t)

	old := &topology.ChannelChaincode{
		Chaincode: topology.Chaincode{
			Name: "ns", Label: "ns", Version: "Version-0.0", Sequence: "3",
			Image: "fsc-cc/base:latest",
		},
	}

	next := nextChaincode(old, "Version-1.0",
		topology.WithLegacyChaincode("github.com/acme/cc"),
		topology.WithPackageFile("/tmp/new.tar.gz"))

	require.Empty(t, next.Chaincode.Image)
	require.Equal(t, "github.com/acme/cc", next.Chaincode.Path)
	require.Equal(t, "golang", next.Chaincode.Lang)
	require.Equal(t, "/tmp/new.tar.gz", next.Chaincode.PackageFile)
	require.Equal(t, "4", next.Chaincode.Sequence)
}
