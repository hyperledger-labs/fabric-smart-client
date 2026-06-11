/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fake

import (
	"context"
	"maps"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	viewendpoint "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Config struct {
	ResolversList []config.Resolver
	ResolveErr    error
	TranslateFn   func(path string) string
}

func (f *Config) Resolvers() ([]config.Resolver, error) {
	if f.ResolveErr != nil {
		return nil, f.ResolveErr
	}
	return f.ResolversList, nil
}

func (f *Config) TranslatePath(path string) string {
	if f.TranslateFn != nil {
		return f.TranslateFn(path)
	}
	return path
}

type BindCall struct {
	LongTerm  view.Identity
	Ephemeral []view.Identity
}

type AddResolverCall struct {
	Name      string
	Domain    string
	Addresses map[string]string
	Aliases   []string
	ID        []byte
	RootID    view.Identity
}

type ResolverServiceBackend struct {
	AddPublicKeyExtractorErr error
	AddPublicKeyCalls        int

	AddResolverErr error
	AddResolverFn  func(name, domain string, addresses map[string]string, aliases []string, id []byte) (view.Identity, error)
	AddResolverIDs []view.Identity
	AddCalls       []AddResolverCall

	BindErr   error
	BindCalls []BindCall
}

func (f *ResolverServiceBackend) Bind(_ context.Context, longTerm view.Identity, ephemeral ...view.Identity) error {
	f.BindCalls = append(f.BindCalls, BindCall{
		LongTerm:  longTerm,
		Ephemeral: append([]view.Identity(nil), ephemeral...),
	})
	return f.BindErr
}

func (f *ResolverServiceBackend) AddResolver(name, domain string, addresses map[string]string, aliases []string, id []byte) (view.Identity, error) {
	call := AddResolverCall{
		Name:      name,
		Domain:    domain,
		Addresses: maps.Clone(addresses),
		Aliases:   append([]string(nil), aliases...),
		ID:        append([]byte(nil), id...),
	}
	if f.AddResolverErr != nil {
		f.AddCalls = append(f.AddCalls, call)
		return nil, f.AddResolverErr
	}

	var rootID view.Identity
	if f.AddResolverFn != nil {
		var err error
		rootID, err = f.AddResolverFn(name, domain, addresses, aliases, id)
		call.RootID = rootID
		f.AddCalls = append(f.AddCalls, call)
		return rootID, err
	} else if len(f.AddResolverIDs) > 0 {
		next := f.AddResolverIDs[0]
		f.AddResolverIDs = f.AddResolverIDs[1:]
		rootID = next
	} else {
		rootID = view.Identity("root-id")
	}

	call.RootID = rootID
	f.AddCalls = append(f.AddCalls, call)
	return rootID, nil
}

func (f *ResolverServiceBackend) AddPublicKeyExtractor(_ viewendpoint.PublicKeyExtractor) error {
	f.AddPublicKeyCalls++
	return f.AddPublicKeyExtractorErr
}

type ResolverService struct {
	ID        view.Identity
	LastLabel string
}

func (s *ResolverService) GetIdentity(label string) view.Identity {
	s.LastLabel = label
	return s.ID
}

type EndpointService struct {
	ID        view.Identity
	Err       error
	CallCount int
	LastLabel string
	LastPKIID []byte
}

func (s *EndpointService) GetIdentity(label string, pkiID []byte) (view.Identity, error) {
	s.CallCount++
	s.LastLabel = label
	s.LastPKIID = pkiID
	return s.ID, s.Err
}
