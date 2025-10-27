/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"bufio"
	"cmp"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	_ "modernc.org/sqlite"
)

const (
	// postgres:latest will not work on Podman, because Podman automatically prefixes it with localhost/
	// docker.io/postgres:latest will not work on Docker, because Docker omits the default repo docker.io
	// itests will not be recognized as a domain, so Podman will still prefix it with localhost
	// Hence we use fsc.itests as domain
	defaultImage         = "fsc.itests/postgres:latest"
	defaultContainerName = "" // we let the container runtime select a name
	defaultDBName        = "fsc-postgres"
	defaultUser          = "pgx_md5"
	defaultPass          = "example"
	defaultHost          = "localhost"
	defaultPort          = "0" // we let the container runtime select a port
	defaultTimeout       = 5 * time.Second
)

type Logger interface {
	Debugf(format string, args ...any)
	Errorf(format string, args ...any)
}

// ContainerConfig contains the configuration data about a postgres container.
type ContainerConfig struct {
	Image     string
	Container string
	*DbConfig
}

// DbConfig contains the configuration data about a postgres database.
type DbConfig struct {
	DBName string
	User   string
	Pass   string
	Host   string
	Port   string
}

// DataSource returns the contents of the DbConfig as data source string.
func (c *DbConfig) DataSource() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", c.Host, c.Port, c.User, c.Pass, c.DBName)
}

// DefaultConfig returns a Postgres container configuration. The configuration entries can be set via option parameters.
func DefaultConfig(opts ...option) *ContainerConfig {
	cfg := &ContainerConfig{
		Image:     defaultImage,
		Container: defaultContainerName,
		DbConfig: &DbConfig{
			DBName: defaultDBName,
			User:   defaultUser,
			Pass:   defaultPass,
			Host:   defaultHost,
			Port:   defaultPort,
		},
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
}

type option func(*ContainerConfig)

// WithPort sets the Postgres port.
func WithPort(port string) option {
	return func(c *ContainerConfig) {
		c.Port = port
	}
}

// WithHost sets the Postgres port.
func WithHost(host string) option {
	return func(c *ContainerConfig) {
		c.Host = host
	}
}

// WithUser sets the Postgres port.
func WithUser(user string) option {
	return func(c *ContainerConfig) {
		c.User = user
	}
}

// WithPassword sets the Postgres port.
func WithPassword(pw string) option {
	return func(c *ContainerConfig) {
		c.Pass = pw
	}
}

// WithDBName sets the Postgres dbname.
func WithDBName(name string) option {
	return func(c *ContainerConfig) {
		c.DBName = name
	}
}

// WithContainerName sets the Postgres dbname.
func WithContainerName(name string) option {
	return func(c *ContainerConfig) {
		c.Container = name
	}
}

// WithContainerImage sets the Postgres dbname.
func WithContainerImage(name string) option {
	return func(c *ContainerConfig) {
		c.Image = name
	}
}

// ConfigFromEnv returns a Postgres container configuration based environment variables.
func ConfigFromEnv() *ContainerConfig {
	return DefaultConfig(
		WithContainerImage(getEnv("POSTGRES_IMAGE", defaultImage)),
		WithContainerName(getEnv("POSTGRES_CONTAINER", defaultContainerName)),
		WithDBName(getEnv("POSTGRES_DB", defaultDBName)),
		WithUser(getEnv("POSTGRES_USER", defaultUser)),
		WithPassword(getEnv("POSTGRES_PASSWORD", defaultPass)),
		WithHost(getEnv("POSTGRES_HOST", defaultHost)),
		WithPort(getEnv("POSTGRES_PORT", defaultPort)),
	)
}

// ConfigFromDataSource returns a Postgres container configuration based on a given postgres connection string.
func ConfigFromDataSource(s string) (*ContainerConfig, error) {
	cfg := make(map[string]string, 6)
	for _, prop := range strings.Split(s, " ") {
		pair := strings.Split(prop, "=")
		k, v := pair[0], pair[1]
		cfg[k] = v
	}

	c := &DbConfig{
		DBName: cfg["dbname"],
		User:   cfg["user"],
		Pass:   cfg["password"],
		Host:   cfg["host"],
		Port:   cfg["port"],
	}
	if len(c.DBName) == 0 || len(c.Port) == 0 || len(c.Pass) == 0 || len(c.User) == 0 {
		return nil, fmt.Errorf("incomplete datasource: %s", s)
	}

	return &ContainerConfig{
		Image:     defaultImage,
		Container: c.DBName,
		DbConfig:  c,
	}, nil
}

// StartPostgres spawns a Postgres container with the given ContainerConfig and Logger.
// It returns a function to terminate and cleanup the spawned Postgres container, a datasource string that
// allows to connect to the database. If an error occurs, the closeF is nil and the datasource string is empty.
func StartPostgres(ctx context.Context, c *ContainerConfig, logger Logger) (func(), string, error) {
	if c == nil {
		return nil, "", fmt.Errorf("container config is nil")
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, "", fmt.Errorf("can't get docker client: %w", err)
	}

	// define postgres port inside the container
	postgresPort, _ := nat.NewPort("tcp", "5432")

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
			postgresPort: struct{}{},
		},
		Healthcheck: &container.HealthConfig{
			Test:     []string{"CMD-SHELL", fmt.Sprintf("pg_isready -U %s -d %s", c.User, c.DBName)},
			Interval: time.Second,
			Timeout:  defaultTimeout,
			Retries:  10,
		},
	}

	// define postgres port exposed by the container
	hostCfg := &container.HostConfig{
		PortBindings: nat.PortMap{
			postgresPort: []nat.PortBinding{
				{
					HostIP:   "127.0.0.1",
					HostPort: c.Port, // if c.Port is empty or 0, the container runtime selects a free port
				},
			},
		},
	}

	resp, err := cli.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, c.Container)
	if err != nil {
		return nil, "", fmt.Errorf("can't create postgres container: %w", err)
	}

	// define our close function that is returned to the caller
	closeFunc := func() {
		// we use a fresh context here as the provided ctx may be canceled already.
		// this is particularly the case if ctx is provided by a test harness and closeFunc is called via t.Cleanup.
		cctx := context.Background()
		_ = cli.ContainerRemove(cctx, resp.ID, container.RemoveOptions{RemoveVolumes: true, Force: true})
		_ = waitUntilContainerRemoved(cctx, cli, resp.ID, defaultTimeout)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		closeFunc()
		return nil, "", fmt.Errorf("can't start postgres container: %w", err)
	}

	// read the actual exposed postgres port
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
	if err := waitUntilHealth(ctx, cli, resp.ID, defaultTimeout); err != nil {
		closeFunc()
		return nil, "", err
	}

	return closeFunc, c.DataSource(), nil
}

// startContainerLogger forwards the container logs for the given logger.
// If the given logger is nil, the function returns. Errors are logged via the given logger.
// The actual container logs are logged with the DEBUG log level.
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
			logger.Debugf(scanner.Text())
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

func waitUntilContainerRemoved(ctx context.Context, cli *client.Client, containerID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		// check timeout first
		if time.Now().After(deadline) {
			return fmt.Errorf("container %s was not removed in time", containerID)
		}

		_, err := cli.ContainerInspect(ctx, containerID)
		if err != nil {
			// When the container is truly gone, Docker returns an error
			if errdefs.IsNotFound(err) {
				return nil // confirmed removed
			}
			return err // any other error is unexpected
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
	return cmp.Or(os.Getenv(key), fallback)
}
