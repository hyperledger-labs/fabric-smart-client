/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestRequestRecipientIdentityFromNodeUsesExplicitNodeAndWallet(t *testing.T) {
	t.Parallel()

	endpointService, bindingStore := newTestEndpointService(t)
	session := newTestSession()
	response, err := (&RecipientData{Identity: view.Identity("ephemeral-bob")}).Bytes()
	require.NoError(t, err)
	session.receive <- &view.Message{Status: view.OK, Payload: response}

	ctx := &testContext{
		ctx:       t.Context(),
		initiator: &testView{},
		session:   session,
		services: map[reflect.Type]interface{}{
			reflect.TypeOf((*endpoint.Service)(nil)): endpointService,
		},
	}

	recipient, err := RequestRecipientIdentityFromNode(ctx, view.Identity("node-b"), view.Identity("wallet-bob"), WithNetwork("test-network"))
	require.NoError(t, err)
	require.Equal(t, view.Identity("ephemeral-bob"), recipient)
	require.Equal(t, view.Identity("node-b"), ctx.getSessionParty)

	request := &RecipientRequest{}
	require.NoError(t, request.FromBytes(session.sentPayload))
	require.Equal(t, "test-network", request.Network)
	require.Equal(t, view.Identity("wallet-bob"), view.Identity(request.WalletID))

	require.Equal(t, []bindingCall{{longTerm: view.Identity("node-b"), ephemeral: []view.Identity{view.Identity("ephemeral-bob")}}}, bindingStore.binds)
}

func TestRespondRequestRecipientIdentityUsesRequestedWallet(t *testing.T) {
	t.Parallel()

	endpointService, bindingStore := newTestEndpointService(t)
	session := newTestSession()
	request, err := (&RecipientRequest{Network: "test-network", WalletID: []byte("wallet-bob")}).Bytes()
	require.NoError(t, err)
	session.receive <- &view.Message{Status: view.OK, Payload: request}
	session.info.Caller = view.Identity("node-a")

	ctx := &testContext{
		ctx:     t.Context(),
		me:      view.Identity("me"),
		session: session,
		services: map[reflect.Type]interface{}{
			reflect.TypeOf((*endpoint.Service)(nil)): endpointService,
			reflect.TypeOf((*fabric.NetworkServiceProvider)(nil)): fabric.NewNetworkServiceProvider(
				&testFNSProvider{fns: &testFNS{localMembership: &testLocalMembership{
					defaultIdentity: view.Identity("default-id"),
					identities: map[string]view.Identity{
						"wallet-bob": view.Identity("wallet-bob-id"),
					},
				}}},
				nil,
			),
		},
	}

	recipient, err := RespondRequestRecipientIdentity(ctx)
	require.NoError(t, err)
	require.Equal(t, view.Identity("wallet-bob-id"), recipient)

	response := &RecipientData{}
	require.NoError(t, response.FromBytes(session.sentPayload))
	require.Equal(t, view.Identity("wallet-bob-id"), response.Identity)
	require.Equal(t, []bindingCall{{longTerm: view.Identity("me"), ephemeral: []view.Identity{view.Identity("wallet-bob-id")}}}, bindingStore.binds)
}

