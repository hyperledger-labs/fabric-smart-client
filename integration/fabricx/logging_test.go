/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricx_test

import (
	"testing"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/atsa"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/common/simple"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/deployment"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/multiendorsement"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

// chaguarLogger is the name of a logger of the Chaguar platform. It follows the package path,
// so a rename of platform/fabricx (#2626) or of the logger names (#2667) changes it, and the
// suites' specs with it.
const chaguarLogger = "fsc.platform.fabricx.core.vault"

// TestTopologies_ChaguarDebug checks that the log spec of each suite turns on Debug for the
// Chaguar platform: a spec entry matches a logger name from its first segment, so an entry that
// names only a later segment, such as fabricx=debug, matches nothing.
func TestTopologies_ChaguarDebug(t *testing.T) { //nolint:paralleltest // the topology constructors assert through the global gomega
	gomega.RegisterTestingT(t)

	for name, topologies := range map[string][]api.Topology{
		"atsa":             atsa.Topology(&atsa.SDK{}, fsc.WebSocket, integration.NoReplication),
		"deployment":       deployment.Topology(&deployment.SDK{}, fsc.WebSocket, integration.NoReplication),
		"multiendorsement": multiendorsement.Topology(&multiendorsement.SDK{}, fsc.WebSocket, integration.NoReplication),
		"simple":           simple.Topology(&simple.SDK{}, fsc.WebSocket),
	} {
		levels := &flogging.LoggerLevels{}
		require.NoError(t, levels.ActivateSpec(fscTopology(t, topologies).Logging.Spec), name)
		require.Equal(t, zapcore.DebugLevel, levels.Level(chaguarLogger), name)
		require.Equal(t, zapcore.InfoLevel, levels.Level("fsc.platform.view"), name)
	}
}

func fscTopology(t *testing.T, topologies []api.Topology) *fsc.Topology {
	t.Helper()

	for _, top := range topologies {
		if fscTop, ok := top.(*fsc.Topology); ok {
			return fscTop
		}
	}
	require.FailNow(t, "no FSC topology")
	return nil
}
