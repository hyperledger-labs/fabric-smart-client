/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	q "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/cond"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

func NewBindingStore(readDB *sql.DB, writeDB WriteDB, table string, errorWrapper driver.SQLErrorWrapper, ci common.CondInterpreter) *BindingStore {
	return &BindingStore{
		table:        table,
		errorWrapper: errorWrapper,
		readDB:       readDB,
		writeDB:      writeDB,
		ci:           ci,
	}
}

type BindingStore struct {
	table        string
	errorWrapper driver.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      WriteDB
	ci           common.CondInterpreter
}

func (db *BindingStore) GetLongTerm(ctx context.Context, ephemeral view.Identity) (view.Identity, error) {
	query, params := q.Select().FieldsByName("long_term_id").
		From(q.Table(db.table)).
		Where(cond.Eq("ephemeral_hash", ephemeral.UniqueID())).
		Format(db.ci)

	logger.Debug(query, params)
	result, err := QueryUniqueContext[view.Identity](ctx, db.readDB, query, params...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting wallet id for identity [%v]", ephemeral)
	}
	logger.Debugf("found wallet id for identity [%v]: %v", ephemeral, result)
	return result, nil
}

func (db *BindingStore) HaveSameBinding(ctx context.Context, this, that view.Identity) (bool, error) {
	query, params := q.Select().FieldsByName("long_term_id").
		From(q.Table(db.table)).
		Where(cond.In("ephemeral_hash", this.UniqueID(), that.UniqueID())).
		Format(db.ci)

	logger.Debug(query, params)
	rows, err := db.readDB.QueryContext(ctx, query, params...)
	if err != nil {
		return false, errors.Wrapf(err, "error querying db")
	}
	defer utils.IgnoreErrorFunc(rows.Close)

	longTermIds := make([]view.Identity, 0, 2)
	for rows.Next() {
		var longTerm view.Identity
		if err := rows.Scan(&longTerm); err != nil {
			return false, err
		}
		longTermIds = append(longTermIds, longTerm)
	}
	if len(longTermIds) != 2 {
		return false, errors.Errorf("%d entries found instead of 2", len(longTermIds))
	}

	return longTermIds[0].Equal(longTermIds[1]), nil
}

func (db *BindingStore) CreateSchema() error {
	return InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		ephemeral_hash TEXT NOT NULL PRIMARY KEY,
		long_term_id BYTEA NOT NULL
	);`, db.table))
}
