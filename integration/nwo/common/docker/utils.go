/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package docker

import (
	"net"
	"strings"
	"sync"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("fsc.integration.fabric")

// Docker is a helper to manage container related actions within nwo.
type Docker struct {
	Client *docker.Client
}

var singleInstance *Docker
var lock = &sync.Mutex{}

// GetInstance a Docker instance, returns nil and an error in case of a failure.
func GetInstance() (*Docker, error) {

	if singleInstance == nil {
		lock.Lock()
		defer lock.Unlock()

		if singleInstance != nil {
			// already created by someone else while we were waiting to get the lock
			return singleInstance, nil
		}

		dockerClient, err := docker.NewClientFromEnv()
		if err != nil {
			return nil, err
		}

		singleInstance = &Docker{dockerClient}
	}

	return singleInstance, nil
}

// CheckImagesExist returns an error if a given container images is not available, returns an error in case of a failure.
// It receives a list of container image names that are checked.
func (d *Docker) CheckImagesExist(requiredImages ...string) error {
	for _, imageName := range requiredImages {
		images, err := d.Client.ListImages(docker.ListImagesOptions{
			Filters: map[string][]string{"reference": {imageName}},
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
	_, err := d.Client.CreateNetwork(
		docker.CreateNetworkOptions{
			Name:   networkID,
			Driver: "bridge",
		},
	)

	return err
}

// Cleanup is a helper function to release all container associated with `networkID`, returns an error in case of a failure.
// It removes all container that meet the condition of the `matchName` predicate function, removes the attached volumes,
// container images, the network.
func (d *Docker) Cleanup(networkID string, matchName func(name string) bool) error {

	// TODO this method is a beast and should be refactored

	nw, err := d.Client.NetworkInfo(networkID)
	if err != nil {
		if _, ok := err.(*docker.NoSuchNetwork); !ok {
			return err
		}
	}

	containers, err := d.Client.ListContainers(docker.ListContainersOptions{All: true})
	if err != nil {
		return err
	}

	for _, c := range containers {
		for _, name := range c.Names {
			if matchName(name) {
				logger.Infof("cleanup container [%s]", name)

				// disconnect the container first
				_ = d.Client.DisconnectNetwork(networkID, docker.NetworkConnectionOptions{Force: true, Container: c.ID})

				// remove container
				if err := d.Client.RemoveContainer(docker.RemoveContainerOptions{ID: c.ID, Force: true}); err != nil {
					return err
				}
				break
			}
		}
	}

	volumes, err := d.Client.ListVolumes(docker.ListVolumesOptions{})
	if err != nil {
		return err
	}

	for _, i := range volumes {
		if strings.HasPrefix(i.Name, networkID) {
			logger.Infof("cleanup volume [%s]", i.Name)
			err := d.Client.RemoveVolumeWithOptions(docker.RemoveVolumeOptions{
				Name:  i.Name,
				Force: false,
			})
			if err != nil {
				return err
			}
			break
		}
	}

	images, err := d.Client.ListImages(docker.ListImagesOptions{All: true})
	if err != nil {
		return err
	}
	for _, i := range images {
		for _, tag := range i.RepoTags {
			if strings.HasPrefix(tag, networkID) {
				logger.Infof("cleanup image [%s]", tag)
				if err := d.Client.RemoveImage(i.ID); err != nil {
					return err
				}
				break
			}
		}
	}

	err = d.Client.RemoveNetwork(nw.ID)
	if err != nil {
		return err
	}

	return nil
}

func (d *Docker) LocalIP(networkID string) (string, error) {
	ni, err := d.Client.NetworkInfo(networkID)
	if err != nil {
		return "", err
	}

	if len(ni.IPAM.Config) != 1 {
		return "", errors.Errorf("IPAM.Config must be set")
	}

	var config docker.IPAMConfig
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
