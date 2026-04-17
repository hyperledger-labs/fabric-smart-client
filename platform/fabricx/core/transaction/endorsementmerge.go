/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"sort"
	"strings"

	"github.com/hyperledger/fabric-x-common/api/applicationpb"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// mergeProposalResponseEndorsements merges the endorsements carried by multiple
// proposal responses into a single application transaction.
//
// Expected behavior:
//   - every proposal response must carry the same transaction payload
//   - each response may contribute one or more endorsements per namespace
//   - duplicate endorsements are ignored by MSP ID
//   - the final endorsement order is sorted by MSP ID for deterministic output.
//
// The returned transaction is reconstructed from the first proposal response,
// with its Endorsements field replaced by the merged endorsements collected
// from all responses.
func mergeProposalResponseEndorsements(resps []driver.ProposalResponse) (*applicationpb.Tx, error) {
	if len(resps) == 0 {
		return nil, errors.New("no proposal responses provided")
	}

	// Use the first response as the base transaction.
	var tx applicationpb.Tx
	if err := proto.Unmarshal(resps[0].Payload(), &tx); err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling base tx")
	}

	// Rebuild endorsements from all responses.
	tx.Endorsements = nil

	merged := make([]*applicationpb.Endorsements, len(tx.Namespaces))
	seen := make([]map[string]struct{}, len(tx.Namespaces))
	for i := range tx.Namespaces {
		merged[i] = &applicationpb.Endorsements{
			EndorsementsWithIdentity: make([]*applicationpb.EndorsementWithIdentity, 0),
		}
		seen[i] = make(map[string]struct{})
	}

	for i, resp := range resps {
		// Decode the candidate transaction from this response
		var candidate applicationpb.Tx
		if err := proto.Unmarshal(resp.Payload(), &candidate); err != nil {
			return nil, errors.Wrapf(err, "failed unmarshalling tx from response %d", i)
		}

		if err := validateTransactionsForMerge(&tx, &candidate, i); err != nil {
			return nil, err
		}

		// Decode the per-namespace endorsements carried by this proposal response
		endorsements, err := unmarshalEndorsementsFromProposalResponse(resp.EndorserSignature())
		if err != nil {
			return nil, errors.Wrapf(err, "failed unmarshalling endorsements from response %d", i)
		}

		if len(endorsements) != len(tx.Namespaces) {
			return nil, errors.Errorf(
				"proposal response %d has %d endorsement sets, expected %d",
				i, len(endorsements), len(tx.Namespaces),
			)
		}

		for nsIdx, nsEndorsements := range endorsements {
			if nsEndorsements == nil || len(nsEndorsements.GetEndorsementsWithIdentity()) == 0 {
				return nil, errors.Errorf("proposal response %d: namespace %d requires at least one endorsement", i, nsIdx)
			}

			for _, e := range nsEndorsements.GetEndorsementsWithIdentity() {
				if e == nil || e.GetIdentity() == nil || e.GetIdentity().GetMspId() == "" {
					return nil, errors.Errorf("proposal response %d: namespace %d endorsement missing MSP ID", i, nsIdx)
				}

				key := endorsementKey(e)
				if _, exists := seen[nsIdx][key]; exists {
					continue
				}

				seen[nsIdx][key] = struct{}{}
				merged[nsIdx].EndorsementsWithIdentity = append(merged[nsIdx].EndorsementsWithIdentity, e)
			}
		}
	}

	tx.Endorsements = merged
	sortEndorsementsByMspID(&tx)

	return &tx, nil
}

func sortEndorsementsByMspID(merged *applicationpb.Tx) {
	for k := range merged.GetEndorsements() {
		// We sort endorsements by the MSP ID of the endorser.
		sort.Slice(merged.Endorsements[k].EndorsementsWithIdentity, func(i, j int) bool {
			return strings.Compare(
				merged.Endorsements[k].EndorsementsWithIdentity[i].GetIdentity().GetMspId(),
				merged.Endorsements[k].EndorsementsWithIdentity[j].GetIdentity().GetMspId(),
			) < 0
		})
	}
}

// validateTransactionsForMerge verifies that the two transactions have identical
// namespace content and can therefore be merged.
func validateTransactionsForMerge(base, candidate *applicationpb.Tx, idx int) error {
	baseNsCount := len(base.GetNamespaces())
	if len(candidate.GetNamespaces()) != baseNsCount {
		return errors.Errorf("proposal response %d: namespace count mismatch", idx)
	}

	for nsIdx := range base.GetNamespaces() {
		if !proto.Equal(base.GetNamespaces()[nsIdx], candidate.GetNamespaces()[nsIdx]) {
			return errors.Errorf("proposal response %d: namespace %d content mismatch", idx, nsIdx)
		}
	}

	return nil
}

// endorsementKey returns the MSP ID used to deduplicate endorsements within a
// namespace during merging.
func endorsementKey(e *applicationpb.EndorsementWithIdentity) string {
	if e == nil || e.GetIdentity() == nil {
		return ""
	}
	return e.GetIdentity().GetMspId()
}
