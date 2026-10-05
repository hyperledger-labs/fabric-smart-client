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

	koanfyaml "github.com/knadh/koanf/parsers/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type DriverType string

type Opts struct {
	// DriverType has on purpose a different name from what we expect in the configuration file as signaled by the yaml tag.
	DriverType   DriverType `yaml:"driver"`
	DataSource   string
	SkipPragmas  bool
	MaxOpenConns int
}

func TestReadFile(t *testing.T) { //nolint:paralleltest
	p, err := NewProvider("./testdata")
	require.NoError(t, err)
	testBasics(t, p)
	testMerge(t, p)
}

func TestProvideFromRaw(t *testing.T) { //nolint:paralleltest
	p, err := NewProvider("./testdata")
	require.NoError(t, err)

	// read content of ./testdata/core.yaml
	raw, err := os.ReadFile("./testdata/core.yaml")
	require.NoError(t, err)
	newProvider, err := p.ProvideFromRaw(raw)
	require.NoError(t, err)
	require.NoError(t, newProvider.SetConfigPath("./testdata/core.yaml"))

	// newProvider should pass the same tests as p
	testBasics(t, newProvider)
	testMerge(t, newProvider)
}

func TestEnvSubstitution(t *testing.T) { //nolint:paralleltest
	t.Setenv("CORE_FSC_KVS_PERSISTENCE_OPTS_DATASOURCE", "new data source")
	t.Setenv("CORE_STR", "new=string=with=characters.\\AND.CAPS")
	t.Setenv("CORE_NUMBER", "10")
	t.Setenv("CORE_DURATION", "10s")
	t.Setenv("CORE_PATH_RELATIVE", "newfile.name")
	t.Setenv("CORE_PATH_ABSOLUTE", "") // empty env vars are disregarded
	t.Setenv("CORE_NON_EXISTENT_KEY", "new")
	t.Setenv("CORE_NESTED_KEYS", "should not be able to replace for string")
	t.Setenv("CORE_CORE_ISFINE", "yes")

	p, err := NewProvider("./testdata")
	require.NoError(t, err)

	path, _ := filepath.Abs("testdata/newfile.name")
	assert.Equal(t, "new=string=with=characters.\\AND.CAPS", p.GetString("str"))
	assert.Equal(t, 10, p.GetInt("number"))
	assert.Equal(t, 10*time.Second, p.GetDuration("duration"))
	assert.Equal(t, path, p.GetPath("path.relative"))
	assert.Equal(t, "/absolute/path/file.name", p.GetPath("path.absolute"))

	assert.Equal(t, "new data source", p.GetString("fsc.kvs.persistence.opts.datasource"))

	var db Opts
	assert.Equal(t, "sql", p.GetString("fsc.kvs.persistence.type"))

	err = p.UnmarshalKey("fsc.kvs.persistence.opts", &db)
	require.NoError(t, err)
	assert.Equal(t, DriverType("sqlite"), db.DriverType)
	assert.Equal(t, "new data source", db.DataSource)
	assert.True(t, db.SkipPragmas)
	assert.Equal(t, 0, db.MaxOpenConns)
	err = p.UnmarshalKey("FSC.kvs.PerSistEnce.opTs", &db)
	require.NoError(t, err)
	assert.Equal(t, DriverType("sqlite"), db.DriverType)
	assert.Equal(t, "new data source", db.DataSource)
	assert.True(t, db.SkipPragmas)
	assert.Equal(t, 0, db.MaxOpenConns)

	// ensure other keys higher up the tree are not removed
	assert.True(t, p.GetBool("fsc.kvs.keyexists"))
	var c map[string]any
	err = p.UnmarshalKey("fsc.kvs", &c)
	require.NoError(t, err)
	require.NotNil(t, c["keyexists"])
	assert.True(t, c["keyexists"].(bool))

	assert.Equal(t, "new", p.GetString("non.existent.key"))
	assert.Equal(t, 1, p.GetInt("nested.keys.one"))
	assert.Equal(t, "yes", p.GetString("core.isFine"))
}

