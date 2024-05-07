/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	"modernc.org/sqlite"
)

type postgresErrorMapper struct{}

func (m *postgresErrorMapper) WrapError(err error) error {
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

type sqliteErrorMapper struct{}

func (m *sqliteErrorMapper) WrapError(err error) error {
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

type noErrorMapper struct{}

func (m *noErrorMapper) WrapError(err error) error { return err }
