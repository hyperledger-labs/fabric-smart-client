/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package prometheus

import (
	"context"
	"fmt"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring/monitoring"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	prom_api "github.com/prometheus/client_golang/api"
	prom_v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

type Reporter interface {
	GetVector(metric model.Metric) (model.Vector, error)
	GetViewOperations(nodeName, viewName string) (int, error)
}

type reporter struct {
	api prom_v1.API
}

func NewLocalReporter() (*reporter, error) {
	return NewReporter(fmt.Sprintf("http://0.0.0.0:%d", monitoring.PrometheusPort))
}

func NewReporter(address string) (*reporter, error) {
	promClient, err := prom_api.NewClient(prom_api.Config{Address: address})
	if err != nil {
		return nil, err
	}
	return &reporter{api: prom_v1.NewAPI(promClient)}, nil
}

func (c *reporter) GetVector(metric model.Metric) (model.Vector, error) {
	val, warnings, err := c.api.Query(context.Background(), metric.String(), time.Now())
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
	m := model.Metric{"__name__": "fsc_view_operations"}
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
	if len(vector) == 0 {
		return 0, nil
	}
	if len(vector) != 1 {
		return 0, errors.New("expected vector len 1")
	}
	return int(vector[0].Value), nil
}
