/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	"context"
	"strings"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"go.opentelemetry.io/otel/trace"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

var logger = logging.MustGetLogger()

// Loader reconstructs read-write sets of fabricx transactions from stored envelopes and
// endorser transactions. Fabricx envelopes are always of type HeaderType_MESSAGE and the
// loader decodes them itself; it does not dispatch to pluggable payload handlers.
type Loader struct {
	Network            string
	Channel            string
	EnvelopeService    driver.EnvelopeService
	TransactionService driver.EndorserTransactionService
	TransactionManager driver.TransactionManager

	Vault driver.RWSetInspector
}

func NewLoader(
	network string,
	channel string,
	envelopeService driver.EnvelopeService,
	transactionService driver.EndorserTransactionService,
	transactionManager driver.TransactionManager,
	vault driver.RWSetInspector,
) driver.RWSetLoader {
	return &Loader{
		Network:            network,
		Channel:            channel,
		EnvelopeService:    envelopeService,
		TransactionService: transactionService,
		TransactionManager: transactionManager,
		Vault:              vault,
	}
}

// AddHandlerProvider is a no-op that always returns nil: the loader decodes fabricx
// envelopes itself. It exists to satisfy driver.RWSetLoader, whose callers register the
// same providers on every channel's loader at startup and fail on an error.
func (*Loader) AddHandlerProvider(cb.HeaderType, driver.RWSetPayloadHandlerProvider) error {
	return nil
}

func (c *Loader) GetRWSetFromEvn(ctx context.Context, txID driver2.TxID) (driver.RWSet, driver.ProcessTransaction, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("start_get_rwset_from_evn")
	defer span.AddEvent("end_get_rwset_from_evn")

	if !c.EnvelopeService.Exists(ctx, txID) {
		return nil, nil, errors.Errorf("envelope does not exists for [txID=%s]", txID)
	}

	rawEnv, err := c.EnvelopeService.LoadEnvelope(ctx, txID)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "load envelope [txID=%s]", txID)
	}

	payl, chdr, err := c.unmarshalEnvelope(txID, rawEnv)
	if err != nil {
		return nil, nil, errors.Wrap(err, "unmarshal payload and channel header")
	}

	rws, err := c.Vault.NewRWSetFromBytes(ctx, chdr.TxId, payl.Data)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "create new rws for [txID=%s]", chdr.TxId)
	}

	var function string
	if anyKeyContains(rws, "initialized") {
		function = "init"
	}

	logger.Debugf("retrieved processed transaction from envelope [txID=%s] [function=%s]", txID, function)
	pt := &processedTransaction{
		network:  c.Network,
		channel:  chdr.ChannelId,
		id:       chdr.TxId,
		function: function,
	}

	return rws, pt, nil
}

func (c *Loader) GetRWSetFromETx(ctx context.Context, txID driver2.TxID) (driver.RWSet, driver.ProcessTransaction, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("start_get_rwset_from_etx")
	defer span.AddEvent("end_get_rwset_from_etx")

	if !c.TransactionService.Exists(ctx, txID) {
		return nil, nil, errors.Errorf("transaction does not exists for [txID=%s]", txID)
	}

	raw, err := c.TransactionService.LoadTransaction(ctx, txID)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cannot load etx [txID=%s]", txID)
	}

	tx, err := c.TransactionManager.NewTransactionFromBytes(ctx, c.Channel, raw)
	if err != nil {
		return nil, nil, errors.Wrap(err, "new transaction from bytes")
	}

	rws, err := tx.GetRWSet()
	if err != nil {
		return nil, nil, errors.Wrap(err, "get rwset")
	}

	return rws, tx, nil
}

func (c *Loader) GetInspectingRWSetFromEvn(ctx context.Context, txID driver2.TxID, envelopeRaw []byte) (driver.RWSet, driver.ProcessTransaction, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("start_get_inspecting_rwset_from_evn")
	defer span.AddEvent("end_get_inspecting_rwset_from_evn")

	logger.Debugf("retrieve rwset from envelope [channel=%s] [txID=%s]", c.Channel, txID)

	payl, chdr, err := c.unmarshalEnvelope(txID, envelopeRaw)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cannot unmarshal envelope [txID=%s]", txID)
	}

	rws, err := c.Vault.InspectRWSet(ctx, payl.Data)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cannot inspect rwset for [txID=%s]", txID)
	}

	var function string
	if anyKeyContains(rws, "initialized") {
		function = "init"
	}

	logger.Debugf("retrieved inspecting processed transaction from env [txID=%s] [function=%s]", txID, function)
	pt := &processedTransaction{
		network:  c.Network,
		channel:  chdr.ChannelId,
		id:       chdr.TxId,
		function: function,
	}

	return rws, pt, nil
}

// unmarshalEnvelope parses raw and checks that it is a fabricx transaction envelope for
// txID on the loader's channel.
func (c *Loader) unmarshalEnvelope(txID driver2.TxID, raw []byte) (*cb.Payload, *cb.ChannelHeader, error) {
	_, payl, chdr, err := fabricutils.UnmarshalTx(raw)
	if err != nil {
		return nil, nil, err
	}
	if cb.HeaderType(chdr.Type) != cb.HeaderType_MESSAGE {
		return nil, nil, errors.Errorf("unsupported header type %v, expected %v", cb.HeaderType(chdr.Type), cb.HeaderType_MESSAGE)
	}
	if txID != chdr.TxId {
		return nil, nil, errors.Errorf("txID mismatch in channel header, expected=%s, actual=%s", txID, chdr.TxId)
	}
	if c.Channel != chdr.ChannelId {
		return nil, nil, errors.Errorf("channel mismatch in channel header, expected=%s, actual=%s", c.Channel, chdr.ChannelId)
	}
	return payl, chdr, nil
}

func anyKeyContains(rws driver.RWSet, substr string) bool {
	for _, ns := range rws.Namespaces() {
		for pos := range rws.NumReads(ns) {
			if k, err := rws.GetReadKeyAt(ns, pos); err == nil && strings.Contains(k, substr) {
				return true
			}
		}
	}
	return false
}

type processedTransaction struct {
	network  string
	channel  string
	id       string
	function string
	params   []string
}

func (pt *processedTransaction) Network() string {
	return pt.network
}

func (pt *processedTransaction) Channel() string {
	return pt.channel
}

func (pt *processedTransaction) ID() string {
	return pt.id
}

func (pt *processedTransaction) FunctionAndParameters() (string, []string) {
	return pt.function, pt.params
}