func TestExchangeRecipientIdentitiesWithNodeUsesExplicitNodeAndWallet(t *testing.T) {
	t.Parallel()

	endpointService, bindingStore := newTestEndpointService(t)
	session := newTestSession()
	response, err := (&RecipientData{Identity: view.Identity("ephemeral-bob")}).Bytes()
	require.NoError(t, err)
	session.receive <- &view.Message{Status: view.OK, Payload: response}

	ctx := &testContext{
		ctx:       t.Context(),
		me:        view.Identity("me"),
		initiator: &testView{},
		session:   session,
		services: map[reflect.Type]interface{}{
			reflect.TypeOf((*endpoint.Service)(nil)): endpointService,
			reflect.TypeOf((*fabric.NetworkServiceProvider)(nil)): fabric.NewNetworkServiceProvider(
				&testFNSProvider{fns: &testFNS{localMembership: &testLocalMembership{
					defaultIdentity: view.Identity("alice-id"),
				}}},
				nil,
			),
		},
	}

	me, other, err := ExchangeRecipientIdentitiesWithNode(ctx, view.Identity("node-b"), view.Identity("wallet-bob"))
	require.NoError(t, err)
	require.Equal(t, view.Identity("alice-id"), me)
	require.Equal(t, view.Identity("ephemeral-bob"), other)
	require.Equal(t, view.Identity("node-b"), ctx.getSessionParty)

	request := &ExchangeRecipientRequest{}
	require.NoError(t, request.FromBytes(session.sentPayload))
	require.Equal(t, view.Identity("wallet-bob"), view.Identity(request.WalletID))
	require.Equal(t, view.Identity("alice-id"), request.RecipientData.Identity)
	require.Equal(t, []bindingCall{
		{longTerm: view.Identity("node-b"), ephemeral: []view.Identity{view.Identity("ephemeral-bob")}},
		{longTerm: view.Identity("me"), ephemeral: []view.Identity{view.Identity("alice-id")}},
	}, bindingStore.binds)
}

type testView struct{}

func (t *testView) Call(view.Context) (interface{}, error) {
	return nil, nil
}

type testContext struct {
	ctx             context.Context
	me              view.Identity
	initiator       view.View
	session         *testSession
	services        map[reflect.Type]interface{}
	getSessionParty view.Identity
}

func (c *testContext) GetService(v interface{}) (interface{}, error) {
	t, ok := v.(reflect.Type)
	if !ok {
		t = reflect.TypeOf(v)
	}
	s, ok := c.services[t]
	if !ok {
		return nil, errors.New("service not found")
	}
	return s, nil
}

func (c *testContext) ID() string {
	return "test"
}

func (c *testContext) RunView(v view.View, opts ...view.RunViewOption) (interface{}, error) {
	return v.Call(c)
}

func (c *testContext) Me() view.Identity {
	return c.me
}

func (c *testContext) IsMe(id view.Identity) bool {
	return c.me.Equal(id)
}

func (c *testContext) Initiator() view.View {
	return c.initiator
}

func (c *testContext) GetSession(caller view.View, party view.Identity, boundToViews ...view.View) (view.Session, error) {
	c.getSessionParty = party
	return c.session, nil
}

func (c *testContext) GetSessionByID(id string, party view.Identity) (view.Session, error) {
	return c.session, nil
}

func (c *testContext) Session() view.Session {
	return c.session
}

func (c *testContext) Context() context.Context {
	return c.ctx
}

func (c *testContext) OnError(callback func()) {}

func (c *testContext) StartSpan(name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return c.ctx, trace.SpanFromContext(c.ctx)
}

func (c *testContext) StartSpanFrom(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return ctx, trace.SpanFromContext(ctx)
}

type testSession struct {
	info        view.SessionInfo
	receive     chan *view.Message
	sentPayload []byte
}

func newTestSession() *testSession {
	return &testSession{
		info:    view.SessionInfo{},
		receive: make(chan *view.Message, 1),
	}
}

func (s *testSession) Info() view.SessionInfo {
	return s.info
}

func (s *testSession) Send(payload []byte) error {
	s.sentPayload = append([]byte(nil), payload...)
	return nil
}

func (s *testSession) SendWithContext(ctx context.Context, payload []byte) error {
	return s.Send(payload)
}

func (s *testSession) SendError(payload []byte) error {
	return s.Send(payload)
}

func (s *testSession) SendErrorWithContext(ctx context.Context, payload []byte) error {
	return s.Send(payload)
}

func (s *testSession) Receive() <-chan *view.Message {
	return s.receive
}

func (s *testSession) Close() {}

type bindingCall struct {
	longTerm  view.Identity
	ephemeral []view.Identity
}

type testBindingStore struct {
	binds []bindingCall
}

