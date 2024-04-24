/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

// TransactionFilter is used to filter unknown transactions.
// If the filter accepts, the transaction is processed by the commit pipeline anyway.
type TransactionFilter interface {
	Accept(txID string, env []byte) (bool, error)
}
