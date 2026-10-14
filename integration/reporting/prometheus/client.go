/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package prometheus

import (
	"context"
	"fmt"
	"time"

	prom_api "github.com/prometheus/client_golang/api"
	prom_v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring/monitoring"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

const (
	// ViewCallsOperationsMetric is the counter of view calls run by an FSC node.
	ViewCallsOperationsMetric = "fsc_view_services_view_calls_operations"
)

// Reporter queries a Prometheus server for the metrics scraped from FSC nodes.
type Reporter interface {
	// GetVector runs an instant query for the series matching metric and returns
	// their current samples. It fails if the server returns warnings or a result
	// that is not a vector.
	GetVector(metric model.Metric) (model.Vector, error)
	// GetViewOperations returns the number of calls of viewName on nodeName, summed
	// across all series. An empty nodeName or viewName matches any value.
	GetViewOperations(nodeName, viewName string) (int, error)
}

type reporter struct {
	api prom_v1.API
}

// NewLocalReporter returns a Reporter for a Prometheus server on the local host
// at the default port monitoring.PrometheusPort.
func NewLocalReporter() (*reporter, error) {
	return NewReporter(fmt.Sprintf("http://0.0.0.0:%d", monitoring.PrometheusPort))
}

// NewReporter returns a Reporter for the Prometheus server at address.
func NewReporter(address string) (*reporter, error) {
	promClient, err := prom_api.NewClient(prom_api.Config{Address: address})
	if err != nil {
		return nil, err
	}
	return &reporter{api: prom_v1.NewAPI(promClient)}, nil
}

func (c *reporter) GetVector(metric model.Metric) (model.Vector, error) {
	val, warnings, _, err := c.api.Query(context.Background(), metric.String(), time.Now())
	if err != nil {
		return nil, err
	}
	if len(warnings) > 0 {
		return nil, errors.New("expected no warnings")
	}
	if val.Type() != model.ValVector {
		return nil, errors.New("expected val vector")
	}

	vector, ok := val.(model.Vector)
	if !ok {
		return nil, errors.New("expected vector")
	}
	return vector, nil
}

func (c *reporter) GetViewOperations(nodeName, viewName string) (int, error) {
	m := model.Metric{"__name__": ViewCallsOperationsMetric}
	if len(nodeName) > 0 {
		m["job"] = model.LabelValue(fmt.Sprintf("FSC Node %s", nodeName))
	}
	if len(viewName) > 0 {
		m["view"] = model.LabelValue(viewName)
	}
	vector, err := c.GetVector(m)
	if err != nil {
		return 0, err
	}
	// One series per distinct value of the other labels (success, initiator_view, ...).
	var total model.SampleValue
	for _, s := range vector {
		total += s.Value
	}
	return int(total), nil
}
