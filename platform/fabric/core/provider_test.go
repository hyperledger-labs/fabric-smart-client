/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package core

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// The fakes below embed their driver interface rather than implementing it in full: the
// provider only reaches a handful of methods, and an unimplemented one nil-panics rather
// than silently returning a zero value.

// stubDriver returns whatever it is configured with, so the newFNS paths can be driven
// individually.
type stubDriver struct {
	fns driver.FabricNetworkService
	err error

	newCalls []string
}

func (d *stubDriver) New(network string, _ bool) (driver.FabricNetworkService, error) {
	d.newCalls = append(d.newCalls, network)

	return d.fns, d.err
}

type stubFNS struct {
	driver.FabricNetworkService

	configService driver.ConfigService
	channels      map[string]driver.Channel
	channelErr    error
}

func (f *stubFNS) ConfigService() driver.ConfigService { return f.configService }

func (f *stubFNS) Channel(name string) (driver.Channel, error) {
	if f.channelErr != nil {
		return nil, f.channelErr
	}

	return f.channels[name], nil
}

type stubConfigService struct {
	driver.ConfigService

	channelIDs []string
}

func (c *stubConfigService) ChannelIDs() []string { return c.channelIDs }

type stubChannel struct {
	driver.Channel

	committer *stubCommitter
	delivery  *stubDelivery
	closeErr  error

	closed bool
}

func (c *stubChannel) Committer() driver.Committer { return c.committer }
func (c *stubChannel) Delivery() driver.Delivery   { return c.delivery }

func (c *stubChannel) Close() error {
	c.closed = true

	return c.closeErr
}

type stubCommitter struct {
	driver.Committer

	started bool
	err     error
}

func (c *stubCommitter) Start(context.Context) error {
	c.started = true

	return c.err
}

type stubDelivery struct {
	driver.Delivery

	started bool
	err     error
}

func (d *stubDelivery) Start(context.Context) error {
	d.started = true

	return d.err
}

// newStubFNS assembles a network service with one channel, wired so Start and Stop can
// reach its committer, delivery and Close.
func newStubFNS(channelName string) (*stubFNS, *stubChannel) {
	ch := &stubChannel{committer: &stubCommitter{}, delivery: &stubDelivery{}}

	return &stubFNS{
		configService: &stubConfigService{channelIDs: []string{channelName}},
		channels:      map[string]driver.Channel{channelName: ch},
	}, ch
}

// newProviderWithDriver builds a provider over baseYAML, whose single network names the
// "generic" driver.
func newProviderWithDriver(t *testing.T, d driver.Driver) *FSNProvider {
	t.Helper()

	provider, err := NewFabricNetworkServiceProvider(
		newTestProvider(t, baseYAML),
		[]NamedDriver{{Name: "generic", Driver: d}},
		nil,
	)
	require.NoError(t, err)

	return provider
}

// TestFSNProvider_DefaultName checks the default network's name is reported.
func TestFSNProvider_DefaultName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "default", newProviderWithDriver(t, &stubDriver{}).DefaultName())
}

// TestFabricNetworkService_InstantiatesAndCaches checks the network is built on first
// access and served from the cache afterwards.
func TestFabricNetworkService_InstantiatesAndCaches(t *testing.T) {
	t.Parallel()

	fns, _ := newStubFNS("default-channel")
	d := &stubDriver{fns: fns}
	provider := newProviderWithDriver(t, d)

	first, err := provider.FabricNetworkService("default")
	require.NoError(t, err)
	assert.Same(t, fns, first)

	second, err := provider.FabricNetworkService("default")
	require.NoError(t, err)
	assert.Same(t, first, second)
	assert.Len(t, d.newCalls, 1, "the driver is only asked once")
}

// TestFabricNetworkService_EmptyNameUsesDefault checks an empty name resolves to the
// default network.
func TestFabricNetworkService_EmptyNameUsesDefault(t *testing.T) {
	t.Parallel()

	fns, _ := newStubFNS("default-channel")
	d := &stubDriver{fns: fns}

	got, err := newProviderWithDriver(t, d).FabricNetworkService("")

	require.NoError(t, err)
	assert.Same(t, fns, got)
	assert.Equal(t, []string{"default"}, d.newCalls)
}

