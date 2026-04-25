/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
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
	return WaitUntilReadyWithTLS(ctx, grpcEndpoint, nil)
}

func WaitUntilReadyWithTLS(ctx context.Context, grpcEndpoint string, tlsConfig *tls.Config) error {
	logger.Infof("Wait until ready %v", grpcEndpoint)

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

	creds := insecure.NewCredentials()
	if tlsConfig != nil {
		creds = credentials.NewTLS(tlsConfig)
	}

	options := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
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

	logger.Infof("Ready! (t=%v)", time.Since(startWaitingAt))
	return nil
}

func TLSClientConfig(rootCertPaths []string) (*tls.Config, error) {
	cp := x509.NewCertPool()
	for _, rootCertPath := range rootCertPaths {
		rootCert, err := os.ReadFile(rootCertPath)
		if err != nil {
			return nil, fmt.Errorf("read root cert %q: %w", rootCertPath, err)
		}
		if !cp.AppendCertsFromPEM(rootCert) {
			return nil, fmt.Errorf("parse root cert %q", rootCertPath)
		}
	}

	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    cp,
	}, nil
}
