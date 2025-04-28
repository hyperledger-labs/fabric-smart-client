/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

type Docker struct {
	NetworkID      string
	RequiredImages []string
}

func (d *Docker) Setup() error {
	// getting our docker helper, check required images exists and launch a docker network
	client, err := docker.GetInstance()
	if err != nil {
		return fmt.Errorf("failed to get docker helper: %w", err)
	}

	// check if all docker images we need are available
	err = client.CheckImagesExist(d.RequiredImages...)
	if err != nil {
		return errors.Wrapf(err, "check failed; Require the following container images: %s", d.RequiredImages)
	}

	// create a container network associated with our platform networkID
	err = client.CreateNetwork(d.NetworkID)
	if err != nil {
		return errors.Wrapf(err, "creating network '%s' failed", d.NetworkID)
	}

	return nil
}

func (d *Docker) Cleanup() error {
	dockerClient, err := docker.GetInstance()
	if err != nil {
		return err
	}

	// remove all container components
	if err := dockerClient.Cleanup(d.NetworkID, func(name string) bool {
		return strings.HasPrefix(name, "/"+d.NetworkID)
	}); err != nil {
		return errors.Wrapf(err, "cleanup failed")
	}

	return nil
}

func WaitUntilReady(ctx context.Context, grpcEndpoint string) error {
	logger.Infof("Wait until read")

	startWaitingAt := time.Now()

	serviceConfig := `{
		"healthCheckConfig": {
			"serviceName": ""
		},
		"methodConfig": [{
			"name": [{"service": ""}],
			"waitForReady": true,
			"retryPolicy": {
				"MaxAttempts": 5,
				"InitialBackoff": ".1s",
				"MaxBackoff": "1s",
				"BackoffMultiplier": 2.0,
				"RetryableStatusCodes": [ "UNAVAILABLE" ]
			}
		}]
	}`

	options := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(serviceConfig),
	}

	conn, err := grpc.NewClient(grpcEndpoint, options...)
	if err != nil {
		return fmt.Errorf("grpc Dial(%q) failed: %w", grpcEndpoint, err)
	}
	defer utils.IgnoreErrorFunc(conn.Close)

	healthClient := healthgrpc.NewHealthClient(conn)
	res, err := healthClient.Check(ctx, &healthgrpc.HealthCheckRequest{
		Service: "",
	})
	if status.Code(err) == codes.Canceled {
		return fmt.Errorf("healthcheck canceled: %w", err)
	}
	if err != nil {
		return fmt.Errorf("healthcheck failed: %w", err)
	}

	if res.Status != healthgrpc.HealthCheckResponse_SERVING {
		return fmt.Errorf("invalid status .... %s", res)
	}

	logger.Infof("Ready! (t=%s)", time.Since(startWaitingAt))
	return nil
}