// TestFabricNetworkService_UnknownNetwork checks a name that is not configured is reported.
func TestFabricNetworkService_UnknownNetwork(t *testing.T) {
	t.Parallel()

	_, err := newProviderWithDriver(t, &stubDriver{}).FabricNetworkService("not-configured")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not-configured")
}

// TestNewFNS_DriverNotRegistered checks a network naming a driver the provider was not
// given is reported rather than falling back to another driver.
func TestNewFNS_DriverNotRegistered(t *testing.T) {
	t.Parallel()

	provider, err := NewFabricNetworkServiceProvider(newTestProvider(t, baseYAML), nil, nil)
	require.NoError(t, err)

	_, err = provider.FabricNetworkService("default")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "generic")
}

// TestNewFNS_DriverFails checks a driver error is propagated.
func TestNewFNS_DriverFails(t *testing.T) {
	t.Parallel()

	d := &stubDriver{err: errors.New("driver exploded")}

	_, err := newProviderWithDriver(t, d).FabricNetworkService("default")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "driver exploded")
}

// TestStart checks every channel of every configured network has its committer and
// delivery service started.
func TestStart(t *testing.T) {
	t.Parallel()

	fns, ch := newStubFNS("default-channel")

	require.NoError(t, newProviderWithDriver(t, &stubDriver{fns: fns}).Start(t.Context()))

	assert.True(t, ch.committer.started)
	assert.True(t, ch.delivery.started)
}

// TestStart_CommitterFails checks a committer that will not start is reported.
func TestStart_CommitterFails(t *testing.T) {
	t.Parallel()

	fns, ch := newStubFNS("default-channel")
	ch.committer.err = errors.New("committer exploded")

	err := newProviderWithDriver(t, &stubDriver{fns: fns}).Start(t.Context())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "committer exploded")
	assert.False(t, ch.delivery.started, "delivery is not started after the committer fails")
}

// TestStart_DeliveryFails checks a delivery service that will not start is reported.
func TestStart_DeliveryFails(t *testing.T) {
	t.Parallel()

	fns, ch := newStubFNS("default-channel")
	ch.delivery.err = errors.New("delivery exploded")

	err := newProviderWithDriver(t, &stubDriver{fns: fns}).Start(t.Context())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "delivery exploded")
}

// TestStart_ChannelFails checks a channel that cannot be resolved is reported.
func TestStart_ChannelFails(t *testing.T) {
	t.Parallel()

	fns, _ := newStubFNS("default-channel")
	fns.channelErr = errors.New("no such channel")

	err := newProviderWithDriver(t, &stubDriver{fns: fns}).Start(t.Context())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such channel")
}

// TestStart_NetworkFails checks a network that cannot be instantiated is reported.
func TestStart_NetworkFails(t *testing.T) {
	t.Parallel()

	d := &stubDriver{err: errors.New("driver exploded")}

	err := newProviderWithDriver(t, d).Start(t.Context())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "driver exploded")
}

// TestStop checks every channel is closed.
func TestStop(t *testing.T) {
	t.Parallel()

	fns, ch := newStubFNS("default-channel")

	require.NoError(t, newProviderWithDriver(t, &stubDriver{fns: fns}).Stop())

	assert.True(t, ch.closed)
}

// TestStop_ChannelCloseFails checks a channel that fails to close is logged rather than
// aborting the rest of the shutdown.
func TestStop_ChannelCloseFails(t *testing.T) {
	t.Parallel()

	fns, ch := newStubFNS("default-channel")
	ch.closeErr = errors.New("close exploded")

	require.NoError(t, newProviderWithDriver(t, &stubDriver{fns: fns}).Stop())

	assert.True(t, ch.closed)
}

