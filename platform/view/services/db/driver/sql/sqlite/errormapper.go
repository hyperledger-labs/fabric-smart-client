/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"modernc.org/sqlite"
)

type errorMapper struct{}

func (m *errorMapper) WrapError(err error) error {
	if err, ok := err.(*sqlite.Error); ok {
		switch err.Code() {
		case 1555:
			return errors.Wrapf(driver.UniqueKeyViolation, err.Error())
		default:
			logger.Warnf("Unmapped sqlite error with code [%d]", err.Code())
		}
	}
	return err
}
