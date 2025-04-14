package db

import (
	"fmt"
	"strings"
	"time"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

var (
	defaultMaxIdleConns = common.CopyPtr(common.DefaultMaxIdleConns)
	defaultMaxIdleTime  = common.CopyPtr(common.DefaultMaxIdleTime)
)

var memOpts = Opts{
	Driver:          "sqlite",
	DataSource:      "file::memory:?cache=shared",
	SkipCreateTable: false,
	SkipPragmas:     false,
	MaxOpenConns:    10,
	MaxIdleConns:    defaultMaxIdleConns,
	MaxIdleTime:     defaultMaxIdleTime,
}

func notSetError(key string) error {
	return fmt.Errorf(
		"either %s in core.yaml or the %s environment variable must be specified", key,
		strings.ToUpper("CORE_"+strings.ReplaceAll(key, ".", "_")),
	)
}

type tableOpts struct {
	tableName  string
	driverType driver2.PersistenceType
	dbOpts     driver.DbOpts
}

func (o tableOpts) TableName() string                   { return o.tableName }
func (o tableOpts) DriverType() driver2.PersistenceType { return o.driverType }
func (o tableOpts) DbOpts() driver.DbOpts               { return o.dbOpts }

type Config struct {
	Type driver2.PersistenceType
	Opts Opts
}

type Opts struct {
	Driver          common.SQLDriverType
	DataSource      string
	TablePrefix     string
	SkipCreateTable bool
	SkipPragmas     bool
	MaxOpenConns    int
	MaxIdleConns    *int
	MaxIdleTime     *time.Duration
}

type opts struct {
	driver          driver.SQLDriverType
	dataSource      string
	skipCreateTable bool
	skipPragmas     bool
	maxOpenConns    int
	maxIdleConns    int
	maxIdleTime     time.Duration
}

func (o *opts) Driver() driver.SQLDriverType { return o.driver }

func (o *opts) DataSource() string { return o.dataSource }

func (o *opts) SkipCreateTable() bool { return o.skipCreateTable }

func (o *opts) SkipPragmas() bool { return o.skipPragmas }

func (o *opts) MaxOpenConns() int { return o.maxOpenConns }

func (o *opts) MaxIdleConns() int { return o.maxIdleConns }

func (o *opts) MaxIdleTime() time.Duration { return o.maxIdleTime }
