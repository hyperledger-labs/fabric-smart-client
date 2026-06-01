/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"go.yaml.in/yaml/v3"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// MergeYAML deep-merges multiple YAML documents left to right and returns the result as YAML.
// Maps are merged recursively; for all other types (scalars, arrays) the later document wins.
func MergeYAML(docs ...[]byte) ([]byte, error) {
	merged := map[string]any{}
	for _, doc := range docs {
		m := map[string]any{}
		if err := yaml.Unmarshal(doc, &m); err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal YAML document")
		}
		deepMergeMap(merged, m)
	}
	out, err := yaml.Marshal(merged)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal merged YAML")
	}
	return out, nil
}

func deepMergeMap(dst, src map[string]any) {
	for k, sv := range src {
		if dv, ok := dst[k]; ok {
			if dstMap, ok := dv.(map[string]any); ok {
				if srcMap, ok := sv.(map[string]any); ok {
					deepMergeMap(dstMap, srcMap)
					continue
				}
			}
		}
		dst[k] = sv
	}
}
