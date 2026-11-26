/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package auction_test

import (
	"context"
	"time"

	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/examples/auction/states"
	"github.com/hyperledger-labs/fabric-smart-client/examples/auction/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
)

const viewTimeout = 2 * time.Minute

// call runs the view registered as fid on node and returns its raw result.
func call(ii *integration.Infrastructure, node, fid string, in any) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), viewTimeout)
	defer cancel()
	res, err := ii.Client(node).CallViewWithContext(ctx, fid, common.JSONMarshall(in))
	if err != nil {
		return nil, err
	}
	raw, ok := res.([]byte)
	Expect(ok).To(BeTrue(), "unexpected view result type %T", res)
	return raw, nil
}

func Init(ii *integration.Infrastructure) {
	_, err := call(ii, "house", "init", nil)
	Expect(err).NotTo(HaveOccurred())
}

func Create(ii *integration.Infrastructure, item string) string {
	raw, err := call(ii, "house", "create", &views.CreateAuction{Item: item})
	Expect(err).NotTo(HaveOccurred())
	return common.JSONUnmarshalString(raw)
}

func Query(ii *integration.Infrastructure, node, auctionID string) *states.Auction {
	raw, err := call(ii, node, "query", &views.QueryAuction{AuctionID: auctionID})
	Expect(err).NotTo(HaveOccurred())
	a := &states.Auction{}
	common.JSONUnmarshal(raw, a)
	return a
}

func Bid(ii *integration.Infrastructure, bidder, auctionID string, price uint64) (string, error) {
	raw, err := call(ii, bidder, "bid", &views.PlaceBid{AuctionID: auctionID, Price: price, House: ii.Identity("house")})
	if err != nil {
		return "", err
	}
	return common.JSONUnmarshalString(raw), nil
}

func Close(ii *integration.Infrastructure, auctionID string) {
	_, err := call(ii, "house", "close", &views.CloseAuction{AuctionID: auctionID})
	Expect(err).NotTo(HaveOccurred())
}

func Reveal(ii *integration.Infrastructure, auctionID string) *states.Bid {
	raw, err := call(ii, "house", "reveal", &views.Reveal{AuctionID: auctionID})
	Expect(err).NotTo(HaveOccurred())
	winner := &states.Bid{}
	common.JSONUnmarshal(raw, winner)
	return winner
}

func AuditBid(ii *integration.Infrastructure, bidder, auctionID, bidID string) views.Outcome {
	raw, err := call(ii, bidder, "audit", &views.AuditBid{AuctionID: auctionID, BidID: bidID})
	Expect(err).NotTo(HaveOccurred())
	return views.Outcome(common.JSONUnmarshalString(raw))
}
