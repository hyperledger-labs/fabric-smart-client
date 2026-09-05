/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tlsconfig

// removed maps a no-longer-supported configuration key to the key that replaces it. Keys are
// stored lowercased, matching what the configuration backend holds.
//
// Grouped by the phase that removed each key, purely as a reading aid. The phases were split
// across one file each while they were being implemented in parallel, so four branches could
// append without conflicting; that is over, and one table is easier to read than four.
//
// Note what is NOT here: the per-endpoint keys of a Fabric network or a Fabric-x service —
// tlsEnabled, tlsDisabled, tlsClientSideAuth, tlsRootCertFile, rootCerts and the flat
// clientKey/clientCert. Those sit inside array elements, where the strict subtree decode
// rejects them directly, either as an unknown field or as a string where a map is expected.
var removed = map[string]string{
	// Phase 1 — the node's own listeners.
	"fsc.p2p.opts.websocket.tls.serverrootcas": "fsc.p2p.opts.websocket.tls.rootCAs.files",

	// Phase 2 — metrics. This never meant transport TLS: it gated whether scraping /metrics
	// required a client certificate.
	"fsc.metrics.prometheus.tls": "fsc.metrics.clientAuthRequired",

	// Phase 3 — a Fabric network's client side. Relative to the network's prefix, so
	// CheckRemovedKeys is called with "fabric.<network>." for each network.
	//
	// clientAuthRequired was live client configuration that reached RequireClientCert on the
	// dialling side, misnamed as a server-side field: a rename, not a removal.
	"tls.clientauthrequired": "tls.clientAuthEnabled",
	"tls.serverhostoverride": "tls.serverNameOverride",
	// Both named a single CA file; the client template takes a list.
	"tls.rootcert":     "tls.rootCAs.files",
	"tls.rootcertfile": "tls.rootCAs.files",
	// The ordering.* pair shadowed the network block for orderer connections alone, so the
	// same two settings had two homes and the narrower one silently won. The network block
	// now applies to every connection, orderers included.
	"ordering.tlsenabled":            "tls.enabled",
	"ordering.tlsclientauthrequired": "tls.clientAuthEnabled",
}
