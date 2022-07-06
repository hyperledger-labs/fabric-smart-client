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

type Docker struct {
	Client *docker.Client
}

var singleInstance *Docker
var lock = &sync.Mutex{}

func GetInstance() (*Docker, error) {

	if singleInstance == nil {
		lock.Lock()
		defer lock.Unlock()

		dockerClient, err := docker.NewClientFromEnv()
		if err != nil {
			return nil, err
		}

		singleInstance = &Docker{dockerClient}
	}

	return singleInstance, nil
}

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

func (d *Docker) CreateNetwork(networkID string) error {
	_, err := d.Client.CreateNetwork(
		docker.CreateNetworkOptions{
			Name:   networkID,
			Driver: "bridge",
		},
	)

	return err
}

type matcher func(name string) bool

func (d *Docker) Cleanup(networkID string, matchName matcher) error {
	nw, err := d.Client.NetworkInfo(networkID)
	if _, ok := err.(*docker.NoSuchNetwork); err != nil && ok {
		return nil
	}
	if err != nil {
		return err
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
				if err := d.Client.DisconnectNetwork(networkID, docker.NetworkConnectionOptions{Force: true, Container: c.ID}); err != nil {
					// ignore any errors
					//return err
				}

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
