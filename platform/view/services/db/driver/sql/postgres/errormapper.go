/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	errors2 "errors"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/jackc/pgx/v5/pgconn"
)

type errorMapper struct{}

func (m *errorMapper) WrapError(err error) error {
	var pgErr *pgconn.PgError
	if !errors2.As(err, &pgErr) {
		logger.Warnf("Error of type [%T] not pgError", err)
		return err
	}

	switch pgErr.Code {
	case "23505":
		return errors.Wrapf(driver.UniqueKeyViolation, "%s", err)
	case "40P01":
		return errors.Wrapf(driver.DeadlockDetected, "%s", err)
	default:
		logger.Warnf("Unmapped postgres error with code [%s]", pgErr)
		return pgErr
	}
}
