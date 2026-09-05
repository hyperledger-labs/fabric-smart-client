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

// CheckRemovedKeys returns an error naming every no-longer-supported configuration key
// present under prefix, together with the key that replaces it. Removed keys are rejected
// rather than translated, so a configuration that no longer means what it says fails at
// startup instead of silently weakening transport security.
//
// Only keys under prefix are considered; pass "fsc" to check this node's listeners, or a
// network prefix to check one Fabric network. It returns nil when no removed key is present.
func CheckRemovedKeys(src Source, prefix string) error {
	var errs []error
	lower := strings.ToLower(prefix)
	for gone, replacement := range removed {
		if !strings.HasPrefix(gone, lower) {
			continue
		}
		// IsSet, not RawSubtree: most removed keys are leaf values, and RawSubtree reports
		// only subtrees. Using it here silently passed every leaf in the migration table.
		if src.IsSet(gone) {
			errs = append(errs, errors.Errorf(
				"configuration key [%s] has been removed; use [%s] instead", gone, replacement))
		}
	}
	return errors.Join(errs...)
}
