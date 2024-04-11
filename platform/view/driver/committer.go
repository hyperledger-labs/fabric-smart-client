/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

type TransactionFilter interface {
	Accept(txID string, env []byte) (bool, error)
}
