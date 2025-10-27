/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	_ "modernc.org/sqlite"
)

// postgres:latest will not work on Podman, because Podman automatically prefixes it with localhost/
// docker.io/postgres:latest will not work on Docker, because Docker omits the default repo docker.io
// itests will not be recognized as a domain, so Podman will still prefix it with localhost
// Hence we use fsc.itests as domain
const PostgresImage = "fsc.itests/postgres:latest"

type Logger interface {
	Log(...any)
	Errorf(string, ...any)
}

type (
	CloseF func()
)

type DataSourceProvider interface {
	DataSource() string
}

type ContainerConfig struct {
	Image     string
	Container string
	*DbConfig
}

func (c *DbConfig) DataSource() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", c.Host, c.Port, c.User, c.Pass, c.DBName)
}

type DbConfig struct {
	DBName string
	User   string
	Pass   string
	Host   string
	Port   string
}

type fmtLogger struct{}

func (l *fmtLogger) Log(args ...any) {
	fmt.Println(args...)
}

func (l *fmtLogger) Errorf(format string, args ...any) {
	_ = fmt.Errorf(format, args...)
}

func DefaultConfig(node string) *ContainerConfig {
	return defaultConfigWithPort(node, 5432)
}

func defaultConfigWithPort(node string, port int) *ContainerConfig {
	return &ContainerConfig{
		Image:     PostgresImage,
		Container: fmt.Sprintf("fsc-postgres-%s", node),
		DbConfig: &DbConfig{
			DBName: node,
			User:   "pgx_md5",
			Pass:   "example",
			Host:   "localhost",
			Port:   strconv.Itoa(port),
		},
	}
}

func ConfigFromEnv() *ContainerConfig {
	return &ContainerConfig{
		Image:     getEnv("POSTGRES_IMAGE", PostgresImage),
		Container: getEnv("POSTGRES_CONTAINER", ""), // we let the container runtime select a name
		DbConfig: &DbConfig{
			DBName: getEnv("POSTGRES_DB", "testdb"),
			User:   getEnv("POSTGRES_USER", "pgx_md5"),
			Pass:   getEnv("POSTGRES_PASSWORD", "example"),
			Host:   getEnv("POSTGRES_HOST", "localhost"),
			Port:   getEnv("POSTGRES_PORT", "0"), // we let the container runtime select a port
		},
	}
}

func ConfigFromDataSource(s string) (*ContainerConfig, error) {
	config := make(map[string]string, 6)
	for _, prop := range strings.Split(s, " ") {
		pair := strings.Split(prop, "=")
		key, val := pair[0], pair[1]
		config[key] = val
	}

	c := &DbConfig{
		DBName: config["dbname"],
		User:   config["user"],
		Pass:   config["password"],
		Host:   config["host"],
		Port:   config["port"],
	}
	if len(c.DBName) == 0 || len(c.Port) == 0 || len(c.Pass) == 0 || len(c.User) == 0 {
		return nil, fmt.Errorf("incomplete datasource: %s", s)
	}

	return &ContainerConfig{
		Image:     PostgresImage,
		Container: fmt.Sprintf("fsc-postgres-%s", c.DBName),
		DbConfig:  c,
	}, nil
}

func StartMultiplePostgres(ctx context.Context, configs []*ContainerConfig) (CloseF, error) {
	l := &fmtLogger{}
	closeFuncs := make([]func(), 0, len(configs))

	// start all postgres instances
	for _, c := range configs {
		// we validate configs and check that the container name and port is set;
		// even though we
		if len(c.Container) == 0 || len(c.Port) == 0 || c.Port == "0" {
			return nil, fmt.Errorf("invalid container config: %v", c)
		}

		// we ignore the returned connection str
		closeFunc, _, err := StartPostgres(ctx, c, l)
		if err != nil {
			// close all started instances
			for _, f := range closeFuncs {
				if f != nil {
					f()
				}
			}
			// exit on error
			return nil, err
		}

		closeFuncs = append(closeFuncs, closeFunc)
	}

	// merge all close functions
	closeFunc := func() {
		for _, f := range closeFuncs {
			if f != nil {
				f()
			}
		}
	}
	return closeFunc, nil
}

