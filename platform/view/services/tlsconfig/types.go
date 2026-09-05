/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package tlsconfig resolves, merges, validates and loads the TLS configuration of every
// FSC surface. One server-side template and one client-side template, reused verbatim
// everywhere, with the composite of the two on surfaces that both listen and dial.
//
// See docs/superpowers/specs/2026-09-03-tls-configuration-refactor-design.md.
package tlsconfig

// File is a single configured path, in the nested `{file: ...}` form Hyperledger Fabric
// uses. The path is resolved relative to the configuration file.
type File struct {
	File string `yaml:"file"`
}

// Files is a list of configured paths, in the nested `{files: [...]}` form. An empty list
// set explicitly overrides an inherited non-empty one; an absent [Files] inherits.
type Files struct {
	Files []string `yaml:"files"`
}

// ServerTLS is the configured TLS of a listener accepting connections.
//
// Every field is a pointer: a nil field is absent and inherits from the parent block, while
// a non-nil field overrides it even when the value is false or empty.
type ServerTLS struct {
	Enabled            *bool  `yaml:"enabled"`
	Cert               *File  `yaml:"cert"`
	Key                *File  `yaml:"key"`
	ClientAuthRequired *bool  `yaml:"clientAuthRequired"`
	ClientRootCAs      *Files `yaml:"clientRootCAs"`
}

// ClientTLS is the configured TLS of a connection being dialled out. Its fields follow the
// same absent-versus-set rule as [ServerTLS].
type ClientTLS struct {
	Enabled            *bool   `yaml:"enabled"`
	RootCAs            *Files  `yaml:"rootCAs"`
	ClientAuthEnabled  *bool   `yaml:"clientAuthEnabled"`
	ClientCert         *File   `yaml:"clientCert"`
	ClientKey          *File   `yaml:"clientKey"`
	ServerNameOverride *string `yaml:"serverNameOverride"`
}

// TLS is the configured TLS of a surface that both listens and dials, carrying the union of
// [ServerTLS] and [ClientTLS] in one block. Its fields follow the same absent-versus-set
// rule as [ServerTLS].
type TLS struct {
	// Flat rather than embedding ServerTLS and ClientTLS: both halves carry Enabled, so
	// embedding would give an ambiguous selector and two decoders competing for `enabled`.

	Enabled            *bool   `yaml:"enabled"`
	Cert               *File   `yaml:"cert"`
	Key                *File   `yaml:"key"`
	ClientAuthRequired *bool   `yaml:"clientAuthRequired"`
	ClientRootCAs      *Files  `yaml:"clientRootCAs"`
	RootCAs            *Files  `yaml:"rootCAs"`
	ClientAuthEnabled  *bool   `yaml:"clientAuthEnabled"`
	ClientCert         *File   `yaml:"clientCert"`
	ClientKey          *File   `yaml:"clientKey"`
	ServerNameOverride *string `yaml:"serverNameOverride"`
}
