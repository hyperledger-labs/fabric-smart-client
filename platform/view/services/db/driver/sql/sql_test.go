/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path"
	"strconv"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/dbtest"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

func TestSqlite(t *testing.T) {
	tempDir := t.TempDir()
	for _, c := range dbtest.Cases {
		db, err := initSqliteVersioned(tempDir, c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer db.Close()
			c.Fn(xt, db)
		})
	}
	for _, c := range dbtest.UnversionedCases {
		un, err := initSqliteUnversioned(tempDir, c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer un.Close()
			c.Fn(xt, un)
		})
	}
}

func TestPostgres(t *testing.T) {
	if os.Getenv("TEST_POSTGRES") != "true" {
		t.Skip("set environment variable TEST_POSTGRES to true to include postgres test")
	}
	if testing.Short() {
		t.Skip("skipping postgres test in short mode")
	}
	t.Log("starting postgres")
	terminate, pgConnStr, err := startPostgres(t, false)
	if err != nil {
		t.Fatal(err)
	}
	defer terminate()
	t.Log("postgres ready")

	for _, c := range dbtest.Cases {
		db, err := initPostgresVersioned(pgConnStr, c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer db.Close()
			c.Fn(xt, db)
		})
	}
	for _, c := range dbtest.UnversionedCases {
		un, err := initPostgresUnversioned(pgConnStr, c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer un.Close()
			c.Fn(xt, un)
		})
	}
}

func initSqliteUnversioned(tempDir, key string) (*Unversioned, error) {
	readDB, writeDB, err := openDB("sqlite", fmt.Sprintf("file:%s.sqlite?_pragma=busy_timeout(1000)", path.Join(tempDir, key)), 0, false)
	if err != nil {
		return nil, err
	}
	p := NewUnversioned(readDB, writeDB, "test")
	if err := p.CreateSchema(); err != nil {
		return nil, err
	}
	return p, nil
}

func initSqliteVersioned(tempDir, key string) (*Persistence, error) {
	readDB, writeDB, err := openDB("sqlite", fmt.Sprintf("%s.sqlite", path.Join(tempDir, key)), 0, false)
	if err != nil || readDB == nil || writeDB == nil {
		return nil, fmt.Errorf("database not open: %w", err)
	}
	p := NewPersistence(readDB, writeDB, "test")
	if err := p.CreateSchema(); err != nil {
		return nil, err
	}
	return p, nil
}

func initPostgresVersioned(pgConnStr, key string) (*Persistence, error) {
	readDB, writeDB, err := openDB("postgres", pgConnStr, 50, false)
	if err != nil || readDB == nil || writeDB == nil {
		return nil, fmt.Errorf("database not open: %w", err)
	}
	p := NewPersistence(readDB, writeDB, key)
	if err := p.CreateSchema(); err != nil {
		return nil, err
	}
	return p, nil
}

func initPostgresUnversioned(pgConnStr, key string) (*Unversioned, error) {
	readDB, writeDB, err := openDB("postgres", pgConnStr, 0, false)
	if err != nil || readDB == nil || writeDB == nil {
		return nil, fmt.Errorf("database not open: %w", err)
	}
	p := NewUnversioned(readDB, writeDB, key)
	if err := p.CreateSchema(); err != nil {
		return nil, fmt.Errorf("can't create schema: %w", err)
	}
	return p, nil
}

type Logger interface {
	Log(...any)
	Errorf(string, ...any)
}

func startPostgres(t Logger, printLogs bool) (func(), string, error) {
	containerImage := getEnv("POSTGRES_IMAGE", "postgres:latest")
	containerName := getEnv("POSTGRES_CONTAINER", "fsc-postgres")
	host := getEnv("POSTGRES_HOST", "localhost")
	dbName := getEnv("POSTGRES_DB", "testdb")
	user := getEnv("POSTGRES_USER", "postgres")
	pass := getEnv("POSTGRES_PASSWORD", "example")
	port := getEnv("POSTGRES_PORT", "5432")
	p, err := strconv.Atoi(port)
	if err != nil {
		return nil, "", fmt.Errorf("port must be a number: %s", port)
	}
	pgConnStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", host, p, user, pass, dbName)

	// images
	d, err := docker.GetInstance()
	if err != nil {
		return nil, "", fmt.Errorf("can't get docker instance: %s", err)
	}
	err = d.CheckImagesExist(containerImage)
	if err != nil {
		return nil, "", fmt.Errorf("image does not exist. Do: docker pull %s", containerImage)
	}

	// start container
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, "", fmt.Errorf("can't start postgres: %s", err)
	}
	ctx := context.Background()
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Env: []string{
			"POSTGRES_DB=" + dbName,
			"POSTGRES_USER=" + user,
			"POSTGRES_PASSWORD=" + pass,
		},
		Hostname: containerName,
		Image:    containerImage,
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
					HostPort: port,
				},
			},
		},
	}, nil, nil, containerName)
	if err != nil {
		return nil, "", fmt.Errorf("can't start postgres: %s", err)
	}
	closeFunc := func() {
		t.Log("removing postgres container")
		cli.ContainerRemove(ctx, resp.ID, types.ContainerRemoveOptions{Force: true})
	}
	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		closeFunc()
		return nil, "", fmt.Errorf("can't start postgres: %s", err)
	}

	// Forward logs to test logger
	if printLogs {
		go func() {
			reader, err := cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{
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
			return nil, "", fmt.Errorf("can't inspect postgres container: %s", err)
		}
		if inspect.State.Health == nil {
			closeFunc()
			return nil, "", fmt.Errorf("can't start postgres: cannot get health")
		}
		status := inspect.State.Health.Status
		switch status {
		case "unhealthy":
			closeFunc()
			return nil, "", fmt.Errorf("postgres container unhealthy")
		case "healthy":
			// wait half a second longer, the healthcheck can be overly optimistic
			time.Sleep(500 * time.Millisecond)
			return closeFunc, pgConnStr, nil
		default:
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
