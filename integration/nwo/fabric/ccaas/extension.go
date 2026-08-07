/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccaas

import (
	"context"

	cerrdefs "github.com/containerd/errdefs"
	dcli "github.com/moby/moby/client"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// Mount is a host-path-to-container-path bind for chaincode data (e.g. public params).
type Mount struct {
	Source string
	Target string
}

// ChaincodeExtension is the per-chaincode hook the CCaaS deployer consults.
// When a chaincode registers none, DefaultExtension is used.
type ChaincodeExtension interface {
	// EnsureImage is the build-trigger hook, called before deployment.
	// The default checks image presence and fails fast when missing.
	EnsureImage(cc *topology.Chaincode) error
	// ContainerEnv contributes extra env vars and data mounts for the container.
	ContainerEnv(cc *topology.Chaincode) (env []string, mounts []Mount, err error)
}

// ImageInspector reports whether a local image reference exists.
type ImageInspector func(image string) (bool, error)

// DockerImageInspector reports image presence via the docker daemon.
func DockerImageInspector(image string) (bool, error) {
	cli, err := dcli.New(dcli.FromEnv)
	if err != nil {
		return false, errors.Wrapf(err, "failed to create docker client")
	}
	defer func() { _ = cli.Close() }()
	if _, err := cli.ImageInspect(context.Background(), image); err != nil {
		if cerrdefs.IsNotFound(err) {
			return false, nil
		}
		return false, errors.Wrapf(err, "failed to inspect image %s", image)
	}
	return true, nil
}

// DefaultExtension checks image presence and adds no extra env or mounts.
type DefaultExtension struct {
	Inspect ImageInspector
}

func (d DefaultExtension) EnsureImage(cc *topology.Chaincode) error {
	present, err := d.Inspect(cc.Image)
	if err != nil {
		return err
	}
	if !present {
		return errors.Errorf("chaincode image %q not found; build it with `make chaincode-images`", cc.Image)
	}
	return nil
}

func (d DefaultExtension) ContainerEnv(*topology.Chaincode) ([]string, []Mount, error) {
	return nil, nil, nil
}
