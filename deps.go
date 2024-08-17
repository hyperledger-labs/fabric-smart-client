//go:build deps

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deps

import (
	_ "github.com/IBM/idemix/tools/idemixgen"
	_ "github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/statecouchdb"
	_ "github.com/hyperledger/fabric/core/ledger/pvtdatastorage"
	_ "github.com/hyperledger/fabric/orderer/common/server"
)
