/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"modernc.org/sqlite"
)

var errorMap = map[int]error{
	1555: driver.UniqueKeyViolation,
	2067: driver.UniqueKeyViolation,
	5:    driver.SqlBusy,
}

type ErrorMapper struct{}

func (m *ErrorMapper) WrapError(err error) error {
	pgErr, ok := err.(*sqlite.Error)
	if !ok {
		return err
	}
	mappedErr, ok := errorMap[pgErr.Code()]
	if !ok {
		logger.Warnf("Unmapped sqlite error with code [%d]", pgErr.Code())
		return pgErr
	}
	return errors.Wrapf(mappedErr, "%s", err)
}
