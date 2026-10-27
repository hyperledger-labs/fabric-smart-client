/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"testing"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

// persistenceSections is the part of the rendered core.yaml that selects and configures persistences.
type persistenceSections struct {
	FSC struct {
		Persistences map[string]struct {
			Type string         `yaml:"type"`
			Opts map[string]any `yaml:"opts"`
		} `yaml:"persistences"`
		KVS    map[string]any `yaml:"kvs"`
		Stores map[string]any `yaml:",inline"`
	} `yaml:"fsc"`
}

func renderPersistences(t *testing.T, opts ...node.Option) persistenceSections {
	t.Helper()
	var cfg persistenceSections
	require.NoError(t, yaml.Unmarshal([]byte(renderCoreTemplate(t, node.CoreTemplate, NewTopology(), opts...)), &cfg))
	return cfg
}

func putPostgres(name driver.PersistenceName, sql node.SQLOpts) node.Option {
	return func(o *node.Options) error {
		o.PutPostgresPersistence(name, sql)
		return nil
	}
}

const dataSource = "host=localhost port=5432 user=u password=p dbname=db sslmode=disable"

func TestCoreTemplate_Persistences(t *testing.T) {
	t.Parallel()

	t.Run("defaults", func(t *testing.T) {
		t.Parallel()
		cfg := renderPersistences(t, putPostgres("default", node.SQLOpts{DataSource: dataSource}))

		require.Equal(t, "postgres", cfg.FSC.Persistences["default"].Type)
		require.Nil(t, cfg.FSC.Persistences["default"].Opts["tablePrefix"])
		require.NotContains(t, cfg.FSC.Persistences["default"].Opts, "tls")
		require.NotContains(t, cfg.FSC.KVS, "persistence")
	})

	t.Run("table prefix", func(t *testing.T) {
		t.Parallel()
		cfg := renderPersistences(t, putPostgres("default", node.SQLOpts{DataSource: dataSource, TablePrefix: "borrower"}))

		require.Equal(t, "borrower", cfg.FSC.Persistences["default"].Opts["tablePrefix"])
	})

	t.Run("tls", func(t *testing.T) {
		t.Parallel()
		cfg := renderPersistences(t, putPostgres("default", node.SQLOpts{DataSource: dataSource, TLS: true, TLSRootCertPath: "/ca.crt"}))

		require.Equal(t, map[string]any{
			"enabled":        true,
			"ssl_mode":       "verify-full",
			"root_cert_path": "/ca.crt",
		}, cfg.FSC.Persistences["default"].Opts["tls"])
	})

	t.Run("store on a second persistence", func(t *testing.T) {
		t.Parallel()
		cfg := renderPersistences(t,
			putPostgres("default", node.SQLOpts{DataSource: dataSource}),
			putPostgres("second", node.SQLOpts{DataSource: dataSource}),
			func(o *node.Options) error {
				o.PutPostgresPersistenceName("fsc_kvs", "second")
				o.PutPostgresPersistenceName("fsc_binding", "second")
				return nil
			},
		)

		require.Equal(t, "postgres", cfg.FSC.Persistences["second"].Type)
		require.Equal(t, "second", cfg.FSC.KVS["persistence"])
		require.Contains(t, cfg.FSC.KVS, "cache", "the persistence must not displace the rest of fsc.kvs")
		require.Equal(t, map[string]any{"persistence": "second"}, cfg.FSC.Stores["binding"])
	})
}

func TestStorePersistences(t *testing.T) {
	t.Parallel()

	o := &node.Options{}
	o.PutPersistenceKey("default")
	o.PutPersistenceKey("fabric_vault")
	o.PutPostgresPersistenceName("fsc_kvs", "second")
	o.PutPostgresPersistenceName("fsc_binding", "default")

	require.Equal(t, map[string]driver.PersistenceName{"kvs": "second"}, StorePersistences(o, "fsc"))
	require.Equal(t, map[string]driver.PersistenceName{"vault": "fabric_vault_persistence"}, StorePersistences(o, "fabric"))
	require.Contains(t, GetPersistences(o, "storage"), driver.PersistenceName("fabric_vault_persistence"))
}

func TestOptions_PutPostgresPersistenceReplaces(t *testing.T) {
	t.Parallel()

	o := &node.Options{}
	o.PutPostgresPersistence("default", node.SQLOpts{DataSource: dataSource, TablePrefix: "p", TLS: true, TLSRootCertPath: "/ca.crt"})
	o.PutPostgresPersistence("default", node.SQLOpts{DataSource: dataSource, TablePrefix: "q"})

	require.Equal(t, map[driver.PersistenceName]*node.SQLOpts{
		"default": {DataSource: dataSource, TablePrefix: "q"},
	}, o.GetPostgresPersistences())
}

func TestOptions_PersistencesSurviveTemplateClone(t *testing.T) {
	t.Parallel()
	gomega.RegisterTestingT(t)

	template := node.NewNode("template")
	template.Options.PutPersistenceKey("default")
	template.Options.PutPostgresPersistence("second", node.SQLOpts{DataSource: dataSource, TablePrefix: "p", TLS: true})
	template.Options.PutPostgresPersistenceName("fsc_kvs", "second")

	clone := node.NewNodeFromTemplate("clone", template).Options
	require.Equal(t, []node.PersistenceKey{"default", "fsc_kvs"}, clone.GetAllPersistenceKeys())
	name, ok := clone.GetPersistenceName("fsc_kvs")
	require.True(t, ok)
	require.Equal(t, driver.PersistenceName("second"), name)
	require.Equal(t, map[driver.PersistenceName]*node.SQLOpts{
		"second": {DataSource: dataSource, TablePrefix: "p", TLS: true},
	}, clone.GetPostgresPersistences())

	clone.PutPostgresPersistence("third", node.SQLOpts{DataSource: dataSource})
	clone.PutPersistenceKey("fabric_vault")
	require.Len(t, clone.GetPostgresPersistences(), 2)
	require.Contains(t, clone.GetAllPersistenceKeys(), node.PersistenceKey("fabric_vault"))
}
