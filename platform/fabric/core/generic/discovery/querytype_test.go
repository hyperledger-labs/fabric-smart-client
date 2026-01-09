/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"strconv"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/discovery"
	"github.com/stretchr/testify/require"
)

func TestGetQueryType(t *testing.T) {
	tests := []struct {
		q        *discovery.Query
		expected QueryType
	}{
		{q: &discovery.Query{Query: &discovery.Query_PeerQuery{PeerQuery: &discovery.PeerMembershipQuery{}}}, expected: PeerMembershipQueryType},
		{q: &discovery.Query{Query: &discovery.Query_ConfigQuery{ConfigQuery: &discovery.ConfigQuery{}}}, expected: ConfigQueryType},
		{q: &discovery.Query{Query: &discovery.Query_CcQuery{CcQuery: &discovery.ChaincodeQuery{}}}, expected: ChaincodeQueryType},
		{q: &discovery.Query{Query: &discovery.Query_LocalPeers{LocalPeers: &discovery.LocalPeerQuery{}}}, expected: LocalMembershipQueryType},
		{q: &discovery.Query{Query: &discovery.Query_CcQuery{}}, expected: InvalidQueryType},
		{q: nil, expected: InvalidQueryType},
	}

	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			require.Equal(t, tt.expected, GetQueryType(tt.q))
		})
	}
}
