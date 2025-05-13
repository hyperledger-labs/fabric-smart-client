/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

type IsolationLevelMapper interface {
	Map(level driver.IsolationLevel) (sql.IsolationLevel, error)
}
