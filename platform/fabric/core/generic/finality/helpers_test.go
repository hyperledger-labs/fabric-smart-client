/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

func newProcessedTransaction(pt driver.ProcessedTransaction) *fabric.ProcessedTransaction {
	fPT := &fabric.ProcessedTransaction{}
	v := reflect.ValueOf(fPT).Elem()
	f := v.FieldByName("pt")
	ptr := reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem()
	ptr.Set(reflect.ValueOf(pt))
	return fPT
}
