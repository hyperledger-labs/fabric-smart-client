/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"fmt"

	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/web"
)

// NewWebClientConfigFromFSC returns a web configuration from an FSC node configuration file
func NewWebClientConfigFromFSC(confDir string) (*web.Config, error) {
	config := &web.Config{}
	configProvider, err := config2.NewProvider(confDir)
	if err != nil {
		return nil, err
	}
	if configProvider.GetBool("fsc.web.tls.enabled") {
		config.URL = fmt.Sprintf("https://%s", configProvider.GetString("fsc.web.address"))
		config.CACert = configProvider.TranslatePath(configProvider.GetStringSlice("fsc.web.tls.clientRootCAs.files")[0])
		config.TLSCert = configProvider.GetPath("fsc.web.tls.cert.file")
		config.TLSKey = configProvider.GetPath("fsc.web.tls.key.file")
	} else {
		config.URL = fmt.Sprintf("http://%s", configProvider.GetString("fsc.web.address"))
	}
	return config, nil
}
