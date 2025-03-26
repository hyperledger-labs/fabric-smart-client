/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	dig2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/registry"
	"go.uber.org/dig"
)

type mockConfigService struct {
	strings map[string]string
	bools   map[string]bool
}

func (s *mockConfigService) GetString(key string) string            { return s.strings[key] }
func (s *mockConfigService) GetInt(string) int                      { return 0 }
func (s *mockConfigService) GetDuration(string) time.Duration       { return 0 }
func (s *mockConfigService) GetBool(key string) bool                { return s.bools[key] }
func (s *mockConfigService) GetStringSlice(string) []string         { return nil }
func (s *mockConfigService) IsSet(string) bool                      { return false }
func (s *mockConfigService) UnmarshalKey(string, interface{}) error { return nil }
func (s *mockConfigService) ConfigFileUsed() string                 { return "" }
func (s *mockConfigService) GetPath(string) string                  { return "" }
func (s *mockConfigService) TranslatePath(string) string            { return "" }

type Opt = func(*mockConfigService)

func WithBool(key string, value bool) Opt {
	return func(s *mockConfigService) {
		s.bools[key] = value
	}
}

func WithString(key, value string) Opt {
	return func(s *mockConfigService) {
		s.strings[key] = value
	}
}

// DryRunWiring instantiates an SDK and dry runs its lifecycle to detect possible problems with dependency injection,
// i.e. missing dependencies, duplicate provisions, cyclic dependencies
func DryRunWiring[S dig2.SDK](decorator func(sdk dig2.SDK) S, opts ...Opt) error {
	return DryRunWiringWithContainer(decorator, NewContainer(dig.DryRun(true)), opts...)
}

func DryRunWiringWithContainer[S dig2.SDK](decorator func(sdk dig2.SDK) S, c dig2.Container, opts ...Opt) error {
	config := &mockConfigService{
		strings: make(map[string]string),
		bools:   make(map[string]bool),
	}
	for _, opt := range opts {
		opt(config)
	}

	provider := registry.New()
	if err := provider.RegisterService(config); err != nil {
		return err
	}
	viewSDK := NewSDKFrom(dig2.NewBaseSDK(c, config), &mockNodeProvider{provider})
	sdk := decorator(viewSDK)
	if err := sdk.Install(); err != nil {
		return err
	}
	if err := sdk.Start(context.Background()); err != nil {
		return err
	}
	return nil
}

type mockNodeProvider struct {
	*registry.ServiceProvider
}

func (p *mockNodeProvider) RegisterViewManager(node.ViewManager)   {}
func (p *mockNodeProvider) RegisterViewRegistry(node.ViewRegistry) {}
