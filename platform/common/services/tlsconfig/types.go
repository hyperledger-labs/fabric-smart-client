/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package tlsconfig resolves, merges, validates and loads the TLS configuration of every
// FSC surface. One server-side template and one client-side template, reused verbatim
// everywhere, with the composite of the two on surfaces that both listen and dial.
//
// A block is written in the nested form Hyperledger Fabric uses — cert: {file: ...},
// rootCAs: {files: [...]} — and every field is a pointer, so a field the configuration omits
// inherits from the parent block while a field it sets overrides that parent even when the
// value is false or an empty list. Keeping "absent" distinct from "false" is the whole reason
// for the pointers: a plain bool cannot say "this listener explicitly opts out" and "this
// listener said nothing" differently.
//
// Resolving a block decodes it strictly, so an unknown key is an error naming both the key and
// the block rather than a silently ignored setting; reads every file it names; validates the
// combination; and yields one [grpc.SecureOptions] holding the PEM contents, ready for a
// listener or a dialer.
//
// The configuration keys themselves, and the replacements for those no longer supported, are
// documented in docs/configuration.md.
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
//
// MinVersion and MaxVersion are crypto/tls version constants: 771 is TLS 1.2 and 772 is
// TLS 1.3. Absent means the default range, which each listener's own fallback fills in --
// fsc.web and fsc.metrics through SecureOptions.TLSConfig, fsc.grpc through the chained
// fallback in grpc/server.go's NewGRPCServerFromListener.
type ServerTLS struct {
	Enabled            *bool   `yaml:"enabled"`
	Cert               *File   `yaml:"cert"`
	Key                *File   `yaml:"key"`
	ClientAuthRequired *bool   `yaml:"clientAuthRequired"`
	ClientRootCAs      *Files  `yaml:"clientRootCAs"`
	MinVersion         *uint16 `yaml:"minVersion"`
	MaxVersion         *uint16 `yaml:"maxVersion"`
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
	MinVersion         *uint16 `yaml:"minVersion"`
	MaxVersion         *uint16 `yaml:"maxVersion"`
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
