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

var errorMap = map[string]error{
	"23505": driver.UniqueKeyViolation,
	"40P01": driver.DeadlockDetected,
}

type errorMapper struct{}

func (m *errorMapper) WrapError(err error) error {
	var pgErr *pgconn.PgError
	if !errors2.As(err, &pgErr) {
		logger.Warnf("Error of type [%T] not pgError", err)
		return err
	}
	mappedErr, ok := errorMap[pgErr.Code]
	if !ok {
		logger.Warnf("Unmapped postgres error with code [%s]", pgErr.Code)
		return pgErr
	}
	return errors.Wrapf(mappedErr, "%s", err)
}
