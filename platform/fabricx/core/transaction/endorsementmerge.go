/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"bytes"
	"sort"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-x-common/api/applicationpb"
)

// mergeProposalResponseEndorsements merges the endorsements carried by multiple
// proposal responses into a single application transaction.
//
// Expected behavior:
//   - every proposal response must carry the same transaction payload
//   - each response may contribute one or more endorsements per namespace
//   - duplicate endorsements are ignored
//   - the final endorsement order is sorted for deterministic output.
//
// The returned transaction is reconstructed from the first proposal response,
// with its Endorsements field replaced by the merged endorsements collected
// from all responses.
func mergeProposalResponseEndorsements(resps []driver.ProposalResponse) (*applicationpb.Tx, error) {
	if len(resps) == 0 {
		return nil, errors.New("no proposal responses provided")
	}

	// Use the first response as the base transaction
	var tx applicationpb.Tx
	if err := proto.Unmarshal(resps[0].Results(), &tx); err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling base tx")
	}
	// rebuild this field from the endorsements carried by all responses.
	tx.Endorsements = nil

	merged := make([]*applicationpb.Endorsements, len(tx.Namespaces))
	seen := make([]map[string]struct{}, len(tx.Namespaces))
	for i := range tx.Namespaces {
		merged[i] = &applicationpb.Endorsements{}
		seen[i] = map[string]struct{}{}
	}

	for i, resp := range resps {
		// Decode the candidate transaction from this response
		var candidate applicationpb.Tx
		if err := proto.Unmarshal(resp.Results(), &candidate); err != nil {
			return nil, errors.Wrapf(err, "failed unmarshalling tx from response %d", i)
		}

		if !sameTxIgnoringEndorsements(&tx, &candidate) {
			return nil, errors.Errorf("proposal response %d carries a different tx payload", i)
		}

		// Decode the per-namespace endorsements carried by this proposal response
		endorsements, err := unmarshalEndorsementsFromProposalResponse(resp.EndorserSignature())
		if err != nil {
			return nil, errors.Wrapf(err, "failed unmarshalling endorsements from response %d", i)
		}

		// There must be exactly one endorsement set per namespace in the tx.
		if len(endorsements) != len(tx.Namespaces) {
			return nil, errors.Errorf(
				"proposal response %d has %d endorsement sets, expected %d",
				i, len(endorsements), len(tx.Namespaces),
			)
		}

		// Merge endorsements namespace by namespace.
		for ns := range endorsements {
			items := endorsements[ns].GetEndorsementsWithIdentity()
			if len(items) == 0 {
				return nil, errors.Errorf("proposal response %d has no endorsements for namespace %d", i, ns)
			}

			for _, item := range items {
				key := endorsementKey(item)
				if _, ok := seen[ns][key]; ok {
					continue
				}
				seen[ns][key] = struct{}{}
				merged[ns].EndorsementsWithIdentity = append(merged[ns].EndorsementsWithIdentity, item)
			}
		}
	}

	// Sort endorsements inside each namespace deterministically so that
	// the final merged transaction is stable across runs and response order
	for _, e := range merged {
		sort.Slice(e.EndorsementsWithIdentity, func(i, j int) bool {
			return bytes.Compare(
				e.EndorsementsWithIdentity[i].GetEndorsement(),
				e.EndorsementsWithIdentity[j].GetEndorsement(),
			) < 0
		})
	}

	tx.Endorsements = merged
	return &tx, nil
}

// sameTxIgnoringEndorsements returns true if the two transactions are identical
// once their Endorsements fields are removed.
//
// This is used to verify that all proposal responses refer to the same
// transaction payload before their endorsements are merged.
func sameTxIgnoringEndorsements(a, b *applicationpb.Tx) bool {
	aCopy := proto.Clone(a).(*applicationpb.Tx)
	bCopy := proto.Clone(b).(*applicationpb.Tx)
	aCopy.Endorsements = nil
	bCopy.Endorsements = nil

	rawA, err := proto.Marshal(aCopy)
	if err != nil {
		return false
	}
	rawB, err := proto.Marshal(bCopy)
	if err != nil {
		return false
	}
	return bytes.Equal(rawA, rawB)
}

// endorsementKey returns a deterministic key used to deduplicate endorsements
// within a namespace during merging.
func endorsementKey(e *applicationpb.EndorsementWithIdentity) string {
	if e == nil {
		return ""
	}
	return string(e.GetEndorsement())
}