func (b *testBindingStore) GetLongTerm(_ context.Context, ephemeral view.Identity) (view.Identity, error) {
	for _, bind := range b.binds {
		for _, candidate := range bind.ephemeral {
			if candidate.Equal(ephemeral) {
				return bind.longTerm, nil
			}
		}
	}
	return nil, nil
}

func (b *testBindingStore) HaveSameBinding(context.Context, view.Identity, view.Identity) (bool, error) {
	return false, nil
}

func (b *testBindingStore) PutBindings(_ context.Context, longTerm view.Identity, ephemeral ...view.Identity) error {
	copyIDs := append([]view.Identity(nil), ephemeral...)
	b.binds = append(b.binds, bindingCall{longTerm: longTerm, ephemeral: copyIDs})
	return nil
}

func newTestEndpointService(t *testing.T) (*endpoint.Service, *testBindingStore) {
	t.Helper()
	bindingStore := &testBindingStore{}
	service, err := endpoint.NewService(bindingStore)
	require.NoError(t, err)
	return service, bindingStore
}

type testFNSProvider struct {
	fns fdriver.FabricNetworkService
}

func (p *testFNSProvider) Names() []string {
	return []string{"test-network"}
}

func (p *testFNSProvider) DefaultName() string {
	return "test-network"
}

func (p *testFNSProvider) FabricNetworkService(id string) (fdriver.FabricNetworkService, error) {
	return p.fns, nil
}

type testFNS struct {
	localMembership fdriver.LocalMembership
}

func (f *testFNS) Name() string {
	return "test-network"
}

func (f *testFNS) OrderingService() fdriver.Ordering {
	return nil
}

func (f *testFNS) TransactionManager() fdriver.TransactionManager {
	return nil
}

func (f *testFNS) ProcessorManager() fdriver.ProcessorManager {
	return nil
}

func (f *testFNS) LocalMembership() fdriver.LocalMembership {
	return f.localMembership
}

func (f *testFNS) IdentityProvider() fdriver.IdentityProvider {
	return &testIdentityProvider{}
}

func (f *testFNS) Channel(name string) (fdriver.Channel, error) {
	return nil, nil
}

func (f *testFNS) Ledger(name string) (fdriver.Ledger, error) {
	return nil, nil
}

func (f *testFNS) Committer(name string) (fdriver.Committer, error) {
	return nil, nil
}

func (f *testFNS) SignerService() fdriver.SignerService {
	return nil
}

func (f *testFNS) ConfigService() fdriver.ConfigService {
	return nil
}

type testIdentityProvider struct{}

func (p *testIdentityProvider) Identity(label string) (view.Identity, error) {
	return view.Identity(label), nil
}

type testLocalMembership struct {
	defaultIdentity view.Identity
	identities      map[string]view.Identity
}

func (l *testLocalMembership) DefaultIdentity() view.Identity {
	return l.defaultIdentity
}

func (l *testLocalMembership) AnonymousIdentity() (view.Identity, error) {
	return nil, nil
}

func (l *testLocalMembership) IsMe(ctx context.Context, id view.Identity) bool {
	return l.defaultIdentity.Equal(id)
}

func (l *testLocalMembership) DefaultSigningIdentity() fdriver.SigningIdentity {
	return nil
}

func (l *testLocalMembership) RegisterX509MSP(id, path, mspID string) error {
	return nil
}

func (l *testLocalMembership) RegisterIdemixMSP(id, path, mspID string) error {
	return nil
}

func (l *testLocalMembership) GetIdentityByID(id string) (view.Identity, error) {
	identity, ok := l.identities[id]
	if !ok {
		return nil, errors.New("identity not found")
	}
	return identity, nil
}

func (l *testLocalMembership) GetIdentityInfoByLabel(mspType, label string) *fdriver.IdentityInfo {
	return nil
}

func (l *testLocalMembership) GetIdentityInfoByIdentity(mspType string, id view.Identity) *fdriver.IdentityInfo {
	return nil
}

func (l *testLocalMembership) Refresh() error {
	return nil
}
