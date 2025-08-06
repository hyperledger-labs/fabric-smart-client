/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package routing

import (
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"gopkg.in/yaml.v2"
)

// staticLabelRouter is a map implementation of label routing
type staticLabelRouter map[string][]host2.PeerIPAddress

func newStaticLabelRouter(configPath string) (*staticLabelRouter, error) {
	bytes, err := os.ReadFile(configPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read config file")
	}
	wrapper := struct {
		Routes staticLabelRouter `yaml:"routes"`
	}{}
	if err := yaml.Unmarshal(bytes, &wrapper); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal config")
	}

	logger.Debugf("Found routes: %v", wrapper.Routes)
	return &wrapper.Routes, nil
}

func (r *staticLabelRouter) Lookup(label string) ([]host2.PeerIPAddress, bool) {
	addrs, ok := (*r)[label]
	return addrs, ok
}
