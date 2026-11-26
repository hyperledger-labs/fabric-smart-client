/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package iou wires the EVM IOU demo into an FSC node: it loads the node's "evm" core.yaml
// section into a Config, builds the Service the demo's views call to sign and submit IOU
// transactions, and registers it with the FSC service registry so GetService can find it.
package iou

import (
	"context"
	"math/big"
	"reflect"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/eip712"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou/chain"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou/contract"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	dig "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

// Service is what the IOU demo's views use to sign and submit transactions against the deployed
// IOU contract: Signer for EIP-712 signatures, Submitter to send transactions, and Client for
// reads (get, logs). It is built once, at node Start, after checking that the node at
// Client/Submitter actually serves the configured chain.
type Service struct {
	Client    client.EVMClient
	Signer    *eip712.Signer
	Submitter *chain.Submitter
	Contract  client.Address
	ChainID   *big.Int

	PollInterval, Timeout time.Duration
}

// NewService builds a Service from cfg: it dials cfg.Endpoint, loads cfg.Key and checks that the
// node's chain ID matches cfg.ChainID, failing otherwise so a demo node never submits a
// transaction to the wrong network.
func NewService(ctx context.Context, cfg Config) (*Service, error) {
	c, err := client.NewJSONRPCClient(cfg.Endpoint, nil)
	if err != nil {
		return nil, err
	}

	return buildService(ctx, cfg, c)
}

// buildService builds a Service against an already-constructed EVMClient, so a test can inject a
// mock in place of NewService's JSON-RPC dial.
func buildService(ctx context.Context, cfg Config, c client.EVMClient) (*Service, error) {
	chainID, err := c.ChainID(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "iou: failed to read the chain id")
	}
	want := big.NewInt(cfg.ChainID)
	if chainID.Cmp(want) != 0 {
		return nil, errors.Errorf("iou: chain id mismatch: node reports %s, configuration says %s", chainID, want)
	}

	key, err := LoadKey(cfg.Key)
	if err != nil {
		return nil, err
	}
	addr, err := client.HexToAddress(cfg.Contract)
	if err != nil {
		return nil, errors.Wrapf(err, "iou: invalid contract address [%s]", cfg.Contract)
	}

	return &Service{
		Client: c,
		Signer: eip712.NewSigner(key),
		Submitter: &chain.Submitter{
			Client:        c,
			Key:           key,
			ChainID:       chainID,
			GasMultiplier: cfg.Gas.Multiplier,
		},
		Contract:     addr,
		ChainID:      chainID,
		PollInterval: cfg.Finality.PollInterval,
		Timeout:      cfg.Finality.Timeout,
	}, nil
}

// Domain returns the EIP-712 domain the Service signs and the contract verifies against: name
// "FSC IOU", version "1", this Service's chain ID and contract address.
func (s *Service) Domain() eip712.Domain {
	return contract.Domain(s.ChainID, s.Contract)
}

// GetService returns the Service registered with sp, the way a view looks it up.
func GetService(sp services.Provider) (*Service, error) {
	s, err := sp.GetService(reflect.TypeFor[*Service]())
	if err != nil {
		return nil, err
	}
	svc, ok := s.(*Service)
	if !ok {
		return nil, errors.Errorf("unexpected service type [%T] for iou service", s)
	}

	return svc, nil
}

// SDK wires the EVM IOU demo into an FSC node, on top of the plain view platform (this demo talks
// to EVM directly over JSON-RPC, not through Fabric or Fabric-X).
type SDK struct {
	dig.SDK

	registry services.Registry
}

// NewSDK returns an SDK for registry, wrapping the view platform's own SDK.
func NewSDK(registry services.Registry) *SDK {
	return &SDK{SDK: viewsdk.NewSDK(registry), registry: registry}
}

// Start reads the node's "evm" configuration, dials it and checks its chain ID (see NewService),
// and registers the resulting Service so views can find it via GetService, before starting the
// view platform. Registering first closes the window where a responder session, triggered by the
// view platform coming up, would find no Service yet; the dial and the chain-ID check happen
// here, not in Install, so Install stays free of network calls.
func (p *SDK) Start(ctx context.Context) error {
	var cfg Config
	if err := p.ConfigService().UnmarshalKey("evm", &cfg); err != nil {
		return errors.Wrap(err, "iou: failed to load the evm configuration")
	}

	service, err := NewService(ctx, cfg)
	if err != nil {
		return err
	}
	if err := p.registry.RegisterService(service); err != nil {
		return err
	}

	return p.SDK.Start(ctx)
}
