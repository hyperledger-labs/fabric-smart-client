/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tlsconfig

import (
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

var logger = logging.MustGetLogger()

func validateServer(key string, so grpc.SecureOptions, dynamicClientRootCAs bool) error {
	if !so.UseTLS {
		// A disabled block carrying a keypair is a mistake worth naming. Client root CAs
		// alone are not: clientRootCAs without clientAuthRequired is the supported
		// "verify if offered" state.
		if len(so.Certificate) > 0 || len(so.Key) > 0 {
			logger.Warnf("[%s] TLS is disabled but a certificate or key is configured", key)
		}
		return nil
	}
	if len(so.Certificate) == 0 || len(so.Key) == 0 {
		return errors.Errorf("[%s] tls.enabled is true but cert.file or key.file is missing", key)
	}
	if so.RequireClientCert && len(so.ClientRootCAs) == 0 && !dynamicClientRootCAs {
		// Bug #1111, stated once, for every surface whose pool is static.
		return errors.Errorf("[%s] clientAuthRequired is true but clientRootCAs.files is "+
			"empty; no client certificate could ever verify", key)
	}
	return nil
}

func validateClient(key string, so grpc.SecureOptions, cert, keyFile *File) error {
	// Exactly one half of the keypair is always a mistake, whether or not clientAuthEnabled
	// ends up true. Checked even when TLS is off, because it is a typo either way.
	certSet := cert != nil && cert.File != ""
	keySet := keyFile != nil && keyFile.File != ""
	if certSet != keySet {
		return errors.Errorf("[%s] exactly one of clientCert.file and clientKey.file is "+
			"set; set both or neither", key)
	}
	if !so.UseTLS {
		return nil
	}
	if so.RequireClientCert && (len(so.Certificate) == 0 || len(so.Key) == 0) {
		return errors.Errorf("[%s] clientAuthEnabled is true but clientCert.file or "+
			"clientKey.file is missing", key)
	}
	return nil
}

// CheckRemovedKeys returns an error naming every no-longer-supported key of the NODE's own
// configuration present under prefix, together with the key that replaces it. Removed keys are
// rejected rather than translated, so a configuration that no longer means what it says fails
// at startup instead of silently weakening transport security.
//
// The keys in [removedNode] are absolute, so prefix narrows the search: pass "fsc" for the
// whole node, or something longer for one subtree. A Fabric network's keys are relative to
// their network and are checked by [CheckRemovedNetworkKeys] instead — the two cannot share
// one loop, because one filters absolute keys while the other qualifies relative ones.
//
// It returns nil when no removed key is present.
func CheckRemovedKeys(src Source, prefix string) error {
	lower := strings.ToLower(prefix)
	var errs []error
	for gone, replacement := range removedNode {
		if !strings.HasPrefix(gone, lower) {
			continue
		}
		if err := removedKeyError(src, gone, replacement); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// CheckRemovedNetworkKeys returns an error naming every no-longer-supported key of ONE Fabric
// network present in the configuration, together with the key that replaces it.
//
// prefix is that network's key prefix, ending in a dot: "fabric.mynet." or "fabric." for the
// default network. The entries in [removedNetwork] are relative to it, so the prefix is
// PREPENDED to reach the configured key — it is not a filter. Getting that backwards makes
// every entry unreachable, since no relative key starts with "fabric.".
func CheckRemovedNetworkKeys(src Source, prefix string) error {
	var errs []error
	for gone, replacement := range removedNetwork {
		if err := removedKeyError(src, prefix+gone, prefix+replacement); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// removedKeyError reports key as removed when it is set. It checks with IsSet, not RawSubtree:
// every key in both tables is a leaf value, and RawSubtree reports only subtrees — using it
// here silently passed the whole migration table once already.
func removedKeyError(src Source, key, replacement string) error {
	if !src.IsSet(key) {
		return nil
	}
	return errors.Errorf("configuration key [%s] has been removed; use [%s] instead",
		key, replacement)
}
