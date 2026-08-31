/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccaas

import (
	"context"

	cerrdefs "github.com/containerd/errdefs"
	dcli "github.com/moby/moby/client"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// imageInspector reports whether a local image reference exists.
type imageInspector func(image string) (bool, error)

// EnsureImagePresent fails with an actionable message when ref is not present
// locally. nwo never builds chaincode images; `make chaincode-images` does.
func EnsureImagePresent(ref string) error {
	return ensureImagePresent(ref, dockerImageInspector)
}

func ensureImagePresent(ref string, inspect imageInspector) error {
	present, err := inspect(ref)
	if err != nil {
		return err
	}
	if !present {
		return errors.Errorf(
			"chaincode image %q not found; build it with `make chaincode-images`", ref)
	}
	return nil
}

// dockerImageInspector reports image presence via the docker daemon.
func dockerImageInspector(image string) (bool, error) {
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
