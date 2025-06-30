/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
	err = p.UnmarshalKey("fsc.kvs.persistence.opts", &db)
	assert.NoError(t, err)
	assert.Equal(t, DriverType("sqlite"), db.Driver)
	assert.Equal(t, "ds", db.DataSource)
	assert.Equal(t, true, db.SkipPragmas)
	assert.Equal(t, 0, db.MaxOpenConns)
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
}
