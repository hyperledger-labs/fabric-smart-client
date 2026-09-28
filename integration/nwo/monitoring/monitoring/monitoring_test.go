/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package monitoring

import (
	"os"
	"testing"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	"github.com/tedsuo/ifrit/grouper"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	nwocontext "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/context"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

// TestMain wires Gomega's fail handler to panic, the same way integration.go
// does for Ginkgo specs, so the gomega.Expect guards under test raise a
// catchable panic instead of aborting the process outside of Ginkgo.
func TestMain(m *testing.M) {
	gomega.RegisterFailHandler(func(message string, _ ...int) { panic(message) })
	os.Exit(m.Run())
}

// fakePlatform is a minimal api.Platform double used to register platforms of
// a given type under a context without building a real *fsc.Platform.
type fakePlatform struct {
	name string
	typ  string
}

func (f *fakePlatform) Name() string            { return f.name }
func (f *fakePlatform) Type() string            { return f.typ }
func (*fakePlatform) GenerateConfigTree()       {}
func (*fakePlatform) GenerateArtifacts()        {}
func (*fakePlatform) Load()                     {}
func (*fakePlatform) Members() []grouper.Member { return nil }
func (*fakePlatform) PostRun(bool)              {}
func (*fakePlatform) Cleanup()                  {}

type fakeMonitoringPlatform struct {
	ctx api.Context
}

func (*fakeMonitoringPlatform) HyperledgerExplorer() bool { return false }
func (f *fakeMonitoringPlatform) GetContext() api.Context { return f.ctx }
func (*fakeMonitoringPlatform) ConfigDir() string         { return "" }
func (*fakeMonitoringPlatform) NetworkID() string         { return "" }
func (*fakeMonitoringPlatform) PrometheusGrafana() bool   { return true }
func (*fakeMonitoringPlatform) PrometheusPort() int       { return 0 }
func (*fakeMonitoringPlatform) GrafanaPort() int          { return 0 }

func TestFscScrapes_NoFscPlatform(t *testing.T) {
	t.Parallel()
	ctx := nwocontext.New("", 0, nil)
	ext := NewExtension(&fakeMonitoringPlatform{ctx: ctx})

	require.Panics(t, func() { ext.fscScrapes(&Prometheus{}) })
}

func TestFscScrapes_MultipleFscPlatforms(t *testing.T) {
	t.Parallel()
	ctx := nwocontext.New("", 0, nil)
	ctx.AddPlatform(&fakePlatform{name: "fsc1", typ: fsc.TopologyName})
	ctx.AddPlatform(&fakePlatform{name: "fsc2", typ: fsc.TopologyName})
	ext := NewExtension(&fakeMonitoringPlatform{ctx: ctx})

	require.Panics(t, func() { ext.fscScrapes(&Prometheus{}) })
}

func TestFscScrapes_WrongPlatformType(t *testing.T) {
	t.Parallel()
	ctx := nwocontext.New("", 0, nil)
	ctx.AddPlatform(&fakePlatform{name: "fsc1", typ: fsc.TopologyName})
	ext := NewExtension(&fakeMonitoringPlatform{ctx: ctx})

	require.Panics(t, func() { ext.fscScrapes(&Prometheus{}) })
}

func TestFscCryptoDir_NoFscPlatform(t *testing.T) {
	t.Parallel()
	ctx := nwocontext.New("", 0, nil)
	ext := NewExtension(&fakeMonitoringPlatform{ctx: ctx})

	require.Panics(t, func() { ext.fscCryptoDir() })
}
