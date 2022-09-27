/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/orion-sdk-go/pkg/bcdb"
	"github.com/hyperledger-labs/orion-sdk-go/pkg/config"
	logger2 "github.com/hyperledger-labs/orion-server/pkg/logger"
	"github.com/hyperledger-labs/orion-server/pkg/types"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

type DataTx struct {
	dataTx bcdb.DataTxContext
}

func (d *DataTx) Put(db string, key string, bytes []byte, a *types.AccessControl) error {
	if err := d.dataTx.Put(db, key, bytes, a); err != nil {
		return errors.Wrapf(err, "failed putting data")
	}
	return nil
}

func (d *DataTx) Get(db string, key string) ([]byte, *types.Metadata, error) {
	r, m, err := d.dataTx.Get(db, key)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed getting data")
	}
	return r, m, nil
}

func (d *DataTx) Commit(sync bool) (string, *types.TxReceiptResponseEnvelope, error) {
	return d.dataTx.Commit(sync)
}

func (d *DataTx) Delete(db string, key string) error {
	return d.dataTx.Delete(db, key)
}

func (d *DataTx) SignAndClose() ([]byte, error) {
	env, err := d.dataTx.SignConstructedTxEnvelopeAndCloseTx()
	if err != nil {
		return nil, errors.Wrapf(err, "failed signing and closing data tx")
	}
	return proto.Marshal(env)
}

func (d *DataTx) AddMustSignUser(userID string) {
	d.dataTx.AddMustSignUser(userID)
}

type LoadedDataTx struct {
	loadedDataTx bcdb.LoadedDataTxContext
	env          *types.DataTxEnvelope
}

func (l *LoadedDataTx) ID() string {
	return l.env.Payload.TxId
}

func (l *LoadedDataTx) Commit() error {
	_, _, err := l.loadedDataTx.Commit(true)
	return err
}

func (l *LoadedDataTx) CoSignAndClose() ([]byte, error) {
	env, err := l.loadedDataTx.CoSignTxEnvelopeAndCloseTx()
	if err != nil {
		return nil, errors.Wrapf(err, "failed co-signing and closing envelope")
	}
	return proto.Marshal(env)
}

func (l *LoadedDataTx) Reads() map[string][]*types.DataRead {
	return l.loadedDataTx.Reads()
}

func (l *LoadedDataTx) Writes() map[string][]*types.DataWrite {
	return l.loadedDataTx.Writes()
}

func (l *LoadedDataTx) MustSignUsers() []string {
	return l.loadedDataTx.MustSignUsers()
}

func (l *LoadedDataTx) SignedUsers() []string {
	return l.loadedDataTx.SignedUsers()
}

type Session struct {
	s bcdb.DBSession
}

func (s *Session) DataTx(txID string) (driver.DataTx, error) {
	var dataTx bcdb.DataTxContext
	var err error
	if len(txID) != 0 {
		dataTx, err = s.s.DataTx(bcdb.WithTxID(txID))
	} else {
		dataTx, err = s.s.DataTx()
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed getting data tc")
	}
	return &DataTx{dataTx: dataTx}, nil
}

func (s *Session) LoadDataTx(env *types.DataTxEnvelope) (driver.LoadedDataTx, error) {
	var dataTx bcdb.LoadedDataTxContext
	var err error
	dataTx, err = s.s.LoadDataTx(env)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting data tc")
	}
	return &LoadedDataTx{
		env:          env,
		loadedDataTx: dataTx,
	}, nil
}

func (s *Session) Ledger() (driver.Ledger, error) {
	l, err := s.s.Ledger()
	if err != nil {
		return nil, errors.Wrap(err, "failed getting ledger")
	}
	return &Ledger{ledger: l}, nil
}

func (s *Session) Query() (driver.Query, error) {
	q, err := s.s.Query()
	if err != nil {
		return nil, errors.Wrap(err, "failed getting query")
	}
	return &Query{query: q}, nil
}

type SessionManager struct {
	config          *config2.Config
	identityManager *IdentityManager
}

func (s *SessionManager) NewSession(id string) (driver.Session, error) {
	logLevel := "info"
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logLevel = "debug"
	}
	c := &logger2.Config{
		Level:         logLevel,
		OutputPath:    []string{"stdout"},
		ErrOutputPath: []string{"stderr"},
		Encoding:      "console",
		Name:          "bcdb-client",
	}
	clientLogger, err := logger2.New(c)
	if err != nil {
		return nil, err
	}

	bcDB, err := bcdb.Create(&config.ConnectionConfig{
		RootCAs: []string{
			s.config.CACert(),
		},
		ReplicaSet: []*config.Replica{
			{
				ID:       s.config.ServerID(),
				Endpoint: s.config.ServerURL(),
			},
		},
		Logger: clientLogger,
	})
	if err != nil {
		return nil, err
	}

	identity := s.identityManager.Identity(id)
	if identity == nil {
		return nil, errors.Errorf("failed getting identity for [%s]", id)
	}

	session, err := bcDB.Session(&config.SessionConfig{
		UserConfig: &config.UserConfig{
			UserID:         id,
			CertPath:       identity.Cert,
			PrivateKeyPath: identity.Key,
		},
		TxTimeout: time.Second * 5,
	})
	return &Session{s: session}, err
}
