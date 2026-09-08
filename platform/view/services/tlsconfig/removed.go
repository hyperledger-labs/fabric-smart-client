/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tlsconfig

// These tables carry only the removed keys that nothing else would reject. A key that lives
// inside a tls: block needs no entry: the strict subtree decode already fails on it, naming
// both the key and the block —
//
//	invalid TLS configuration under [fabric.mynet.tls]: decoding failed due to the
//	following error(s): 'tlsconfig.ClientTLS' has invalid keys: rootcertfile
//
// which covers every renamed field of the server and client templates (clientAuthRequired,
// serverHostOverride, rootCert, rootCertFile, serverRootCAs) and every per-endpoint key of a
// Fabric network or Fabric-x service (tlsEnabled, tlsDisabled, tlsClientSideAuth,
// tlsRootCertFile, rootCerts, and the flat clientKey/clientCert) — those sit inside array
// elements, where the decode rejects them as an unknown field or as a string where a map is
// expected.
//
// What is left is the keys that sit OUTSIDE every tls: subtree, which no decode ever sees.
// The full migration table, including the renames handled by the strict decode, is in
// docs/configuration.md.

// removedNode holds absolute keys, matched under the prefix a caller passes. Callers narrow
// with a prefix ("fsc", "fsc.p2p"); the keys themselves are fully qualified.
var removedNode = map[string]string{
	// Never meant transport TLS: it gated whether scraping /metrics required a client
	// certificate. Read through no struct, so nothing else would catch it.
	"fsc.metrics.prometheus.tls": "fsc.metrics.clientAuthRequired",
}

// removedNetwork holds keys RELATIVE to one Fabric network, checked by
// [CheckRemovedNetworkKeys] with a "fabric.<network>." prefix. A network's name is only known
// at runtime, so these cannot be stored fully qualified.
//
// The ordering.* pair shadowed the network block for orderer connections alone, so the same
// two settings had two homes and the narrower one silently won. The network block now applies
// to every connection, orderers included — which makes a leftover ordering.tlsEnabled: false
// a silent switch from plaintext to TLS, the one entry in this migration whose loss changes
// behaviour rather than just failing to parse. ordering.* is read key by key with GetInt and
// IsSet, never decoded into a struct, so only this table can catch it.
var removedNetwork = map[string]string{
	"ordering.tlsenabled":            "tls.enabled",
	"ordering.tlsclientauthrequired": "tls.clientAuthEnabled",
}
