/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"

	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	api2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

type Network interface {
	TransactionManager() driver.TransactionManager
	EnvelopeService() driver.EnvelopeService
}

type Vault struct {
	*vault.Vault
	network    Network
	vaultStore lastTxGetter
}

type lastTxGetter interface {
	GetLast(ctx context.Context) (*driver3.TxStatus, error)
}

func NewVault(network Network, vaultStore driver2.VaultStore, metricsProvider metrics.Provider, tracerProvider trace.TracerProvider) (*Vault, error) {
	return &Vault{
		Vault:      vault.New(vaultStore, metricsProvider, tracerProvider),
		network:    network,
		vaultStore: vaultStore,
	}, nil
}

func (v *Vault) GetLastTxID(ctx context.Context) (string, error) {
	tx, err := v.vaultStore.GetLast(ctx)
	if err != nil {
		return "", err
	}
	return tx.TxID, nil
}

func (v *Vault) NewRWSet(ctx context.Context, txID driver3.TxID) (api2.RWSet, error) {
	return v.Vault.NewRWSet(ctx, txID)
}

func (v *Vault) NewRWSetFromBytes(ctx context.Context, id string, results []byte) (driver.RWSet, error) {
	return v.Vault.NewRWSetFromBytes(ctx, id, results)
}

func (v *Vault) Status(ctx context.Context, txID driver3.TxID) (driver.ValidationCode, string, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("start_status")
	defer span.AddEvent("end_status")
	vc, message, err := v.Vault.Status(ctx, txID)
	if err != nil {
		return driver.Unknown, "", err
	}
	// if vc == driver.Busy {
	//	return vc, "", nil
	// }
	if vc != driver.Unknown {
		return vc, message, nil
	}

	// give it a second chance
	if v.network.EnvelopeService().Exists(txID) {
		if err := v.extractStoredEnvelopeToVault(txID); err != nil {
			return driver.Unknown, "", errors.WithMessagef(err, "failed to extract stored enveloper for [%s]", txID)
		}
		return driver.Busy, message, nil
	}

	return driver.Unknown, message, nil
}

func (v *Vault) Statuses(ctx context.Context, ids ...driver3.TxID) ([]driver.TxValidationStatus, error) {
	statuses := make([]driver.TxValidationStatus, len(ids))
	for i, id := range ids {
		vc, message, err := v.Status(ctx, id)
		if err != nil {
			return nil, err
		}
		statuses[i] = driver.TxValidationStatus{
			TxID:           id,
			ValidationCode: vc,
			Message:        message,
		}
	}
	return statuses, nil
}

func (v *Vault) DiscardTx(ctx context.Context, txID driver3.TxID, message string) error {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("start_discard_tx")
	defer span.AddEvent("end_discard_tx")

	vc, _, err := v.Vault.Status(ctx, txID)
	if err != nil {
		return errors.Wrapf(err, "failed getting tx's status in state db [%s]", txID)
	}
	if vc != driver.Unknown {
		logger.Debugf("discarding transaction [%s], tx is known", txID)
		return v.Vault.DiscardTx(ctx, txID, message)
	}
	logger.Debugf("discarding transaction [%s], tx is unknown, set status to invalid", txID)
	if err := v.SetDiscarded(ctx, txID, message); err != nil {
		logger.Errorf("failed setting tx discarded [%s] in vault: %s", txID, err)
	}
	return nil
}

func (v *Vault) extractStoredEnvelopeToVault(txID string) error {
	// extract envelope
	envRaw, err := v.network.EnvelopeService().LoadEnvelope(txID)
	if err != nil {
		return errors.WithMessagef(err, "failed to load fabric envelope for [%s]", txID)
	}
	logger.Debugf("unmarshal envelope [%s]", txID)
	env := v.network.TransactionManager().NewEnvelope()
	if err := env.FromBytes(envRaw); err != nil {
		return errors.Wrapf(err, "cannot unmarshal envelope [%s]", txID)
	}
	rws, err := v.NewRWSetFromBytes(context.Background(), txID, env.Results())
	if err != nil {
		return errors.Wrapf(err, "cannot unmarshal envelope [%s]", txID)
	}
	rws.Done()

	return nil
}
