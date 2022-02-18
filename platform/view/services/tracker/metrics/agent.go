/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

// NewStatsdAgent returns a new StatsdAgent
func NewStatsdAgent(host tracing.Host, statsEndpoint tracing.StatsDSink) (*tracing.StatsdAgent, error) {
	return tracing.NewStatsdAgent(host, statsEndpoint)
}

// NewNullAgent returns a new NullAgent
func NewNullAgent() *tracing.NullAgent {
	return tracing.NewNullAgent()
}
