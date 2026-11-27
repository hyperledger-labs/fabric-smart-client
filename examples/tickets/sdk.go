/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tickets

import (
	"encoding/pem"
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets/api"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	common "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	fabricx "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
)

// SDK is the fabricx SDK plus the demo's REST API and web UI, mounted on the
// node's web listener.
type SDK struct {
	common.SDK
}

// NewSDK returns the ticket demo's SDK on top of a fresh fabricx SDK.
func NewSDK(registry services.Registry) *SDK {
	return NewFrom(fabricx.NewSDK(registry))
}

// NewFrom returns the ticket demo's SDK on top of sdk.
func NewFrom(sdk common.SDK) *SDK {
	return &SDK{SDK: sdk}
}

// Install installs the wrapped SDK, then mounts the API and UI.
func (p *SDK) Install() error {
	if err := p.SDK.Install(); err != nil {
		return err
	}
	return p.Container().Invoke(func(ws viewsdk.WebServer, registry services.Registry, mgr *view.Manager, cfg *config.Provider) error {
		cert, err := readCert(cfg.GetPath("fsc.tls.cert.file"))
		if err != nil {
			return errors.WithMessage(err, "failed reading the node's TLS certificate, which the API trusts")
		}
		api.Mount(ws, registry, mgr, cfg.ID(), cert)
		return nil
	})
}

// readCert returns the DER bytes of the first certificate in a PEM file.
func readCert(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed reading [%s]", path)
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.Errorf("[%s] holds no PEM certificate", path)
	}
	return block.Bytes, nil
}
