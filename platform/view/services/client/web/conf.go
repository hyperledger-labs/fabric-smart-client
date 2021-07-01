/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

import (
	"fmt"

	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/config"
)

// NewConfigFromFSC returns a web configuration from an FSC node configuration file
func NewConfigFromFSC(confDir string) (*Config, error) {
	config := &Config{}
	configProvider, err := config2.NewProvider(confDir)
	if err != nil {
		return nil, err
	}
	config.URL = fmt.Sprintf("https://%s", configProvider.GetString("fsc.web.address"))
	config.CACert = configProvider.TranslatePath(configProvider.GetStringSlice("fsc.tls.clientRootCAs.files")[0])
	config.TLSCert = configProvider.GetPath("fsc.tls.cert.file")
	config.TLSKey = configProvider.GetPath("fsc.tls.key.file")

	return config, nil
}
