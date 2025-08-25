/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type DriverType string

type Opts struct {
	Driver       DriverType
	DataSource   string
	SkipPragmas  bool
	MaxOpenConns int
}

func TestReadFile(t *testing.T) {
	p, err := NewProvider("./testdata")
	assert.NoError(t, err)
	testBasics(t, p)
	testMerge(t, p)
}

func TestEnvSubstitution(t *testing.T) {
	_ = os.Setenv("CORE_FSC_KVS_PERSISTENCE_OPTS_DATASOURCE", "new data source")
	_ = os.Setenv("CORE_STR", "new=string=with=characters.\\AND.CAPS")
	_ = os.Setenv("CORE_NUMBER", "10")
	_ = os.Setenv("CORE_DURATION", "10s")
	_ = os.Setenv("CORE_PATH_RELATIVE", "newfile.name")
	_ = os.Setenv("CORE_PATH_ABSOLUTE", "") // empty env vars are disregarded
	_ = os.Setenv("CORE_NON_EXISTENT_KEY", "new")
	_ = os.Setenv("CORE_NESTED_KEYS", "should not be able to replace for string")
	_ = os.Setenv("CORE_CORE_ISFINE", "yes")

	p, err := NewProvider("./testdata")
	assert.NoError(t, err)

	path, _ := filepath.Abs("testdata/newfile.name")
	assert.Equal(t, "new=string=with=characters.\\AND.CAPS", p.GetString("str"))
	assert.Equal(t, 10, p.GetInt("number"))
	assert.Equal(t, 10*time.Second, p.GetDuration("duration"))
	assert.Equal(t, path, p.GetPath("path.relative"))
	assert.Equal(t, "/absolute/path/file.name", p.GetPath("path.absolute"))

	var db Opts
	assert.Equal(t, "sql", p.GetString("fsc.kvs.persistence.type"))
	err = p.UnmarshalKey("fsc.kvs.persistence.opts", &db)
	assert.NoError(t, err)
	assert.Equal(t, DriverType("sqlite"), db.Driver)
	assert.Equal(t, "new data source", db.DataSource)
	assert.Equal(t, true, db.SkipPragmas)
	assert.Equal(t, 0, db.MaxOpenConns)

	// ensure other keys higher up the tree are not removed
	assert.Equal(t, true, p.GetBool("fsc.kvs.keyexists"))
	var c map[string]any
	err = p.UnmarshalKey("fsc.kvs", &c)
	assert.NoError(t, err)
	assert.NotNil(t, c["keyexists"])
	assert.Equal(t, true, c["keyexists"].(bool))

	assert.Equal(t, "new", p.GetString("non.existent.key"))
	assert.Equal(t, 1, p.GetInt("nested.keys.one"))
	assert.Equal(t, "yes", p.GetString("core.isFine"))
}

func testMerge(t *testing.T, p *Provider) {
	newKey := p.GetString("newKey")
	assert.Empty(t, newKey)
	wg := sync.WaitGroup{}
	wg.Add(3)
	p.OnMergeConfig(&mergeConfigHandler{wg: &wg})

	merge1Raw, err := os.ReadFile("./testdata/merge1.yaml")
	require.NoError(t, err)
	require.NoError(t, p.MergeConfig(merge1Raw))

	newKey = p.GetString("newKey")
	assert.Equal(t, "hello world", newKey)

	testBasics(t, p)

	// add nested
	merge2Raw, err := os.ReadFile("./testdata/merge2.yaml")
	require.NoError(t, err)
	require.NoError(t, p.MergeConfig(merge2Raw))

	networkName := p.GetString("fabric.network1.name")
	assert.Equal(t, "pineapple", networkName)

	merge3Raw, err := os.ReadFile("./testdata/merge3.yaml")
	require.NoError(t, err)
	require.NoError(t, p.MergeConfig(merge3Raw))

	networkName = p.GetString("fabric.network1.name")
	assert.Equal(t, "pineapple", networkName)
	networkName = p.GetString("fabric.network2.name")
	assert.Equal(t, "strawberry", networkName)

	testBasics(t, p)

	wg.Wait()
}

func testBasics(t *testing.T, p *Provider) {
	path, _ := filepath.Abs("testdata/file.name")

	assert.Equal(t, "a string", p.GetString("str"))
	assert.Equal(t, 5, p.GetInt("number"))
	assert.Equal(t, 5*time.Second, p.GetDuration("duration"))
	assert.Equal(t, path, p.GetPath("path.relative"))
	assert.Equal(t, "/absolute/path/file.name", p.GetPath("path.absolute"))
	assert.Equal(t, "/absolute/path/dir", p.GetPath("path.dir"))
	assert.Equal(t, "/absolute/path/dir/", p.GetPath("path.trailing"))

	assert.Equal(t, "", p.GetString("emptykey"))
	assert.Equal(t, true, p.GetBool("CAPITALS"))
	assert.Equal(t, true, p.GetBool("capitals"))

	var db Opts
	assert.Equal(t, "sql", p.GetString("fsc.kvs.persistence.type"))
	err := p.UnmarshalKey("fsc.kvs.persistence.opts", &db)
	assert.NoError(t, err)
	assert.Equal(t, DriverType("sqlite"), db.Driver)
	assert.Equal(t, "ds", db.DataSource)
	assert.Equal(t, true, db.SkipPragmas)
	assert.Equal(t, 0, db.MaxOpenConns)
}

type mergeConfigHandler struct {
	wg *sync.WaitGroup
}

func (m *mergeConfigHandler) OnMergeConfig() {
	m.wg.Done()
}
