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
	"net/netip"
	"strconv"
	"strings"
	"sync"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/network"
	dcli "github.com/moby/moby/client"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

var logger = logging.MustGetLogger()

// Docker is a helper to manage container related actions within nwo.
type Docker struct {
	Client dcli.APIClient
}

var (
	once           sync.Once
	singleInstance *Docker
	instanceError  error
)

// GetInstance a Docker instance, returns nil and an error in case of a failure.
func GetInstance() (*Docker, error) {
	once.Do(func() {
		dockerClient, err := dcli.New(dcli.FromEnv)
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
	ctx := context.TODO()
	for _, imageName := range requiredImages {
		images, err := d.Client.ImageList(ctx, dcli.ImageListOptions{
			Filters: make(dcli.Filters).Add("reference", imageName),
		})
		if err != nil {
			return err
		}

		if len(images.Items) != 1 {
			return errors.Errorf("missing required image: %s", imageName)
		}
	}
	return nil
}

// CreateNetwork starts a docker network with the provided `networkID` as name, returns an error in case of a failure.
func (d *Docker) CreateNetwork(networkID string) error {
	_, err := d.Client.NetworkCreate(context.TODO(), networkID, dcli.NetworkCreateOptions{Driver: "bridge"})
	if err != nil {
		return errors.Wrapf(err, "failed creating new docker network with ID='%s'", networkID)
	}
	return nil
}

func (d *Docker) NetworkInfo(networkID string) (network.Inspect, error) {
	result, err := d.Client.NetworkInspect(context.TODO(), networkID, dcli.NetworkInspectOptions{})
	if err != nil {
		return network.Inspect{}, err
	}
	return result.Network, nil
}

// Cleanup is a helper function to release all container associated with `networkID`, returns an error in case of a failure.
// It removes all container that meet the condition of the `matchName` predicate function, removes the attached volumes,
// container images, the network.
func (d *Docker) Cleanup(networkID string, matchName func(name string) bool) error {
	// TODO this method is a beast and should be refactored
	ctx := context.TODO()
	containers, err := d.Client.ContainerList(ctx, dcli.ContainerListOptions{All: true})
	if err != nil {
		return err
	}

	for _, c := range containers.Items {
		for _, name := range c.Names {
			if matchName(name) {
				logger.Infof("cleanup container [%s]", name)

				// disconnect the container first
				_, _ = d.Client.NetworkDisconnect(ctx, networkID, dcli.NetworkDisconnectOptions{
					Container: c.ID,
					Force:     true,
				})

				// remove container
				if _, err := d.Client.ContainerRemove(ctx, c.ID, dcli.ContainerRemoveOptions{Force: true}); err != nil {
					return errors.Wrapf(err, "failed removing docker container='%s'", c.ID)
				}
				break
			}
		}
	}

	volumes, err := d.Client.VolumeList(ctx, dcli.VolumeListOptions{})
	if err != nil {
		return err
	}

	for _, i := range volumes.Items {
		if matchName(i.Name) {
			logger.Infof("cleanup volume [%s]", i.Name)
			_, err := d.Client.VolumeRemove(ctx, i.Name, dcli.VolumeRemoveOptions{})
			if err != nil {
				return errors.Wrapf(err, "failed removing docker volume='%s'", i.Name)
			}
			break
		}
	}

	images, err := d.Client.ImageList(ctx, dcli.ImageListOptions{All: true})
	if err != nil {
		return err
	}
	for _, i := range images.Items {
		for _, tag := range i.RepoTags {
			if matchName(tag) {
				logger.Infof("cleanup image [%s]", tag)
				if _, err := d.Client.ImageRemove(ctx, i.ID, dcli.ImageRemoveOptions{}); err != nil {
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

	_, err = d.Client.NetworkRemove(ctx, nw.ID, dcli.NetworkRemoveOptions{})
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

	subnet := config.Subnet.Addr().String()
	dockerPrefix := subnet[:strings.Index(subnet, ".0")]

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

func PortSet(ports ...int) network.PortSet {
	m := make(network.PortSet, len(ports))
	for _, port := range ports {
		m[network.MustParsePort(fmt.Sprintf("%d/tcp", port))] = struct{}{}
	}
	return m
}

func PortBindings(ports ...int) network.PortMap {
	m := make(network.PortMap, len(ports))
	hostIP := netip.MustParseAddr("0.0.0.0")
	for _, p := range ports {
		port := strconv.Itoa(p)
		m[network.MustParsePort(port+"/tcp")] = []network.PortBinding{{
			HostIP:   hostIP,
			HostPort: port,
		}}
	}
	return m
}

func StartLogs(cli dcli.APIClient, containerID, loggerName string) error {
	dockerLogger := logging.MustGetLogger()
	reader, err := cli.ContainerLogs(context.TODO(), containerID, dcli.ContainerLogsOptions{
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
