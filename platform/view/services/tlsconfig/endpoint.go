/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tlsconfig

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

// ResolveEndpointClient resolves one endpoint's client-side TLS, inheriting per field from the
// network-level block at parentKey. Pass the endpoint's own tls map as child, or nil when it has
// none, in which case the result is the network block's.
//
// The child arrives as a map rather than a key because an array element has no addressable key:
// koanf flattens nested maps but not slice elements, so "orderers.0.tls" does not exist.
func ResolveEndpointClient(src Source, parentKey string, child map[string]any) (grpc.SecureOptions, error) {
	parentRaw, _ := src.RawSubtree(parentKey)
	parent, err := decode[ClientTLS](parentKey, parentRaw)
	if err != nil {
		return grpc.SecureOptions{}, err
	}
	own, err := decode[ClientTLS](parentKey+"[].tls", child)
	if err != nil {
		return grpc.SecureOptions{}, err
	}
	return buildClient(src, parentKey, mergeClient(parent, own), nil, nil)
}

// ArraySource is a [Source] whose configuration also holds arrays of maps. Only the two
// platforms with endpoint arrays satisfy it; a surface that reads a single block does not
// have to carry the method.
type ArraySource interface {
	Source
	// RawSubtrees returns the raw maps at key when it holds an array of maps, as a Fabric
	// network's orderers and peers do.
	RawSubtrees(key string) []map[string]any
}

// ResolveEndpointClients resolves the client-side TLS of the n endpoints configured in the
// array at arrayKey, each inheriting per field from the block at parentKey.
//
// Endpoints are matched to their configuration by position, since an array element has no
// addressable key.
//
// No raw entries at all means the source does not expose arrays — every endpoint then simply
// inherits parentKey, which is a valid configuration. But a non-empty array shorter than the
// decoded endpoints means the two reads of the same array disagree, and continuing would hand
// the trailing endpoints the network block in place of their own, stricter one: a silently
// weakened connection. That is an error. Surplus raw entries cannot misalign the ones used.
func ResolveEndpointClients(src ArraySource, parentKey, arrayKey string, n int) ([]grpc.SecureOptions, error) {
	raw := src.RawSubtrees(arrayKey)
	if len(raw) > 0 && len(raw) < n {
		return nil, errors.Errorf(
			"[%s] holds %d configured entries but %d endpoints were decoded; the raw and "+
				"decoded reads of the same array disagree", arrayKey, len(raw), n)
	}
	out := make([]grpc.SecureOptions, n)
	for i := range n {
		var own map[string]any
		if i < len(raw) {
			own, _ = raw[i]["tls"].(map[string]any)
		}
		resolved, err := ResolveEndpointClient(src, parentKey, own)
		if err != nil {
			return nil, errors.WithMessagef(err, "invalid TLS configuration for %s[%d]", arrayKey, i)
		}
		out[i] = resolved
	}
	return out, nil
}
