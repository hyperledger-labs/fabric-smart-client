/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package couchdb

import (
	"fmt"

	_ "github.com/go-kivik/couchdb/v3" // The CouchDB driver
	"github.com/go-kivik/kivik/v3"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

var (
	logger = flogging.MustGetLogger("fabric-sdk.couchdb")
)

type Config struct {
	User     string
	Password string
	Address  string
}

type Manager struct {
	Config Config
	client *kivik.Client
}

func NewManager(config Config) *Manager {
	return &Manager{Config: config}
}

func (m *Manager) Client() (*kivik.Client, error) {
	if m.client != nil {
		return m.client, nil
	}
	var err error
	m.client, err = kivik.New("couch", fmt.Sprintf("http://%s:%s@%s/", m.Config.User, m.Config.Password, m.Config.Address))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to instantiate couchdb client")
	}
	return m.client, nil
}

func (m *Manager) DB(ctx view.ServiceProvider, name string) (*kivik.DB, error) {
	client, err := m.Client()
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to get client")
	}

	fns := fabric.GetDefaultFNS(ctx)
	if fns == nil {
		return nil, errors.Errorf("cannot find default fabric network")
	}

	dbName := fmt.Sprintf("%s-%s-%s", fns.Name(), fns.DefaultChannel(), name)
	exists, err := client.DBExists(context.TODO(), dbName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to check existence of database [%s]", dbName)
	}
	if exists {
		return client.DB(context.TODO(), dbName), nil
	}

	return nil, errors.Errorf("cannof find db [%s]", name)
}

func GetManager(ctx view.ServiceProvider) *Manager {
	managerBoxed, err := ctx.GetService(&Manager{})
	if err != nil {
		panic("cannot find couchdb manager in context")
	}
	manager, ok := managerBoxed.(*Manager)
	if !ok {
		panic("expected couchdb manager")
	}
	return manager
}
