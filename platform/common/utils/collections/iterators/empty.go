/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"

func NewEmpty[K any]() *empty[K] { return &empty[K]{zero: utils.Zero[K]()} }

type empty[K any] struct{ zero K }

func (i *empty[K]) HasNext() bool { return false }

func (i *empty[K]) Close() {}

func (i *empty[K]) Next() (K, error) { return i.zero, nil }