// StartPostgres spawns a Postgres container with the given ContainerConfig and Logger.
// It returns a CloseF function to terminal and cleanup the spawned Postgres container, a datasource string that
// allows to connect to the database. If an error occurs, the closeF is nil and the datasource string is empty.
func StartPostgres(ctx context.Context, c *ContainerConfig, logger Logger) (CloseF, string, error) {
	if c == nil {
		return nil, "", fmt.Errorf("container config is nil")
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, "", fmt.Errorf("can't get docker client: %w", err)
	}

	containerCfg := &container.Config{
		Image: c.Image,
		// note that if c.Container is empty, the container runtime picks the container name (aka hostname)
		Hostname: c.Container,
		Tty:      false,
		Env: []string{
			"POSTGRES_DB=" + c.DBName,
			"POSTGRES_USER=" + c.User,
			"POSTGRES_PASSWORD=" + c.Pass,
		},
		ExposedPorts: nat.PortSet{
			// we use the default postgres port
			nat.Port("5432/tcp"): struct{}{},
		},
		Healthcheck: &container.HealthConfig{
			Test:     []string{"CMD-SHELL", fmt.Sprintf("pg_isready -U %s -d %s", c.User, c.DBName)},
			Interval: time.Second,
			Timeout:  5 * time.Second,
			Retries:  10,
		},
	}

	hostCfg := &container.HostConfig{}

	// if the c.Port is set and not 0, we bind the port to the internal postgres port
	// otherwise we let the container runtime determine a port
	if len(c.Port) == 0 && c.Port != "0" {
		// FIXME: this is not reallt working yet
		hostCfg.PortBindings = nat.PortMap{
			nat.Port("5432/tcp"): []nat.PortBinding{
				{
					HostIP:   "127.0.0.1",
					HostPort: c.Port,
				},
			},
		}
	}

	resp, err := cli.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, c.Container)
	if err != nil {
		return nil, "", fmt.Errorf("can't create postgres container: %w", err)
	}

	// define our close function that is returned to the caller
	closeFunc := func() {
		_ = cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{RemoveVolumes: true, Force: true})
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		closeFunc()
		return nil, "", fmt.Errorf("can't start postgres container: %w", err)
	}

	hostPort, err := getPostgresPort(ctx, cli, resp.ID)
	if err != nil {
		closeFunc()
		return nil, "", err
	}
	c.Port = hostPort

	// if a logger is provided, we kick of a logger goroutine that scans the container logs
	// and writes them into the logger
	startContainerLogger(ctx, cli, resp.ID, logger)

	// wait until healthy
	if err := waitUntilHealth(ctx, cli, resp.ID, 5*time.Second); err != nil {
		closeFunc()
		return nil, "", err
	}

	return closeFunc, c.DataSource(), nil
}

// startContainerLogger forwards the container logs for the given logger.
// If the given logger is nil, the function exists. Errors are logged via the given logger.
func startContainerLogger(ctx context.Context, cli *client.Client, containerID string, logger Logger) {
	if logger == nil {
		return
	}

	go func() {
		reader, err := cli.ContainerLogs(ctx, containerID, container.LogsOptions{
			ShowStdout: false,
			ShowStderr: true,
			Follow:     true,
			Timestamps: false,
		})
		if err != nil {
			logger.Errorf("can't show logs for container %s: %v", containerID, err)
		}
		defer utils.CloseMute(reader)

		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			logger.Log(scanner.Text())
		}
	}()
}

func waitUntilHealth(ctx context.Context, cli *client.Client, containerID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		// check timeout first
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for container %s to become healthy", containerID)
		}

		inspect, err := cli.ContainerInspect(ctx, containerID)
		if err != nil {
			return fmt.Errorf("inspect failed: %w", err)
		}

		if inspect.State.Health == nil {
			return fmt.Errorf("no healthcheck defined in container %s", containerID)
		}

		switch inspect.State.Health.Status {
		case container.Healthy:
			// yeah - our postgres container is read
			// wait a bit longer, the healthcheck can be overly optimistic
			time.Sleep(2000 * time.Millisecond)
			return nil
		case container.Unhealthy:
			// :(
			return fmt.Errorf("container %s unhealthy", containerID)
		default:
		}

		// sleep and try again
		time.Sleep(500 * time.Millisecond)
	}
}

func getPostgresPort(ctx context.Context, cli *client.Client, containerID string) (string, error) {
	inspection, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", err
	}

	// Look for the exposed port 5432/tcp
	portBindings := inspection.NetworkSettings.Ports["5432/tcp"]
	if len(portBindings) == 0 {
		return "", fmt.Errorf("port 5432 not mapped")
	}

	return portBindings[0].HostPort, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
