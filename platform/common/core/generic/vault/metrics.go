/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
)

type Metrics struct {
	CommitDuration        metrics.Histogram
	BatchedCommitDuration metrics.Histogram
}

func NewMetrics(m metrics.Provider) *Metrics {
	return &Metrics{
		CommitDuration: m.NewHistogram(metrics.HistogramOpts{
			Name:    "commit",
			Help:    "Histogram for the duration of commit",
			Buckets: utils.ExponentialBucketTimeRange(0, 5*time.Second, 15),
		}),
		BatchedCommitDuration: m.NewHistogram(metrics.HistogramOpts{
			Name:    "batched_commit",
			Help:    "Histogram for the duration of commit with the batching overhead",
			Buckets: utils.ExponentialBucketTimeRange(0, 5*time.Second, 15),
		}),
	}
}
