/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tlsconfig

import "cmp"

// mergeServer and mergeClient take, for each field, the child's value when the child set one
// and the parent's otherwise. cmp.Or does exactly that for a pointer field: the first
// non-nil wins.
//
// Safe to carry the pointers over as-is: both structs come from a decode, and mapstructure
// allocates fresh storage on every decode, so nothing here aliases the provider's config
// map. Merging the *raw* maps would alias it — koanf/maps.Merge retains references into its
// source — which is why this merges decoded structs instead.

func mergeServer(parent, child ServerTLS) ServerTLS {
	return ServerTLS{
		Enabled:            cmp.Or(child.Enabled, parent.Enabled),
		Cert:               cmp.Or(child.Cert, parent.Cert),
		Key:                cmp.Or(child.Key, parent.Key),
		ClientAuthRequired: cmp.Or(child.ClientAuthRequired, parent.ClientAuthRequired),
		ClientRootCAs:      cmp.Or(child.ClientRootCAs, parent.ClientRootCAs),
	}
}

func mergeClient(parent, child ClientTLS) ClientTLS {
	return ClientTLS{
		Enabled:            cmp.Or(child.Enabled, parent.Enabled),
		RootCAs:            cmp.Or(child.RootCAs, parent.RootCAs),
		ClientAuthEnabled:  cmp.Or(child.ClientAuthEnabled, parent.ClientAuthEnabled),
		ClientCert:         cmp.Or(child.ClientCert, parent.ClientCert),
		ClientKey:          cmp.Or(child.ClientKey, parent.ClientKey),
		ServerNameOverride: cmp.Or(child.ServerNameOverride, parent.ServerNameOverride),
	}
}