func testMerge(t *testing.T, p *Provider) {
	t.Helper()
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

	networkName := p.GetString(Join("fabric", "network1", "name"))
	assert.Equal(t, "pineapple", networkName)

	merge3Raw, err := os.ReadFile("./testdata/merge3.yaml")
	require.NoError(t, err)
	require.NoError(t, p.MergeConfig(merge3Raw))

	networkName = p.GetString(Join("fabric", "network1", "name"))
	assert.Equal(t, "pineapple", networkName)
	networkName = p.GetString(Join("fabric", "network2", "name"))
	assert.Equal(t, "strawberry", networkName)

	var s string
	require.NoError(t, p.UnmarshalKey(Join("fabric", "network1", "name"), &s))
	assert.Equal(t, "pineapple", s)
	require.NoError(t, p.UnmarshalKey(Join("fabric", "network2", "name"), &s))
	assert.Equal(t, "strawberry", s)

	testBasics(t, p)

	wg.Wait()
}

func testBasics(t *testing.T, p *Provider) {
	t.Helper()
	path, _ := filepath.Abs("testdata/file.name")

	assert.Equal(t, "a string", p.GetString("str"))
	assert.Equal(t, 5, p.GetInt("number"))
	assert.Equal(t, 5*time.Second, p.GetDuration("duration"))
	assert.Equal(t, path, p.GetPath("path.relative"))
	assert.Equal(t, "/absolute/path/file.name", p.GetPath("path.absolute"))
	assert.Equal(t, "/absolute/path/dir", p.GetPath("path.dir"))
	assert.Equal(t, "/absolute/path/dir/", p.GetPath("path.trailing"))

	assert.Empty(t, p.GetString("emptykey"))
	assert.True(t, p.GetBool("CAPITALS"))
	assert.True(t, p.GetBool("capitals"))

	var db Opts
	assert.Equal(t, "sql", p.GetString("fsc.kvs.persistence.type"))
	err := p.UnmarshalKey("fsc.kvs.persistence.opts", &db)
	require.NoError(t, err)
	assert.Equal(t, DriverType("sqlite"), db.DriverType)
	assert.Equal(t, "ds", db.DataSource)
	assert.True(t, db.SkipPragmas)
	assert.Equal(t, 0, db.MaxOpenConns)
}

type mergeConfigHandler struct {
	wg *sync.WaitGroup
}

func (m *mergeConfigHandler) OnMergeConfig() {
	m.wg.Done()
}

type mockProvider struct {
	service any
}

func (m *mockProvider) GetService(_ any) (any, error) {
	return m.service, nil
}

func TestGetProvider(t *testing.T) {
	t.Parallel()
	p := &Provider{}
	mp := &mockProvider{service: p}
	assert.Equal(t, p, GetProvider(mp))

	assert.Panics(t, func() {
		GetProvider(&mockProvider{service: nil})
	})
	assert.Panics(t, func() {
		GetProvider(&mockProvider{service: "not a provider"})
	})
}

func TestGetPath(t *testing.T) { //nolint:paralleltest
	p, err := NewProvider("./testdata")
	require.NoError(t, err)

	assert.Empty(t, p.GetPath("non.existent"))
	assert.Equal(t, filepath.Join(filepath.Dir(p.ConfigFileUsed()), "file.name"), p.GetPath("path.relative"))
}

func TestInvalidYAML(t *testing.T) { //nolint:paralleltest
	raw := []byte("a: [unclosed")

	p, err := NewProvider("./testdata")
	require.NoError(t, err)
	require.Error(t, p.MergeConfig(raw))

	_, err = p.ProvideFromRaw(raw)
	require.ErrorContains(t, err, "failed to read configuration from raw")
}

func TestConfigPathEnv(t *testing.T) { //nolint:paralleltest
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "core.yaml"), []byte("fsc:\n  id: from-env\n"), 0o600))

	t.Setenv("FSCNODE_CFG_PATH", dir)
	p, err := NewProvider("")
	require.NoError(t, err)
	assert.Equal(t, "from-env", p.ID())

	missing := filepath.Join(dir, "missing")
	t.Setenv("FSCNODE_CFG_PATH", missing)
	_, err = NewProvider("")
	require.ErrorContains(t, err, "FSCNODE_CFG_PATH "+missing+" does not exist")

	t.Setenv("FSCNODE_CFG_PATH", filepath.Join(dir, "core.yaml"))
	_, err = NewProvider("")
	require.ErrorContains(t, err, "does not exist")
}

