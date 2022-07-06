/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
)

const CCEnvDefaultImage = "hyperledger/fabric-ccenv:latest"

var RequiredImages = []string{CCEnvDefaultImage}

func (p *Platform) setupDocker() error {
	// getting our docker helper, check required images exists and launch a docker network
	d, err := docker.GetInstance()
	if err != nil {
		return err
	}

	err = d.CheckImagesExist(RequiredImages...)
	if err != nil {
		return err
	}

	err = d.CreateNetwork(p.Network.NetworkID)
	if err != nil {
		return err
	}

	return nil
}

func (p *Platform) cleanupDocker() error {
	dockerClient, err := docker.GetInstance()
	if err != nil {
		return err
	}

	if err := dockerClient.Cleanup(p.Network.NetworkID, func(name string) bool {
		return strings.HasPrefix(name, "/"+p.Network.NetworkID)
	}); err != nil {
		return err
	}

	return nil
}
