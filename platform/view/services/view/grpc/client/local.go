/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"context"
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

const (
	fidLabel tracing.LabelName = "fid"
)

type LocalClient struct {
	serviceLocator services.Provider
	tracer         trace.Tracer
}

func NewLocalClient(registry services.Provider) *LocalClient {
	return &LocalClient{
		serviceLocator: registry,
	}
}

func (n *LocalClient) CallView(fid string, in []byte) (interface{}, error) {
	tracer, err := n.getTracer()
	if err != nil {
		return nil, err
	}
	ctx, span := tracer.Start(context.Background(), "CallView",
		trace.WithSpanKind(trace.SpanKindClient),
		tracing.WithAttributes(tracing.String(fidLabel, fid)))
	defer span.End()
	manager, err := view.GetManager(n.serviceLocator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting view manager [%s]", fid)
	}
	span.AddEvent("start_new_view")
	f, err := manager.NewView(fid, in)
	span.AddEvent("end_new_view")
	if err != nil {
		return nil, errors.Wrapf(err, "failed instantiating view [%s]", fid)
	}
	span.AddEvent("start_initiate_view")
	result, err := manager.InitiateView(f, ctx)
	span.AddEvent("end_initiate_view")
	if err != nil {
		return nil, errors.Wrapf(err, "failed running view [%s]", fid)
	}
	raw, ok := result.([]byte)
	if !ok {
		raw, err = json.Marshal(result)
		if err != nil {
			return nil, errors.Wrapf(err, "failed marshalling result produced by view %s", fid)
		}
	}
	return raw, nil
}

func (n *LocalClient) getTracer() (trace.Tracer, error) {
	if n.tracer == nil {
		tracingProvider, err := tracing.GetProvider(n.serviceLocator)
		if err != nil {
			return nil, err
		}
		n.tracer = tracingProvider.Tracer("node_view_client", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace:  "viewsdk",
			LabelNames: []tracing.LabelName{fidLabel},
		}))
	}
	return n.tracer, nil
}
