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
	if err, ok := err.(*pq.Error); ok && err.Code == "23505" {
		return errors.Wrapf(driver.UniqueKeyViolation, err.Error())
	}
	return err
}

type sqliteErrorMapper struct{}

func (m *sqliteErrorMapper) WrapError(err error) error {
	if err, ok := err.(*sqlite.Error); ok && err.Code() == 1555 {
		return errors.Wrapf(driver.UniqueKeyViolation, err.Error())
	}
	return err
}

type noErrorMapper struct{}

func (m *noErrorMapper) WrapError(err error) error { return err }
