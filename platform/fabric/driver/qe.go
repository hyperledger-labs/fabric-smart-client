/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"

type QueryExecutor interface {
	GetState(namespace string, key string) ([]byte, error)
	GetStateMetadata(namespace, key string) (map[string][]byte, uint64, uint64, error)
	GetStateRangeScanIterator(namespace string, startKey string, endKey string) (driver.VersionedResultsIterator, error)
	Done()
}
