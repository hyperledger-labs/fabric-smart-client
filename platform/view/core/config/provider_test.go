/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	mspdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
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
	assert.NoError(err)

	path, _ := filepath.Abs("testdata/file.name")

	assert.Equal("a string", p.GetString("str"))
	assert.Equal(5, p.GetInt("number"))
	assert.Equal(5*time.Second, p.GetDuration("duration"))
	assert.Equal(path, p.GetPath("path.relative"))
	assert.Equal("/absolute/path/file.name", p.GetPath("path.absolute"))
	assert.Equal("/absolute/path/dir", p.GetPath("path.dir"))
	assert.Equal("/absolute/path/dir/", p.GetPath("path.trailing"))

	assert.Equal("", p.GetString("emptykey"))
	assert.Equal(true, p.GetBool("CAPITALS"))
	assert.Equal(true, p.GetBool("capitals"))

	var db Opts
	assert.Equal("sql", p.GetString("fsc.kvs.persistence.type"))
	err = p.UnmarshalKey("fsc.kvs.persistence.opts", &db)
	assert.NoError(err)
	assert.Equal(DriverType("sqlite"), db.Driver)
	assert.Equal("ds", db.DataSource)
	assert.Equal(true, db.SkipPragmas)
	assert.Equal(0, db.MaxOpenConns)
}

func TestMSPs(t *testing.T) {
	var confs []mspdriver.MSP
	p, _ := NewProvider("./testdata")
	err := p.UnmarshalKey("msps", &confs)
	assert.NoError(err)
	fmt.Println(confs)

	b, err := x509.ToBCCSPOpts(confs[0].Opts[x509.BCCSPOptField])
	assert.NoError(err)
	assert.NotNil(b.SW)
	assert.Equal("SHA2", b.SW.Hash)
	assert.NotNil(b.PKCS11)
	assert.Equal(256, b.PKCS11.Security)
	assert.Equal("someLabel", b.PKCS11.Label)
	assert.Equal("98765432", b.PKCS11.Pin)

}

func TestEnvSubstitution(t *testing.T) {
	os.Setenv("CORE_FSC_KVS_PERSISTENCE_OPTS_DATASOURCE", "new data source")
	os.Setenv("CORE_STR", "new=string=with=characters.\\AND.CAPS")
	os.Setenv("CORE_NUMBER", "10")
	os.Setenv("CORE_DURATION", "10s")
	os.Setenv("CORE_PATH_RELATIVE", "newfile.name")
	os.Setenv("CORE_PATH_ABSOLUTE", "") // empty env vars are disregarded
	os.Setenv("CORE_NON_EXISTENT_KEY", "new")
	os.Setenv("CORE_NESTED_KEYS", "should not be able to replace for string")

	p, err := NewProvider("./testdata")
	assert.NoError(err)

	path, _ := filepath.Abs("testdata/newfile.name")

	assert.Equal("new=string=with=characters.\\AND.CAPS", p.GetString("str"))
	assert.Equal(10, p.GetInt("number"))
	assert.Equal(10*time.Second, p.GetDuration("duration"))
	assert.Equal(path, p.GetPath("path.relative"))
	assert.Equal("/absolute/path/file.name", p.GetPath("path.absolute"))

	var db Opts
	assert.Equal("sql", p.GetString("fsc.kvs.persistence.type"))
	err = p.UnmarshalKey("fsc.kvs.persistence.opts", &db)
	assert.NoError(err)
	assert.Equal(DriverType("sqlite"), db.Driver)
	assert.Equal("new data source", db.DataSource)
	assert.Equal(true, db.SkipPragmas)
	assert.Equal(0, db.MaxOpenConns)

	// ensure other keys higher up the tree are not removed
	assert.Equal(true, p.GetBool("fsc.kvs.keyexists"))
	var c map[string]any
	err = p.UnmarshalKey("fsc.kvs", &c)
	assert.NoError(err)
	assert.NotNil(c["keyexists"])
	assert.Equal(true, c["keyexists"].(bool))

	assert.Equal("new", p.GetString("non.existent.key"))
	assert.Equal(1, p.GetInt("nested.keys.one"))
}
