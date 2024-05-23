/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/lib/pq"
	"github.com/pkg/errors"
)

type unversionedPersistenceNotifier struct {
	*common.UnversionedPersistence
	*notifier
}

func (db *unversionedPersistenceNotifier) CreateSchema() error {
	if err := db.UnversionedPersistence.CreateSchema(); err != nil {
		return err
	}
	return db.notifier.CreateSchema()
}

type versionedPersistenceNotifier struct {
	*common.VersionedPersistence
	*notifier
}

func (db *versionedPersistenceNotifier) CreateSchema() error {
	if err := db.VersionedPersistence.CreateSchema(); err != nil {
		return err
	}
	return db.notifier.CreateSchema()
}

type notifier struct {
	table            string
	notifyOperations []driver.Operation
	writeDB          *sql.DB
	listener         *pq.Listener
	primaryKeys      []driver.ColumnKey
	listeners        []driver.TriggerCallback
	mutex            sync.RWMutex
}

var operationMap = map[string]driver.Operation{
	"DELETE": driver.Delete,
	"INSERT": driver.Insert,
	"UPDATE": driver.Update,
}

const (
	payloadConcatenator  = "&"
	keySeparator         = "_"
	minReconnectInterval = 10 * time.Second
	maxReconnectInterval = 1 * time.Minute
)

func newNotifier(writeDB *sql.DB, table, dataSource string, notifyOperations []driver.Operation, primaryKeys ...driver.ColumnKey) *notifier {
	d := &notifier{
		writeDB:          writeDB,
		table:            table,
		notifyOperations: notifyOperations,
		listener: pq.NewListener(dataSource, minReconnectInterval, maxReconnectInterval, func(event pq.ListenerEventType, err error) {
			switch event {
			case pq.ListenerEventConnected:
				logger.Infof("Listener connected")
			case pq.ListenerEventDisconnected:
				logger.Infof("Listener disconnected")
			default:
				logger.Warnf("Unexpected event: [%v]: %v", event, err)
			}
		}),
		listeners:   []driver.TriggerCallback{},
		primaryKeys: primaryKeys,
	}
	go d.listenForEvents()
	return d
}

func (db *notifier) listenForEvents() {
	for event := range db.listener.Notify {
		logger.Debugf("New event received on table [%s]: %s", event.Channel, event.Extra)
		db.mutex.RLock()
		for _, cb := range db.listeners {
			if operation, payload, err := db.parsePayload(event.Extra); err != nil {
				logger.Warnf("Unexpected parsing error: %v", err)
			} else {
				cb(operation, payload)
			}
		}
		db.mutex.RUnlock()
	}
}

func (db *notifier) parsePayload(s string) (driver.Operation, map[driver.ColumnKey]string, error) {
	items := strings.Split(s, payloadConcatenator)
	if len(items) != 2 {
		return driver.Unknown, nil, errors.Errorf("malformed payload: length %d instead of 2: %s", len(items), s)
	}
	operation, values := operationMap[items[0]], strings.Split(items[1], keySeparator)
	if operation == driver.Unknown {
		return driver.Unknown, nil, errors.Errorf("malformed operation [%v]: %s", operation, s)
	}
	if len(values) != len(db.primaryKeys) {
		return driver.Unknown, nil, errors.Errorf("expected %d keys, but got %d: %s", len(db.primaryKeys), len(values), s)
	}
	payload := make(map[driver.ColumnKey]string)
	for i, key := range db.primaryKeys {
		value := values[i]
		payload[key] = value
	}
	return operation, payload, nil
}

func (db *notifier) Subscribe(callback driver.TriggerCallback) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	if len(db.listeners) == 0 {
		if err := db.listener.Listen(db.table); err != nil {
			return errors.Wrapf(err, "failed to listen for table %s", db.table)
		}
	}
	db.listeners = append(db.listeners, callback)
	return nil
}

func (db *notifier) UnsubscribeAll() error {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	db.listeners = []driver.TriggerCallback{}
	return db.listener.Unlisten(db.table)
}

func (db *notifier) CreateSchema() error {
	query := fmt.Sprintf(`
	CREATE OR REPLACE FUNCTION %s() RETURNS TRIGGER AS $$
			DECLARE
			row RECORD;
			output TEXT;
			
			BEGIN
			-- Checking the Operation Type
			IF (TG_OP = 'DELETE') THEN
				row = OLD;
			ELSE
				row = NEW;
			END IF;
			
			-- Forming the Output as notification. You can choose you own notification.
			output = TG_OP || '%s' || %s;
			
			-- Calling the pg_notify for my_table_update event with output as payload
	
			PERFORM pg_notify('%s',output);
			
			-- Returning null because it is an after trigger.
			RETURN NULL;
			END;
	$$ LANGUAGE plpgsql;
	
	CREATE OR REPLACE TRIGGER trigger_%s
	AFTER %s ON %s
	FOR EACH ROW EXECUTE PROCEDURE %s();
	`,
		triggerFuncName(db.primaryKeys),
		payloadConcatenator, concatenateIDs(db.primaryKeys),
		db.table,
		db.table,
		convertOperations(db.notifyOperations), db.table,
		triggerFuncName(db.primaryKeys),
	)

	logger.Debug(query)
	if _, err := db.writeDB.Exec(query); err != nil {
		return fmt.Errorf("can't create trigger: %w", err)
	}
	return nil
}

func convertOperations(ops []driver.Operation) string {
	opMap := utils.InverseMap(operationMap)
	opStrings := make([]string, len(ops))
	for i, op := range ops {
		opString, ok := opMap[op]
		if !ok {
			panic("op " + strconv.Itoa(int(op)) + " not found")
		}
		opStrings[i] = opString
	}
	return strings.Join(opStrings, " OR ")
}

func triggerFuncName(keys []string) string {
	return fmt.Sprintf("notify_by_%s", strings.Join(keys, "_"))
}

func concatenateIDs(keys []string) string {
	fields := make([]string, len(keys))
	for i, key := range keys {
		fields[i] = fmt.Sprintf("row.%s", key)
	}
	return strings.Join(fields, fmt.Sprintf(" || '%s' || ", keySeparator))
}
