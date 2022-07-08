/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	"github.com/pkg/errors"
)

const CCEnvDefaultImage = "hyperledger/fabric-ccenv:latest"

var RequiredImages = []string{CCEnvDefaultImage}

func (p *Platform) setupDocker() error {
	// getting our docker helper, check required images exists and launch a docker network
	d, err := docker.GetInstance()
	if err != nil {
		return errors.Wrapf(err, "failed to get docker helper")
	}

	// check if all docker images we need are available
	err = d.CheckImagesExist(RequiredImages...)
	if err != nil {
		return errors.Wrapf(err, "check failed; Require the following container images: %s", RequiredImages)
	}

	// create a container network associated with our platform networkID
	err = d.CreateNetwork(p.Network.NetworkID)
	if err != nil {
		return errors.Wrapf(err, "creating network '%s' failed", p.Network.NetworkID)
	}

	return nil
}

func (p *Platform) cleanupDocker() error {
	dockerClient, err := docker.GetInstance()
	if err != nil {
		return err
	}

	// remove all container components
	if err := dockerClient.Cleanup(p.Network.NetworkID, func(name string) bool {
		return strings.HasPrefix(name, "/"+p.Network.NetworkID)
	}); err != nil {
		return errors.Wrapf(err, "cleanup failed")

	}

	return nil
}
