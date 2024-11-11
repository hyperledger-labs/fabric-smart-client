/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"context"
)

type ListenerManagerProvider[V comparable] interface {
	NewManager() ListenerManager[V]
}

type ListenerManager[V comparable] interface {
	AddListener(txID TxID, toAdd FinalityListener[V]) error
	RemoveListener(txID TxID, toRemove FinalityListener[V])
	InvokeListeners(event FinalityEvent[V])
	TxIDs() []TxID
}

// FinalityEvent contains information about the finality of a given transaction
type FinalityEvent[V comparable] struct {
	Ctx               context.Context
	TxID              TxID
	ValidationCode    V
	ValidationMessage string
	Block             BlockNum
	IndexInBlock      TxNum
	Err               error
	Unknown           bool
}
