/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/hashicorp/consul/sdk/freeport"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	_ "modernc.org/sqlite"
)

const PostgresImage = "docker.io/postgres:latest"

type Logger interface {
	Log(...any)
	Errorf(string, ...any)
}

type DataSourceProvider interface {
	DataSource() string
}

type ContainerConfig struct {
	Image     string
	Container string
	*Config
}

type Config struct {
	DBName string
	User   string
	Pass   string
	Host   string
	Port   int
}

func (c *Config) DataSource() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", c.Host, c.Port, c.User, c.Pass, c.DBName)
}

func ReadDataSource(s string) (*ContainerConfig, error) {
	config := make(map[string]string, 6)
	for _, prop := range strings.Split(s, " ") {
		pair := strings.Split(prop, "=")
		key, val := pair[0], pair[1]
		config[key] = val
	}
	port, err := strconv.Atoi(config["port"])
	if err != nil {
		return nil, err
	}
	c := &Config{
		DBName: config["dbname"],
		User:   config["user"],
		Pass:   config["password"],
		Host:   config["host"],
		Port:   port,
	}
	if len(c.DBName) == 0 || c.Port == 0 || len(c.Pass) == 0 || len(c.User) == 0 {
		return nil, fmt.Errorf("incomplete datasource: %s", s)
	}

	return &ContainerConfig{
		Image:     PostgresImage,
		Container: fmt.Sprintf("fsc-postgres-%s", c.DBName),
		Config:    c,
	}, nil
}

type fmtLogger struct{}

func (l *fmtLogger) Log(args ...any) {
	fmt.Println(args...)
}
func (l *fmtLogger) Errorf(format string, args ...any) {
	_ = fmt.Errorf(format, args...)
}

func DefaultConfig(node string) *ContainerConfig {
	ports, err := freeport.Take(1)
	if err != nil {
		panic("could not take free port: " + err.Error())
	}
	return defaultConfigWithPort(node, ports[0])
}

func defaultConfigWithPort(node string, port int) *ContainerConfig {
	return &ContainerConfig{
		Image:     PostgresImage,
		Container: fmt.Sprintf("fsc-postgres-%s", node),
		Config: &Config{
			DBName: node,
			User:   "pgx_md5",
			Pass:   "example",
			Host:   "localhost",
			Port:   port,
		},
	}
}

func StartPostgresWithFmt(configs []*ContainerConfig) (func(), error) {
	if len(configs) == 0 {
		return func() {}, nil
	}
	closeFuncs := make([]func(), 0, len(configs))
	errs := make([]error, 0, len(configs))
	logger := &fmtLogger{}
	for _, c := range configs {
		logger.Log("Starting DB  ", c.DBName)
		if closeFunc, err := startPostgresWithLogger(*c, logger, true); err != nil {
			errs = append(errs, err)
		} else {
			closeFuncs = append(closeFuncs, closeFunc)
		}
	}
	closeFunc := func() {
		for _, f := range closeFuncs {
			f()
		}
	}
	if err := errors.Join(errs...); err != nil {
		closeFunc()
		return func() {}, err
	}
	return closeFunc, nil
}