func TestProviderMore(t *testing.T) { //nolint:paralleltest
	t.Setenv("CORE_FSC_ID", "node1")
	p, err := NewProvider("./testdata")
	require.NoError(t, err)

	// Test ID
	assert.Equal(t, "node1", p.ID())

	// Test IsSet
	assert.True(t, p.IsSet("str"))
	assert.False(t, p.IsSet("non-existent"))

	// Test GetStringSlice
	mergeSliceRaw := []byte("slice: [a, b, c]")
	err = p.MergeConfig(mergeSliceRaw)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, p.GetStringSlice("slice"))

	// Test ConfigFileUsed: it returned the empty string for a provider built from a file,
	// which this pins. Nothing in-tree reads it yet — it is an exported accessor on
	// driver.ConfigService, so downstream callers are the consumers.
	assert.Equal(t, filepath.Join("testdata", "core.yaml"),
		filepath.Join(filepath.Base(filepath.Dir(p.ConfigFileUsed())), filepath.Base(p.ConfigFileUsed())))

	// Test String
	assert.NotEmpty(t, p.String())

	// Test TranslatePath (method)
	p.fullPath = "/tmp/config/core.yaml"
	assert.Equal(t, "/tmp/config/file.txt", p.TranslatePath("file.txt"))
	assert.Empty(t, p.TranslatePath(""))

	// Test MergeConfigEvent Message
	event := &MergeConfigEvent{}
	assert.Equal(t, MergeConfigEventTopic, event.Topic())
	assert.Nil(t, event.Message())
}

func TestProviderError(t *testing.T) {
	t.Parallel()
	_, err := NewProvider("./non-existent-path")
	require.Error(t, err)
}

func TestEnvConversions(t *testing.T) { //nolint:paralleltest
	t.Setenv("CORE_ENV_INT", "123")
	t.Setenv("CORE_ENV_BOOL", "true")
	t.Setenv("CORE_ENV_FLOAT", "1.23")
	t.Setenv("CORE_ENV_MAP", "this should be ignored")

	raw := []byte(`
env:
  int: 1
  bool: false
  float: 0.1
  map:
    a: b
`)
	p, err := (&Provider{}).ProvideFromRaw(raw)
	require.NoError(t, err)

	require.Equal(t, 123, p.GetInt("env.int"))
	require.True(t, p.GetBool("env.bool"))
	// koanf doesn't have GetFloat, but we can unmarshal
	var floatVal float64
	err = p.UnmarshalKey("env.float", &floatVal)
	require.NoError(t, err)
	require.InEpsilon(t, 1.23, floatVal, 0.001)

	var mapVal map[string]any
	err = p.UnmarshalKey("env.map", &mapVal)
	require.NoError(t, err)
	require.Equal(t, "b", mapVal["a"])
}

func TestLowercaseParserRejectsCollidingKeys(t *testing.T) {
	t.Parallel()
	parser := LowercaseParser{Parser: koanfyaml.Parser()}

	for _, tc := range []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "top level",
			yaml:    "Foo: 1\nfoo: 2\n",
			wantErr: "configuration keys [Foo] and [foo] collide after lowercasing",
		},
		{
			name:    "nested",
			yaml:    "fabric:\n  Network1:\n    driver: a\n  network1:\n    driver: b\n",
			wantErr: "configuration keys [fabric.Network1] and [fabric.network1] collide after lowercasing",
		},
		{
			name:    "inside a list",
			yaml:    "fabric:\n  net:\n    channels:\n      - Name: a\n        name: b\n",
			wantErr: "configuration keys [fabric.net.channels[0].Name] and [fabric.net.channels[0].name] collide after lowercasing",
		},
		{
			name:    "inside a nested list",
			yaml:    "a:\n  - - Key: 1\n      key: 2\n",
			wantErr: "configuration keys [a[0][0].Key] and [a[0][0].key] collide after lowercasing",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Map iteration order is random, so parse repeatedly to see both key orders.
			for range 20 {
				_, err := parser.Unmarshal([]byte(tc.yaml))
				require.EqualError(t, err, tc.wantErr)
			}
		})
	}
}

func TestLowercaseParserLowercasesKeys(t *testing.T) {
	t.Parallel()
	m, err := LowercaseParser{Parser: koanfyaml.Parser()}.Unmarshal([]byte("Fabric:\n  Net:\n    Channels:\n      - Name: a\n"))
	require.NoError(t, err)
	require.Equal(t, map[string]any{"fabric": map[string]any{"net": map[string]any{"channels": []any{map[string]any{"name": "a"}}}}}, m)
}
