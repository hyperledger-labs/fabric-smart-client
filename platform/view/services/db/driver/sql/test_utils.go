/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

type Logger interface {
	Log(...any)
	Errorf(string, ...any)
}

var port int32 = 5432

type PostgresConfig struct {
	Image     string
	Container string
	DBName    string
	User      string
	Pass      string
	Host      string
	Port      int
}

func (c *PostgresConfig) DataSource() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", c.Host, c.Port, c.User, c.Pass, c.DBName)
}

type fmtLogger struct{}

func (l *fmtLogger) Log(args ...any) {
	fmt.Println(args...)
}
func (l *fmtLogger) Errorf(format string, args ...any) {
	_ = fmt.Errorf(format, args...)
}

func DefaultConfig(node string) *PostgresConfig {
	return &PostgresConfig{
		Image:     "postgres:latest",
		Container: fmt.Sprintf("fsc-postgres-%s", node),
		DBName:    "tokendb",
		User:      "postgres",
		Pass:      "example",
		Host:      "localhost",
		Port:      int(atomic.AddInt32(&port, 1)),
	}
}

func StartPostgresWithFmt(configs map[string]*PostgresConfig) (func(), error) {
	if len(configs) == 0 {
		configs = map[string]*PostgresConfig{}
	}
	closeFuncs := make([]func(), 0, len(configs))
	errs := make([]error, 0, len(configs))
	logger := &fmtLogger{}
	for node, c := range configs {
		logger.Log("Starting DB for node ", node)
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

func startPostgresWithLogger(c PostgresConfig, t Logger, printLogs bool) (func(), error) {
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
		cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
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
			defer reader.Close()

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
