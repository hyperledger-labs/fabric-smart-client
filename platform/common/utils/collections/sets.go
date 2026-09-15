/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package collections

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/sets"

// NewSet creates a [Set] containing the given items, deduplicated. See
// [sets.New].
func NewSet[V comparable](items ...V) sets.Set[V] { return sets.New(items...) }

// Set is an alias for [sets.Set].
type Set[V comparable] sets.Set[V]
