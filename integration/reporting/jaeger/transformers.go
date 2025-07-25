/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package jaeger

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/slices"
	v1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

func LargestEventGap(t *v1.TracesData) (time.Duration, error) {
	events := slices.SortedSlice[uint64]{}
	if t == nil {
		return 0, nil
	}
	for _, rs := range t.ResourceSpans {
		for _, ss := range rs.ScopeSpans {
			for _, s := range ss.Spans {
				for _, e := range s.Events {
					events.Add(e.TimeUnixNano)
				}
			}
		}
	}
	if len(events) <= 1 {
		return 0, nil
	}

	maxGap := uint64(0)
	for i := range events[1:] {
		if gap := events[i+1] - events[i]; gap > maxGap {
			maxGap = gap
		}
	}
	return time.Duration(maxGap), nil
}