func startPostgresWithLogger(c ContainerConfig, t Logger, printLogs bool) (func(), error) {
	// images
	d, err := docker.GetInstance()
	if err != nil {
		return nil, fmt.Errorf("can't get docker instance: %s", err)
	}
	err = d.CheckImagesExist(c.Image)
	if err != nil {
		return nil, fmt.Errorf("image does not exist. Do: docker pull %s", c.Image)
	}

	// start container
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("can't start postgres: %s", err)
	}
	ctx := context.Background()
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Env: []string{
			"POSTGRES_DB=" + c.DBName,
			"POSTGRES_USER=" + c.User,
			"POSTGRES_PASSWORD=" + c.Pass,
		},
		Hostname: c.Container,
		Image:    c.Image,
		Tty:      false,
		ExposedPorts: nat.PortSet{
			nat.Port("5432/tcp"): struct{}{},
		},
		Healthcheck: &container.HealthConfig{
			Test:     []string{"CMD-SHELL", "pg_isready -U postgres"},
			Interval: time.Second,
			Timeout:  time.Second,
			Retries:  10,
		},
	}, &container.HostConfig{
		PortBindings: nat.PortMap{
			nat.Port("5432/tcp"): []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: strconv.Itoa(c.Port),
				},
			},
		},
	}, nil, nil, c.Container)
	if err != nil {
		return nil, fmt.Errorf("can't start postgres: %s", err)
	}
	closeFunc := func() {
		t.Log("removing postgres container")
		_ = cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
	}
	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		closeFunc()
		return nil, fmt.Errorf("can't start postgres: %s", err)
	}

	// Forward logs to test logger
	if printLogs {
		go func() {
			reader, err := cli.ContainerLogs(ctx, resp.ID, container.LogsOptions{
				ShowStdout: false,
				ShowStderr: true,
				Follow:     true,
				Timestamps: false,
			})
			if err != nil {
				t.Errorf("can't show logs: %s", err)
			}
			defer utils.CloseMute(reader)

			scanner := bufio.NewScanner(reader)
			for scanner.Scan() {
				t.Log(scanner.Text())
			}
		}()
	}
	// wait until healthy
	for {
		inspect, err := cli.ContainerInspect(ctx, resp.ID)
		if err != nil {
			closeFunc()
			return nil, fmt.Errorf("can't inspect postgres container: %s", err)
		}
		if inspect.State.Health == nil {
			closeFunc()
			return nil, fmt.Errorf("can't start postgres: cannot get health")
		}
		status := inspect.State.Health.Status
		switch status {
		case "unhealthy":
			closeFunc()
			return nil, fmt.Errorf("postgres container unhealthy")
		case "healthy":
			// wait a bit longer, the healthcheck can be overly optimistic
			time.Sleep(2000 * time.Millisecond)
			return closeFunc, nil
		default:
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func StartPostgres(t Logger, printLogs bool) (func(), string, error) {
	port := getEnv("POSTGRES_PORT", "5432")
	p, err := strconv.Atoi(port)
	if err != nil {
		return nil, "", fmt.Errorf("port must be a number: %s", port)
	}

	c := ContainerConfig{
		Image:     getEnv("POSTGRES_IMAGE", PostgresImage),
		Container: getEnv("POSTGRES_CONTAINER", "fsc-postgres"),
		Config: &Config{
			DBName: getEnv("POSTGRES_DB", "testdb"),
			User:   getEnv("POSTGRES_USER", "pgx_md5"),
			Pass:   getEnv("POSTGRES_PASSWORD", "example"),
			Host:   getEnv("POSTGRES_HOST", "localhost"),
			Port:   p,
		},
	}
	closeFunc, err := startPostgresWithLogger(c, t, printLogs)
	if err != nil {
		return nil, "", err
	}
	return closeFunc, c.DataSource(), err
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

type dbObject interface {
	CreateSchema() error
}

type persistenceConstructor[V dbObject] func(common2.Opts, string) (V, error)

func initPersistence[V dbObject](constructor persistenceConstructor[V], pgConnStr, name string, maxOpenConns, maxIdleConns int, maxIdleTime time.Duration) (V, error) {
	p, err := constructor(common2.Opts{DataSource: pgConnStr, MaxOpenConns: maxOpenConns, MaxIdleConns: &maxIdleConns, MaxIdleTime: &maxIdleTime}, name)
	if err != nil {
		return utils.Zero[V](), err
	}
	if err := p.CreateSchema(); err != nil {
		return utils.Zero[V](), err
	}
	return p, nil
}

type TestDriver struct {
	Name    string
	ConnStr string
}

func (t *TestDriver) NewKVS(string, driver.Config) (driver.UnversionedPersistence, error) {
	return initPersistence(NewUnversionedPersistence, t.ConnStr, t.Name, 50, 10, time.Minute)
}

func (t *TestDriver) NewBinding(string, driver.Config) (driver.BindingPersistence, error) {
	return initPersistence(NewBindingPersistence, t.ConnStr, t.Name, 50, 10, time.Minute)
}

func (t *TestDriver) NewSignerInfo(string, driver.Config) (driver.SignerInfoPersistence, error) {
	return initPersistence(NewSignerInfoPersistence, t.ConnStr, t.Name, 50, 10, time.Minute)
}

func (t *TestDriver) NewAuditInfo(string, driver.Config) (driver.AuditInfoPersistence, error) {
	return initPersistence(NewAuditInfoPersistence, t.ConnStr, t.Name, 50, 10, time.Minute)
}

func (t *TestDriver) NewEndorseTx(string, driver.Config) (driver.EndorseTxPersistence, error) {
	return initPersistence(NewEndorseTxPersistence, t.ConnStr, t.Name, 50, 10, time.Minute)
}

func (t *TestDriver) NewMetadata(string, driver.Config) (driver.MetadataPersistence, error) {
	return initPersistence(NewMetadataPersistence, t.ConnStr, t.Name, 50, 10, time.Minute)
}

func (t *TestDriver) NewEnvelope(string, driver.Config) (driver.EnvelopePersistence, error) {
	return initPersistence(NewEnvelopePersistence, t.ConnStr, t.Name, 50, 10, time.Minute)
}

func (t *TestDriver) NewVault(string, driver.Config) (driver.VaultPersistence, error) {
	return initPersistence(NewVaultPersistence, t.ConnStr, t.Name, 50, 10, time.Minute)
}
