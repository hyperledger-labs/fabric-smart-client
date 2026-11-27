/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package chain builds, signs and broadcasts EIP-1559 transactions for the IOU demo, and polls for
// their receipts. It owns the submitter's nonce and gas policy; it has no opinion on what the
// transaction does (fund an account, deploy the contract, call create/update) - that is the
// caller's job, via the to/value/data it passes to Send.
package chain

import (
	"context"
	"math"
	"math/big"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/eip712"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// defaultGasMultiplier is applied to EstimateGas's result when GasMultiplier is below 1: the
// buffer absorbs state changes between estimation and execution, since EstimateGas returns the
// exact gas the transaction needed to run at the moment it was called.
const defaultGasMultiplier = 1.2

// defaultPollInterval is what WaitReceipt polls at when poll is zero or negative: time.NewTicker
// panics on a non-positive duration, and a misconfigured poll interval must not crash the demo.
const defaultPollInterval = time.Second

// ErrReverted is returned by WaitReceipt when the transaction was mined but its receipt reports
// status 0 (execution reverted).
var ErrReverted = errors.New("transaction reverted")

// ErrTimeout is returned by WaitReceipt when no receipt arrives within the given timeout.
var ErrTimeout = errors.New("timed out waiting for transaction receipt")

// Submitter builds, signs and broadcasts EIP-1559 transactions from one account. It keeps no state
// across calls to Send: the nonce comes fresh from the node every time, which is enough for a demo
// that submits one transaction at a time and waits for its receipt before sending the next.
type Submitter struct {
	Client        client.EVMClient
	Key           *secp256k1.PrivateKey
	ChainID       *big.Int
	GasMultiplier float64
}

// Send builds, signs and broadcasts an EIP-1559 transaction that calls to with value and data, or,
// when to is nil, deploys a contract with data as the creation code. value nil means zero.
//
// It fetches the sender's pending nonce, estimates gas and multiplies it by GasMultiplier
// (defaultGasMultiplier when GasMultiplier is below 1), fetches the node's suggested fees, signs
// and broadcasts, returning the transaction hash.
func (s *Submitter) Send(ctx context.Context, to *client.Address, value *big.Int, data []byte) (client.Hash, error) {
	from := eip712.PubKeyToAddress(s.Key.PubKey())

	nonce, err := s.Client.PendingNonceAt(ctx, from)
	if err != nil {
		return client.Hash{}, errors.Wrap(err, "chain: failed to get the pending nonce")
	}

	estimate, err := s.Client.EstimateGas(ctx, client.CallMsg{From: &from, To: to, Data: data, Value: value})
	if err != nil {
		return client.Hash{}, errors.Wrap(err, "chain: failed to estimate gas")
	}

	multiplier := s.GasMultiplier
	if multiplier < 1 {
		multiplier = defaultGasMultiplier
	}
	gas := uint64(math.Round(float64(estimate) * multiplier))

	fees, err := s.Client.SuggestGasFees(ctx)
	if err != nil {
		return client.Hash{}, errors.Wrap(err, "chain: failed to get the suggested gas fees")
	}

	tx := &client.DynamicFeeTx{
		ChainID:              s.ChainID,
		Nonce:                nonce,
		MaxPriorityFeePerGas: fees.MaxPriorityFeePerGas,
		MaxFeePerGas:         fees.MaxFeePerGas,
		Gas:                  gas,
		To:                   to,
		Value:                value,
		Data:                 data,
	}

	signed, err := client.SignTx(tx, s.Key)
	if err != nil {
		return client.Hash{}, errors.Wrap(err, "chain: failed to sign the transaction")
	}

	hash, err := s.Client.SendRawTransaction(ctx, signed)
	if err != nil {
		return client.Hash{}, errors.Wrap(err, "chain: failed to send the transaction")
	}

	return hash, nil
}

// WaitReceipt polls c for h's receipt every poll interval (defaultPollInterval when poll is zero
// or negative) until one is found, the transaction reverted, the timeout elapses, or ctx is done.
// A nil receipt with a nil error means pending.
func WaitReceipt(ctx context.Context, c client.EVMClient, h client.Hash, poll, timeout time.Duration) (*client.Receipt, error) {
	if poll <= 0 {
		poll = defaultPollInterval
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	for {
		receipt, err := c.GetTransactionReceipt(ctx, h)
		if err != nil {
			return nil, errors.Wrapf(err, "chain: failed to get the receipt for %s", h.Hex())
		}
		if receipt != nil {
			if receipt.Status == 0 {
				return nil, errors.Wrapf(ErrReverted, "transaction %s reverted", h.Hex())
			}

			return receipt, nil
		}
		if time.Now().After(deadline) {
			return nil, errors.Wrapf(ErrTimeout, "transaction %s", h.Hex())
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
