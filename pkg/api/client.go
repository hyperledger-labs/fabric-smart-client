/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

type ViewClient interface {
	// CallView takes in input a view factory identifier, fid, and an input, in, and invokes the
	// factory f bound to fid on input in. The view returned by the factory is invoked on
	// a freshly created context. This call is blocking until the result is produced or
	// an error is returned.
	CallView(fid string, in []byte) (interface{}, error)

	// Initiate takes in input a view factory identifier, fid, and an input, in, and invokes the
	// factory f bound to fid on input in. The view returned by the factory is invoked on
	// a freshly created context whose identifier, cid, is immediately returned.
	// This call is non-blocking.
	Initiate(fid string, in []byte) (string, error)

	// Track takes in input a context identifier, cid, and returns the latest
	// status of the context as set by the views using it.
	Track(cid string) string

	// IsTxFinal takes in input a transaction id and return nil if the transaction has been committed,
	// an error otherwise.
	IsTxFinal(txid string) error
}
