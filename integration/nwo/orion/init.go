/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"encoding/pem"
	"io/ioutil"
	"time"

	"github.com/hyperledger-labs/orion-sdk-go/pkg/bcdb"
	"github.com/hyperledger-labs/orion-sdk-go/pkg/config"
	logger2 "github.com/hyperledger-labs/orion-server/pkg/logger"
	"github.com/hyperledger-labs/orion-server/pkg/types"
	. "github.com/onsi/gomega"
)

func (p *Platform) InitOrionServer() {
	logger.Infof("initializing orion server")
	bcDB := p.CreateDBInstance()
	logger.Infof("create admin session")
	session := p.CreateUserSession(bcDB, "admin")
	p.initDBs(session)
	p.initUsers(session)
}

func (p *Platform) CreateUserSession(bcdb bcdb.BCDB, user string) bcdb.DBSession {
	session, err := bcdb.Session(&config.SessionConfig{
		UserConfig: &config.UserConfig{
			UserID:         user,
			CertPath:       p.pem(user),
			PrivateKeyPath: p.key(user),
		},
		TxTimeout: time.Second * 5,
	})
	Expect(err).ToNot(HaveOccurred())
	return session
}

func (p *Platform) CreateDBInstance() bcdb.BCDB {
	c := &logger2.Config{
		Level:         "info",
		OutputPath:    []string{"stdout"},
		ErrOutputPath: []string{"stderr"},
		Encoding:      "console",
		Name:          "bcdb-client",
	}
	clientLogger, err := logger2.New(c)
	Expect(err).ToNot(HaveOccurred())

	bcDB, err := bcdb.Create(&config.ConnectionConfig{
		RootCAs: []string{
			p.caPem(),
		},
		ReplicaSet: []*config.Replica{
			{
				ID:       p.localConfig.Server.Identity.ID,
				Endpoint: p.serverUrl.String(),
			},
		},
		Logger: clientLogger,
	})
	Expect(err).ToNot(HaveOccurred())

	return bcDB
}

func (p *Platform) initDBs(session bcdb.DBSession) {
	tx, err := session.DBsTx()
	Expect(err).ToNot(HaveOccurred())

	logger.Infof("creating databases [%v]", p.Topology.DBs)
	for _, db := range p.Topology.DBs {
		err = tx.CreateDB(db.Name, nil)
		Expect(err).ToNot(HaveOccurred())
	}

	txID, txReceipt, err := tx.Commit(true)
	Expect(err).ToNot(HaveOccurred())

	logger.Infof("transaction to create carDB has been submitted, txID = %s, txReceipt = %s", txID, txReceipt.String())
}

func (p *Platform) initUsers(session bcdb.DBSession) {
	for _, db := range p.Topology.DBs {

		for _, role := range db.Roles {
			usersTx, err := session.UsersTx()
			Expect(err).ToNot(HaveOccurred())

			certPath := p.pem(role)
			certFile, err := ioutil.ReadFile(certPath)
			Expect(err).ToNot(HaveOccurred())
			certBlock, _ := pem.Decode(certFile)
			err = usersTx.PutUser(
				&types.User{
					Id:          role,
					Certificate: certBlock.Bytes,
					Privilege: &types.Privilege{
						DbPermission: map[string]types.Privilege_Access{db.Name: 1},
					},
				}, &types.AccessControl{
					ReadWriteUsers: usersMap("admin"),
					ReadUsers:      usersMap("admin"),
				})
			if err != nil {
				usersTx.Abort()
				Expect(err).ToNot(HaveOccurred())
			}

			txID, receipt, err := usersTx.Commit(true)
			Expect(err).ToNot(HaveOccurred())
			logger.Infof("transaction to provision user record has been committed, user-ID: %s, txID = %s, block = %d, txIdx = %d", role, txID, receipt.GetResponse().GetReceipt().GetHeader().GetBaseHeader().GetNumber(), receipt.GetResponse().GetReceipt().GetTxIndex())
		}
	}
}

func usersMap(users ...string) map[string]bool {
	m := make(map[string]bool)
	for _, u := range users {
		m[u] = true
	}
	return m
}
