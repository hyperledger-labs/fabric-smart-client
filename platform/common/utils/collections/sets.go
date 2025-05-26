/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package collections

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/sets"

func NewSet[V comparable](items ...V) sets.Set[V] { return sets.New(items...) }

type Set[V comparable] sets.Set[V]
