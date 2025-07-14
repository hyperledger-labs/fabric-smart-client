/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgxlisten"
	"github.com/pkg/errors"
)

var AllOperations = []driver.Operation{driver.Insert, driver.Update, driver.Delete}

type Notifier struct {
	table            string
	notifyOperations []driver.Operation
	writeDB          *sql.DB
	listener         *pgxlisten.Listener
	primaryKeys      []primaryKey
	once             sync.Once
}

var operationMap = map[string]driver.Operation{
	"DELETE": driver.Delete,
	"INSERT": driver.Insert,
	"UPDATE": driver.Update,
}

type primaryKey struct {
	name         driver.ColumnKey
	valueDecoder func(string) (string, error)
}

func NewSimplePrimaryKey(name driver.ColumnKey) *primaryKey {
	return &primaryKey{name: name, valueDecoder: identity}
}

const (
	payloadConcatenator = "&"
	keySeparator        = "_"
	reconnectInterval   = 10 * time.Second
)

func NewNotifier(writeDB *sql.DB, table, dataSource string, notifyOperations []driver.Operation, primaryKeys ...primaryKey) *Notifier {
	return &Notifier{
		writeDB:          writeDB,
		table:            table,
		notifyOperations: notifyOperations,
		listener: &pgxlisten.Listener{
			Connect: func(ctx context.Context) (*pgx.Conn, error) { return pgx.Connect(ctx, dataSource) },
			LogError: func(ctx context.Context, err error) {
				logger.Errorf("error encountered in [%s]: %s", dataSource, err.Error())
			},
			ReconnectDelay: reconnectInterval,
		},
		primaryKeys: primaryKeys,
	}
}

func (db *Notifier) Subscribe(callback driver.TriggerCallback) error {
	db.once.Do(func() {
		logger.Debugf("First subscription for notifier of [%s]. Notifier starts listening...", db.table)
		go func() {
			if err := db.listener.Listen(context.TODO()); err != nil {
				logger.Errorf("notifier listen for [%s] failed: %s", db.table, err.Error())
			}
		}()
	})

	db.listener.Handle(db.table, &notificationHandler{table: db.table, primaryKeys: db.primaryKeys, callback: callback})
	return nil
}

type notificationHandler struct {
	table       string
	primaryKeys []primaryKey
	callback    driver.TriggerCallback
}

func (h *notificationHandler) parsePayload(s string) (driver.Operation, map[driver.ColumnKey]string, error) {
	items := strings.Split(s, payloadConcatenator)
	if len(items) != 2 {
		return driver.Unknown, nil, errors.Errorf("malformed payload: length %d instead of 2: %s", len(items), s)
	}
	operation, values := operationMap[items[0]], strings.Split(items[1], keySeparator)
	if operation == driver.Unknown {
		return driver.Unknown, nil, errors.Errorf("malformed operation [%v]: %s", operation, s)
	}
	if len(values) != len(h.primaryKeys) {
		return driver.Unknown, nil, errors.Errorf("expected %d keys, but got %d: %s", len(h.primaryKeys), len(values), s)
	}
	payload := make(map[driver.ColumnKey]string)
	for i, key := range h.primaryKeys {
		value, err := key.valueDecoder(values[i])
		if err != nil {
			return driver.Unknown, nil, errors.Wrapf(err, "failed to decode value [%s] for key [%s]", values[i], key.name)
		}
		payload[key.name] = value
	}
	return operation, payload, nil
}

func (h *notificationHandler) HandleNotification(ctx context.Context, notification *pgconn.Notification, _ *pgx.Conn) error {
	if notification == nil || len(notification.Payload) == 0 {
		logger.Warnf("nil event received on table [%s], investigate the possible cause", h.table)
		return nil
	}
	logger.DebugfContext(ctx, "new event received on table [%s]: %s", notification.Channel, notification.Payload)
	op, vals, err := h.parsePayload(notification.Payload)
	if err != nil {
		logger.Errorf("failed parsing payload [%s]: %s", notification.Payload, err.Error())
		return errors.Wrapf(err, "failed parsing payload [%s]", notification.Payload)
	}
	h.callback(op, vals)
	return nil
}

func (db *Notifier) UnsubscribeAll() error {
	logger.Debugf("Unsubscribe called")
	return nil
}

func (db *Notifier) GetSchema() string {
	primaryKeys := make([]driver.ColumnKey, len(db.primaryKeys))
	for i, key := range db.primaryKeys {
		primaryKeys[i] = key.name
	}
	funcName := triggerFuncName(primaryKeys)
	lock := utils.MustGet(utils.HashInt64([]byte(funcName)))
	return fmt.Sprintf(`
	SELECT pg_advisory_xact_lock(%d);
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
		lock,
		funcName,
		payloadConcatenator, concatenateIDs(primaryKeys),
		db.table,
		db.table,
		convertOperations(db.notifyOperations), db.table,
		funcName,
	)
}

func (db *Notifier) CreateSchema() error {
	return common.InitSchema(db.writeDB, db.GetSchema())
}

func convertOperations(ops []driver.Operation) string {
	opMap := collections.InverseMap(operationMap)
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
