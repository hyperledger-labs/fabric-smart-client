/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/lib/pq"
	"github.com/pkg/errors"
)

type errorMapper struct{}

func (m *errorMapper) WrapError(err error) error {
	if err, ok := err.(*pq.Error); ok {
		switch err.Code {
		case "23505":
			return errors.Wrapf(driver.UniqueKeyViolation, err.Error())
		case "40P01":
			return errors.Wrapf(driver.DeadlockDetected, err.Error())
		default:
			logger.Warnf("Unmapped postgres error with code [%s]", err.Code)
		}
	}
	return err
}
