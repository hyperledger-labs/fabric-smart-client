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
		db := initSqliteVersioned(t, tempDir, c.Name)
		t.Run(c.Name, func(xt *testing.T) {
			defer db.Close()
			c.Fn(xt, db)
		})
	}
	for _, c := range dbtest.UnversionedCases {
		un := initSqliteUnversioned(t, tempDir, c.Name)
		t.Run(c.Name, func(xt *testing.T) {
			defer un.Close()
			c.Fn(xt, un)
		})
	}
}

func TestPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping postgres test in short mode")
	}
	t.Log("starting postgres")
	terminate, pgConnStr := startPostgres(t, false)
	defer terminate()
	t.Log("postgres ready")

	for _, c := range dbtest.Cases {
		db := initPostgresVersioned(t, pgConnStr, c.Name)
		t.Run(c.Name, func(xt *testing.T) {
			defer db.Close()
			c.Fn(xt, db)
		})
	}
	for _, c := range dbtest.UnversionedCases {
		un := initPostgresUnversioned(t, pgConnStr, c.Name)
		t.Run(c.Name, func(xt *testing.T) {
			defer un.Close()
			c.Fn(xt, un)
		})
	}
}

func initSqliteUnversioned(t *testing.T, tempDir, key string) *Unversioned {
	db, err := openDB("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", path.Join(tempDir, key)), 2)
	if err != nil {
		t.Fatal(err)
	}
	p := NewUnversioned(db, "test")
	if err := p.CreateSchema(); err != nil {
		t.Fatal(err)
	}
	return p
}

func initSqliteVersioned(t *testing.T, tempDir, key string) *Persistence {
	db, err := openDB("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", path.Join(tempDir, key)), 2)
	if err != nil {
		t.Fatal(err)
	}
	p := NewPersistence(db, "test")
	if err := p.CreateSchema(); err != nil {
		t.Fatal(err)
	}
	return p
}

func initPostgresVersioned(t *testing.T, pgConnStr, key string) *Persistence {
	db, err := openDB("postgres", pgConnStr, 10)
	if err != nil {
		t.Fatal(err)
	}
	if db == nil {
		t.Fatal("database is nil")
	}
	p := NewPersistence(db, key)
	if err := p.CreateSchema(); err != nil {
		t.Fatal(err)
	}
	return p
}

func initPostgresUnversioned(t *testing.T, pgConnStr, key string) *Unversioned {
	db, err := openDB("postgres", pgConnStr, 10)
	if err != nil {
		t.Fatal(err)
	}
	if db == nil {
		t.Fatal("database is nil")
	}
	p := NewUnversioned(db, key)
	if err := p.CreateSchema(); err != nil {
		t.Fatal(err)
	}
	return p
}

func startPostgres(t *testing.T, printLogs bool) (func(), string) {
	containerImage := getEnv("POSTGRES_IMAGE", "postgres:latest")
	containerName := getEnv("POSTGRES_CONTAINER", "fsc-postgres")
	host := getEnv("POSTGRES_HOST", "localhost")
	dbName := getEnv("POSTGRES_DB", "testdb")
	user := getEnv("POSTGRES_USER", "postgres")
	pass := getEnv("POSTGRES_PASSWORD", "example")
	port := getEnv("POSTGRES_PORT", "5432")
	p, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("port must be a number: %s", port)
	}
	pgConnStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", host, p, user, pass, dbName)

	// images
	d, err := docker.GetInstance()
	if err != nil {
		t.Fatalf("can't get docker instance: %s", err)
	}
	err = d.CheckImagesExist(containerImage)
	if err != nil {
		t.Fatalf("image does not exist. Do: docker pull %s", containerImage)
	}

	// start container
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("can't start postgres: %s", err)
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
		t.Fatalf("can't start postgres: %s", err)
	}
	closeFunc := func() {
		t.Log("removing postgres container")
		cli.ContainerRemove(ctx, resp.ID, types.ContainerRemoveOptions{Force: true})
	}
	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		closeFunc()
		t.Fatalf("can't start postgres: %s", err)
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
			t.Fatalf("can't inspect postgres container: %s", err)
		}
		if inspect.State.Health == nil {
			closeFunc()
			t.Fatal("can't start postgres: cannot get health")
		}
		status := inspect.State.Health.Status
		switch status {
		case "unhealthy":
			closeFunc()
			t.Fatal("postgres container unhealthy")
		case "healthy":
			// wait half a second longer, the healthcheck can be overly optimistic
			time.Sleep(500 * time.Millisecond)
			return closeFunc, pgConnStr
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
