/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mock

import "sync"

// TxStatusQuerier is a hand-written stub for finality.TxStatusQuerier.
type TxStatusQuerier struct {
	lock sync.Mutex
	// GetTransactionStatusesStub, when set, fully controls the return values.
	GetTransactionStatusesStub func(txIDs []string) (map[string]int32, error)
	calls                      [][]string
}

func (m *TxStatusQuerier) GetTransactionStatuses(txIDs []string) (map[string]int32, error) {
	m.lock.Lock()
	m.calls = append(m.calls, txIDs)
	stub := m.GetTransactionStatusesStub
	m.lock.Unlock()

	if stub != nil {
		return stub(txIDs)
	}
	return map[string]int32{}, nil
}

// CallCount returns how many times GetTransactionStatuses was invoked.
func (m *TxStatusQuerier) CallCount() int {
	m.lock.Lock()
	defer m.lock.Unlock()
	return len(m.calls)
}

// ArgsForCall returns the txIDs passed to the i-th invocation.
func (m *TxStatusQuerier) ArgsForCall(i int) []string {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.calls[i]
}
