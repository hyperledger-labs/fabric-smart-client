/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	q "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/cond"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

func NewSignerInfoStore(writeDB WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci common.CondInterpreter) *SignerInfoStore {
	return &SignerInfoStore{
		table:        table,
		errorWrapper: errorWrapper,
		readDB:       readDB,
		writeDB:      writeDB,
		ci:           ci,
	}
}

type SignerInfoStore struct {
	table        string
	errorWrapper driver.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      WriteDB
	ci           common.CondInterpreter
}

func (db *SignerInfoStore) FilterExistingSigners(ids ...view.Identity) ([]view.Identity, error) {
	idHashes := make([]string, len(ids))
	inverseMap := make(map[string]*view.Identity, len(ids))
	for i, id := range ids {
		idHash := id.UniqueID()
		idHashes[i] = idHash
		inverseMap[idHash] = &id
	}

	query, params := q.Select().FieldsByName("id").
		From(q.Table(db.table)).
		Where(cond.In("id", idHashes...)).
		Format(db.ci, nil)
	logger.Debug(query, params)

	rows, err := db.readDB.Query(query, params...)
	if err != nil {
		return nil, errors.Wrapf(err, "error querying db")
	}

	it := NewIterator(rows, func(idHash *string) error { return rows.Scan(idHash) })
	existingSigners := iterators.Map(it, func(h *string) (*view.Identity, error) { return inverseMap[*h], nil })
	return iterators.ReadAllValues(existingSigners)
}

func (db *SignerInfoStore) PutSigner(id view.Identity) error {
	query, params := q.InsertInto(db.table).
		Fields("id").
		Row(id.UniqueID()).
		Format()

	logger.Debug(query, params)
	_, err := db.writeDB.Exec(query, params...)
	if err == nil {
		logger.Debugf("Signer [%s] registered", id)
		return nil
	}
	if errors.Is(db.errorWrapper.WrapError(err), driver.UniqueKeyViolation) {
		logger.Warnf("Signer [%s] already in db. Skipping...", id)
		return nil
	}

	return errors.Wrapf(err, "failed executing query [%s]", query)
}

func (db *SignerInfoStore) CreateSchema() error {
	return InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		id TEXT NOT NULL PRIMARY KEY
	);`, db.table))
}
