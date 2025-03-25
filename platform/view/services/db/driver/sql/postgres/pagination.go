/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"

func NewPaginatedInterpreter() common.PaginationInterpreter {
	return common.NewPaginationInterpreter()
}
