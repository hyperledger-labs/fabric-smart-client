/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"encoding/pem"
	"os"
	"time"

	"github.com/hyperledger-labs/orion-sdk-go/pkg/bcdb"
	"github.com/hyperledger-labs/orion-sdk-go/pkg/config"
	logger2 "github.com/hyperledger-labs/orion-server/pkg/logger"
	"github.com/hyperledger-labs/orion-server/pkg/types"
)

type HelperConfig struct {
	*InitConfig `yaml:"init"`
}

type InitConfig struct {
	ServerUrl           string            `yaml:"serverUrl"`
	CACertPath          string            `yaml:"caCertPath"`
	ServerID            string            `yaml:"serverID"`
	AdminID             string            `yaml:"adminID"`
	AdminCertPath       string            `yaml:"adminCertPath"`
	AdminPrivateKeyPath string            `yaml:"adminPrivateKeyPath"`
	CertPaths           map[string]string `yaml:"certPaths"`
	DBs                 []DB              `yaml:"dbs"`
}

func (p *InitConfig) Init() error {
	logger.Infof("initializing orion server")
	bcDB, err := p.CreateDBInstance()
	if err != nil {
		return err
	}
	logger.Infof("create admin session")
	session, err := p.CreateUserSession(bcDB)
	if err != nil {
		return err
	}
	if err := p.initDBs(session); err != nil {
		return err
	}
	if err := p.initUsers(session); err != nil {
		return err
	}
	return nil
}

func (p *InitConfig) CreateUserSession(bcdb bcdb.BCDB) (bcdb.DBSession, error) {
	return bcdb.Session(&config.SessionConfig{
		UserConfig: &config.UserConfig{
			UserID:         p.AdminID,
			CertPath:       p.AdminCertPath,
			PrivateKeyPath: p.AdminPrivateKeyPath,
		},
		TxTimeout: time.Second * 5,
	})
}

func (p *InitConfig) CreateDBInstance() (bcdb.BCDB, error) {
	c := &logger2.Config{
		Level:         "info",
		OutputPath:    []string{"stdout"},
		ErrOutputPath: []string{"stderr"},
		Encoding:      "console",
		Name:          "bcdb-client",
	}
	clientLogger, err := logger2.New(c)
	if err != nil {
		return nil, err
	}

	return bcdb.Create(&config.ConnectionConfig{
		RootCAs: []string{
			p.CACertPath,
		},
		ReplicaSet: []*config.Replica{
			{
				ID:       p.ServerID,
				Endpoint: p.ServerUrl,
			},
		},
		Logger: clientLogger,
	})
}

func (p *InitConfig) initDBs(session bcdb.DBSession) error {
	tx, err := session.DBsTx()
	if err != nil {
		return err
	}

	logger.Infof("creating databases [%v]", p.DBs)
	for _, db := range p.DBs {
		if err := tx.CreateDB(db.Name, nil); err != nil {
			return err
		}
	}

	txID, txReceipt, err := tx.Commit(true)
	if err != nil {
		return err
	}

	logger.Infof("transaction to create carDB has been submitted, txID = %s, txReceipt = %s", txID, txReceipt.String())
	return nil
}

func (p *InitConfig) initUsers(session bcdb.DBSession) error {
	for _, db := range p.DBs {

		for _, role := range db.Roles {
			usersTx, err := session.UsersTx()
			if err != nil {
				return err
			}

			certPath := p.CertPaths[role]
			certFile, err := os.ReadFile(certPath)
			if err != nil {
				return err
			}
			certBlock, _ := pem.Decode(certFile)
			err = usersTx.PutUser(
				&types.User{
					Id:          role,
					Certificate: certBlock.Bytes,
					Privilege: &types.Privilege{
						DbPermission: map[string]types.Privilege_Access{db.Name: types.Privilege_ReadWrite},
					},
				},
				nil,
				//&types.AccessControl{
				//	ReadWriteUsers: usersMap("admin"),
				//	ReadUsers:      usersMap("admin"),
				//},
			)
			if err != nil {
				usersTx.Abort()
				return err
			}

			txID, receipt, err := usersTx.Commit(true)
			if err != nil {
				return err
			}
			logger.Infof("transaction to provision user record has been committed, user-ID: %s, txID = %s, block = %d, txIdx = %d", role, txID, receipt.GetResponse().GetReceipt().GetHeader().GetBaseHeader().GetNumber(), receipt.GetResponse().GetReceipt().GetTxIndex())
		}
	}
	return nil
}

func usersMap(users ...string) map[string]bool {
	m := make(map[string]bool)
	for _, u := range users {
		m[u] = true
	}
	return m
}
