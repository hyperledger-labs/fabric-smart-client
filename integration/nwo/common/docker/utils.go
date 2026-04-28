/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package docker

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/errdefs"
	"github.com/docker/go-connections/nat"
	"github.com/moby/moby/client"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

var logger = logging.MustGetLogger()

// Docker is a helper to manage container related actions within nwo.
type Docker struct {
	Client *client.Client
}

var (
	once           sync.Once
	singleInstance *Docker
	instanceError  error
)

// GetInstance a Docker instance, returns nil and an error in case of a failure.
func GetInstance() (*Docker, error) {
	once.Do(func() {
		dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			instanceError = errors.Wrapf(err, "failed to create new docker client instance")
		}

		singleInstance = &Docker{Client: dockerClient}
	})

	return singleInstance, instanceError
}

// CheckImagesExist returns an error if a given container images is not available, returns an error in case of a failure.
// It receives a list of container image names that are checked.
func (d *Docker) CheckImagesExist(requiredImages ...string) error {
	ctx := context.Background()
	for _, imageName := range requiredImages {
		images, err := d.Client.ImageList(ctx, image.ListOptions{
			Filters: filters.NewArgs(filters.Arg("reference", imageName)),
		})
		if err != nil {
			return err
		}

		if len(images) != 1 {
			return errors.Errorf("missing required image: %s", imageName)
		}
	}
	return nil
}

// CreateNetwork starts a docker network with the provided `networkID` as name, returns an error in case of a failure.
func (d *Docker) CreateNetwork(networkID string) error {
	_, err := d.Client.NetworkCreate(context.Background(), networkID, network.CreateOptions{Driver: "bridge"})
	if err != nil {
		return errors.Wrapf(err, "failed creating new docker network with ID='%s'", networkID)
	}
	return nil
}

func (d *Docker) NetworkInfo(networkID string) (network.Inspect, error) {
	return d.Client.NetworkInspect(context.Background(), networkID, network.InspectOptions{})
}

// Cleanup is a helper function to release all container associated with `networkID`, returns an error in case of a failure.
// It removes all container that meet the condition of the `matchName` predicate function, removes the attached volumes,
// container images, the network.
func (d *Docker) Cleanup(networkID string, matchName func(name string) bool) error {
	// TODO this method is a beast and should be refactored
	ctx := context.Background()
	containers, err := d.Client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return err
	}

	for _, c := range containers {
		for _, name := range c.Names {
			if matchName(name) {
				logger.Infof("cleanup container [%s]", name)

				// disconnect the container first
				_ = d.Client.NetworkDisconnect(ctx, networkID, c.ID, true)

				// remove container
				if err := d.Client.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true}); err != nil {
					return errors.Wrapf(err, "failed removing docker container='%s'", c.ID)
				}
				break
			}
		}
	}

	volumes, err := d.Client.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return err
	}

	for _, i := range volumes.Volumes {
		if i != nil && matchName(i.Name) {
			logger.Infof("cleanup volume [%s]", i.Name)
			err := d.Client.VolumeRemove(ctx, i.Name, false)
			if err != nil {
				return errors.Wrapf(err, "failed removing docker volume='%s'", i.Name)
			}
			break
		}
	}

	images, err := d.Client.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		return err
	}
	for _, i := range images {
		for _, tag := range i.RepoTags {
			if matchName(tag) {
				logger.Infof("cleanup image [%s]", tag)
				if _, err := d.Client.ImageRemove(ctx, i.ID, image.RemoveOptions{}); err != nil {
					return errors.Wrapf(err, "failed removing docker image='%s'", i.ID)
				}
				break
			}
		}
	}

	nw, err := d.NetworkInfo(networkID)
	if err != nil {
		if !errdefs.IsNotFound(err) {
			return errors.Wrapf(err, "failed retrieving network information for network with ID='%s'", networkID)
		}

		// we just return here as there is no need to try to remove the network that does not exist anymore
		return nil
	}

	err = d.Client.NetworkRemove(ctx, nw.ID)
	if err != nil {
		return errors.Wrapf(err, "failed removing docker network='%s' with networkID='%s'", nw.ID, networkID)
	}

	return nil
}

func (d *Docker) LocalIP(networkID string) (string, error) {
	ni, err := d.NetworkInfo(networkID)
	if err != nil {
		if !errdefs.IsNotFound(err) {
			return "", errors.Wrapf(err, "failed retrieving network information for network with ID='%s'", networkID)
		}
	}

	if len(ni.IPAM.Config) != 1 {
		return "", errors.Errorf("IPAM.Config must be set")
	}

	var config network.IPAMConfig
	for _, cfg := range ni.IPAM.Config {
		config = cfg
		break
	}

	dockerPrefix := config.Subnet[:strings.Index(config.Subnet, ".0")]

	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			if strings.Index(addr.String(), dockerPrefix) == 0 {
				ipWithSubnet := addr.String()
				i := strings.Index(ipWithSubnet, "/")
				return ipWithSubnet[:i], nil
			}
		}
	}

	return "127.0.0.1", nil
}

func PortSet(ports ...int) nat.PortSet {
	m := make(nat.PortSet, len(ports))
	for _, port := range ports {
		m[nat.Port(fmt.Sprintf("%d/tcp", port))] = struct{}{}
	}
	return m
}

func PortBindings(ports ...int) nat.PortMap {
	m := make(nat.PortMap, len(ports))
	for _, p := range ports {
		port := strconv.Itoa(p)
		m[nat.Port(port+"/tcp")] = []nat.PortBinding{{
			HostIP:   "0.0.0.0",
			HostPort: port,
		}}
	}
	return m
}

func StartLogs(cli *client.Client, containerID, loggerName string) error {
	dockerLogger := logging.MustGetLogger()
	reader, err := cli.ContainerLogs(context.Background(), containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: false,
	})
	if err != nil {
		return err
	}

	go func() {
		defer utils.IgnoreErrorFunc(reader.Close)
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			dockerLogger.Debugf("%s", scanner.Text())
		}
	}()
	return nil
}
