/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package couchdb

import (
	"encoding/json"
	"fmt"

	_ "github.com/go-kivik/couchdb/v3" // The CouchDB driver
	"github.com/go-kivik/kivik/v3"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

const (
	ID  = "_id"
	REV = "_rev"
)

type SyncProcessor struct {
	Client *kivik.Client
}

func (c *SyncProcessor) Process(req fabric.Request, tx fabric.ProcessTransaction, rws *fabric.RWSet, ns string) error {
	if ns != "iou" {
		return nil
	}

	db, err := c.DB(tx, ns)
	if err != nil {
		return errors.WithMessagef(err, "failed to get database")
	}

	// TODO: if the transaction has been already committed, skip it

	// collects writes
	var writes []interface{}

	// TODO: append a write to mark that the transaction has been committed

	// load the revisions of the read dependencies
	var refs []kivik.BulkGetReference
	for i := 0; i < rws.NumReads(ns); i++ {
		k, _, err := rws.GetReadAt(ns, i)
		if err != nil {
			return errors.Wrapf(err, "failed to get read at [%s:%d]", ns, i)
		}
		logger.Debugf("lookup reference for key [%s]", k)
		refs = append(refs, kivik.BulkGetReference{
			ID: k,
		})
	}
	revs := map[string]string{}
	if len(refs) != 0 {
		rows, err := db.BulkGet(context.TODO(), refs)
		if err != nil {
			return errors.Wrapf(err, "failed to get read dependencies")
		}
		for rows.Next() {
			var x map[string]interface{}
			if err := rows.ScanDoc(&x); err != nil {
				return errors.Wrapf(err, "failed to get document from query")
			}
			logger.Debugf("reference for [%s] is [%s]", x[ID], x[REV])
			if x[REV] != nil {
				revs[x[ID].(string)] = x[REV].(string)
			}
		}
	}

	// collect the writes
	for i := 0; i < rws.NumWrites(ns); i++ {
		k, v, err := rws.GetWriteAt(ns, i)
		if err != nil {
			return errors.Wrapf(err, "failed to get write at [%s:%d]", ns, i)
		}
		// TODO: support delete

		logger.Debugf("appending write [%s:%s]...", k, string(v))

		var x map[string]interface{}
		if err := json.Unmarshal(v, &x); err != nil {
			return errors.Wrapf(err, "failed to unmarshal state to map")
		}
		x[ID] = k
		if rev, ok := revs[k]; ok {
			x[REV] = rev
			logger.Debugf("appending write [%s:%s] with rev [%s]...", k, string(v), rev)
		}
		write, err := json.Marshal(x)
		if err != nil {
			return errors.Wrapf(err, "failed to marshall state with id")
		}
		writes = append(writes, write)
	}

	// push the changes
	_, err = db.BulkDocs(context.TODO(), writes)
	if err != nil {
		return errors.Wrapf(err, "failed to store document at [%s:%d]", ns, len(writes))
	}

	logger.Debugf("couchdb sync on [%s:%s:%s]", tx.Network(), tx.Channel(), ns)
	return nil
}

func (c *SyncProcessor) DB(tx fabric.ProcessTransaction, ns string) (*kivik.DB, error) {
	dbName := fmt.Sprintf("%s-%s-%s", tx.Network(), tx.Channel(), ns)
	exists, err := c.Client.DBExists(context.TODO(), dbName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to check existence of database [%s]", dbName)
	}
	if exists {
		return c.Client.DB(context.TODO(), dbName), nil
	}
	if err := c.Client.CreateDB(context.TODO(), dbName); err != nil {
		return nil, errors.Wrapf(err, "failed to create database [%s]", dbName)
	}
	return c.Client.DB(context.TODO(), dbName), nil
}
