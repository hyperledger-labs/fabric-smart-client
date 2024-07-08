/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	errors2 "github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

const (
	txIdLabel tracing.LabelName = "tx_id"
)

type Committer interface {
	// IsFinal takes in input a transaction id and waits for its confirmation.
	IsFinal(ctx context.Context, txID string) error
}

type Network interface {
	SessionManager() driver.SessionManager
	IdentityManager() driver.IdentityManager
}

type finality struct {
	committer Committer
	network   Network
	tracer    trace.Tracer
}

func NewService(committer Committer, network Network, tracerProvider trace.TracerProvider) (*finality, error) {
	return &finality{
		committer: committer,
		network:   network,
		tracer: tracerProvider.Tracer("finality", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace:  "orionsdk",
			LabelNames: []tracing.LabelName{txIdLabel},
		})),
	}, nil
}

func (f *finality) IsFinal(ctx context.Context, txID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	newCtx, span := f.tracer.Start(ctx, "finality_check", tracing.WithAttributes(tracing.String(txIdLabel, txID)))
	defer span.End()

	// ask the committer first
	err := f.committer.IsFinal(newCtx, txID)
	if err == nil {
		return nil
	}

	// if the transaction is unknown, then check the custodian
	var err2 error
	if errors.HasCause(err, committer.ErrUnknownTX) {
		// ask the ledger
		span.AddEvent("open_session")
		s, err2 := f.network.SessionManager().NewSession(f.network.IdentityManager().Me())
		if err2 == nil {
			l, err2 := s.Ledger()
			if err2 == nil {
				span.AddEvent("get_tx_receipt")
				flag, err2 := l.GetTransactionReceipt(txID)
				if err2 == nil {
					if flag == driver.VALID {
						return nil
					}
					return errors2.Errorf("invalid transaction [%s]", txID)
				}
			}
		}
	}
	return errors2.Errorf("failed retrieveing transaction finality for [%s]: [%s][%s]", txID, err, err2)
}
