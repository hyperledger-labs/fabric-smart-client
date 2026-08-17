/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package events

import (
	"context"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/goleak"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
)

type blockingDelivery struct{ started chan struct{} }

func (d *blockingDelivery) ScanBlock(ctx context.Context, _ fabric.BlockCallback) error {
	close(d.started)
	<-ctx.Done() // block until the manager context is cancelled
	return ctx.Err()
}

type nopQuery struct{}

func (nopQuery) QueryByID(ctx context.Context, _ driver.BlockNum, _ map[EventID][]ListenerEntry[testEvent]) (<-chan []testEvent, error) {
	ch := make(chan []testEvent)
	close(ch)
	return ch, nil
}

type testMapper struct{}

func (testMapper) MapTxData(context.Context, []byte, *common.BlockMetadata, driver.BlockNum, driver.TxNum) (map[driver.Namespace]testEvent, error) {
	return nil, nil
}
func (testMapper) MapProcessedTx(*fabric.ProcessedTransaction) ([]testEvent, error) { return nil, nil }

func TestListenerManager_StopsOnContextCancel(t *testing.T) { //nolint:paralleltest // uses goleak.VerifyNone; must run serially
	defer goleak.VerifyNone(t)

	ctx, cancel := context.WithCancel(context.Background())
	del := &blockingDelivery{started: make(chan struct{})}
	cfg := DeliveryListenerManagerConfig{ListenerTimeout: time.Hour}

	_, err := NewListenerManager[testEvent](ctx, logging.MustGetLogger(), cfg, del, nopQuery{}, &noop.Tracer{}, testMapper{})
	require.NoError(t, err)

	<-del.started // ensure the start goroutine is running
	cancel()      // must unwind the start goroutine and any cache goroutine
}
