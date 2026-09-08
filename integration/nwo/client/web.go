/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tlsconfig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/client"
)

// NewWebClientConfigFromFSC returns a web client configuration read from an FSC node's
// configuration directory.
//
// The node's own TLS material is resolved through tlsconfig, so it is inherited from fsc.tls
// exactly as the listener inherits it. Reading fsc.web.tls.enabled directly would report
// false for a node that inherits the field, and the client would then dial plaintext against
// a TLS listener.
func NewWebClientConfigFromFSC(confDir string) (*client.Config, error) {
	configProvider, err := config2.NewProvider(confDir)
	if err != nil {
		return nil, err
	}

	tlsOpts, err := tlsconfig.ResolveServer(configProvider, "fsc.tls", "fsc.web.tls")
	if err != nil {
		return nil, errors.Wrap(err, "failed resolving fsc.web.tls")
	}

	config := &client.Config{Host: configProvider.GetString("fsc.web.address")}
	if !tlsOpts.UseTLS {
		return config, nil
	}
	if len(tlsOpts.Certificate) == 0 {
		return nil, errors.New("web configuration has TLS enabled but no certificate")
	}
	// The node's server certificate doubles as the trust anchor and as the client
	// certificate, which is what this helper has always done.
	config.CACertRaw = tlsOpts.Certificate
	config.TLSCertRaw = tlsOpts.Certificate
	config.TLSKeyRaw = tlsOpts.Key
	return config, nil
}