// TestStop_NetworkFails checks a network that cannot be instantiated aborts the shutdown.
func TestStop_NetworkFails(t *testing.T) {
	t.Parallel()

	d := &stubDriver{err: errors.New("driver exploded")}

	err := newProviderWithDriver(t, d).Stop()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "driver exploded")
}

// stubServiceProvider returns whatever it is configured with, for the service-lookup paths.
type stubServiceProvider struct {
	service any
	err     error
}

func (p *stubServiceProvider) GetService(any) (any, error) { return p.service, p.err }

// TestGetFabricNetworkServiceProvider covers the three outcomes of the service lookup.
func TestGetFabricNetworkServiceProvider(t *testing.T) {
	t.Parallel()

	t.Run("returns the registered provider", func(t *testing.T) {
		t.Parallel()

		registered := newProviderWithDriver(t, &stubDriver{})

		got, err := GetFabricNetworkServiceProvider(&stubServiceProvider{service: registered})

		require.NoError(t, err)
		assert.Same(t, registered, got)
	})

	t.Run("reports a lookup failure", func(t *testing.T) {
		t.Parallel()

		_, err := GetFabricNetworkServiceProvider(&stubServiceProvider{err: errors.New("not registered")})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not registered")
	})

	t.Run("reports a service of the wrong type", func(t *testing.T) {
		t.Parallel()

		_, err := GetFabricNetworkServiceProvider(&stubServiceProvider{service: "not a provider"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected service type")
	})
}

// TestNewFNS_NoDriverNamedTriesAll checks that a network which names no driver is offered to
// every registered driver in turn, and the first that yields a network wins.
func TestNewFNS_NoDriverNamedTriesAll(t *testing.T) {
	t.Parallel()

	const noDriverYAML = `
fabric:
  enabled: true
  default:
    default: true
    channels:
      - Name: default-channel
`

	fns, _ := newStubFNS("default-channel")
	failing := &stubDriver{err: errors.New("cannot handle this network")}
	working := &stubDriver{fns: fns}

	provider, err := NewFabricNetworkServiceProvider(
		newTestProvider(t, noDriverYAML),
		[]NamedDriver{{Name: "failing", Driver: failing}, {Name: "working", Driver: working}},
		nil,
	)
	require.NoError(t, err)

	got, err := provider.FabricNetworkService("default")

	require.NoError(t, err)
	assert.Same(t, fns, got)
}

// TestNewFNS_NoDriverNamedAndNoneWork checks the error when no registered driver can build
// the network.
func TestNewFNS_NoDriverNamedAndNoneWork(t *testing.T) {
	t.Parallel()

	const noDriverYAML = `
fabric:
  enabled: true
  default:
    default: true
    channels:
      - Name: default-channel
`

	provider, err := NewFabricNetworkServiceProvider(
		newTestProvider(t, noDriverYAML),
		[]NamedDriver{{Name: "failing", Driver: &stubDriver{err: errors.New("cannot handle this network")}}},
		nil,
	)
	require.NoError(t, err)

	_, err = provider.FabricNetworkService("default")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no network driver found")
}

// TestValidateIdentifier covers the three ways an identifier can be rejected.
func TestValidateIdentifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		err   string
	}{
		{"empty", "", "cannot be empty"},
		{"too long", strings.Repeat("a", maxIdentifierLength+1), "cannot be longer than"},
		{"illegal characters", "Bad_Name", "illegal characters"},
		{"valid", "my-channel.1", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateIdentifier(tc.input)

			if tc.err == "" {
				assert.NoError(t, err)

				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.err)
		})
	}
}

// TestAddNetwork_RejectsEmptyChannelName checks a channel declared without a name is
// rejected, rather than reaching the identifier rules with an empty string.
func TestAddNetwork_RejectsEmptyChannelName(t *testing.T) {
	t.Parallel()

	provider, err := NewFabricNetworkServiceProvider(newTestProvider(t, baseYAML), nil, nil)
	require.NoError(t, err)

	err = provider.AddNetwork([]byte(`
fabric:
  other:
    driver: generic
    channels:
      - Name: ""
`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "channel name is empty")
}
