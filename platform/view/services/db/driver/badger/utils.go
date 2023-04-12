/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package badger

import (
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	dbproto "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/keys"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

const (
	defaultGCInterval     = 5 * time.Minute // TODO let the user define this via config
	defaultGCDiscardRatio = 0.5             // recommended ratio by badger docs
)

//go:generate counterfeiter -o mock/badger.go -fake-name BadgerDB . badgerDBInterface

// badgerDBInterface exists mainly for testing the auto cleaner
type badgerDBInterface interface {
	IsClosed() bool
	RunValueLogGC(discardRatio float64) error
	Opts() badger.Options
}

// autoCleaner runs badger garbage collection periodically as long as the db is open
func autoCleaner(db badgerDBInterface, badgerGCInterval time.Duration, badgerDiscardRatio float64) context.CancelFunc {
	if db == nil || db.Opts().InMemory {
		// not needed when we run badger in memory mode
		return nil
	}

	ctx, chancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(badgerGCInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if db.IsClosed() {
					// no need to clean anymore
					return
				}
				if err := db.RunValueLogGC(badgerDiscardRatio); err != nil {
					switch err {
					case badger.ErrRejected:
						logger.Warnf("badger: value log garbage collection rejected")
					case badger.ErrNoRewrite:
						// do nothing
					default:
						logger.Warnf("badger: unexpected error while performing value log clean up: %s", err)
					}
				}
				// continue with the next tick to clean up again
			}
		}
	}()

	return chancel
}

func dbKey(namespace, key string) string {
	return namespace + keys.NamespaceSeparator + key
}

func txVersionedValue(txn *badger.Txn, dbKey string) (*dbproto.VersionedValue, error) {
	it, err := txn.Get([]byte(dbKey))
	if err == badger.ErrKeyNotFound {
		return &dbproto.VersionedValue{
			Version: dbproto.V1,
		}, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "could not retrieve item for key %s", dbKey)
	}

	return versionedValue(it, dbKey)
}

func versionedValue(item *badger.Item, dbKey string) (*dbproto.VersionedValue, error) {
	protoValue := &dbproto.VersionedValue{}
	err := item.Value(func(val []byte) error {
		if err := proto.Unmarshal(val, protoValue); err != nil {
			return errors.Wrapf(err, "could not unmarshal VersionedValue for key %s", dbKey)
		}

		if protoValue.Version != dbproto.V1 {
			return errors.Errorf("invalid version, expected %d, got %d", dbproto.V1, protoValue.Version)
		}

		return nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "could not get value for key %s", dbKey)
	}

	return protoValue, nil
}
