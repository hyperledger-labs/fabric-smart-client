/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/orion-sdk-go/pkg/bcdb"
	"github.com/hyperledger-labs/orion-server/pkg/types"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

var logger = flogging.MustGetLogger("orion-sdk.delivery")

var (
	ErrComm = errors.New("communication issue")
)

type Callback func(block *types.AugmentedBlockHeader) (bool, error)

// Vault models a key-value store that can be updated by committing rwsets
type Vault interface {
	// GetLastTxID returns the last transaction id committed
	GetLastTxID() (string, error)
}

type Network interface {
	SessionManager() driver.SessionManager
	IdentityManager() driver.IdentityManager
	Name() string
}

type DeliverStream interface {
	// Receive returns
	//    - *types.BlockHeader if IncludeTxIDs is set to false in the delivery config
	//    - *types.AugmentedBlockHeader if IncludeTxIDs is set to true in the delivery config
	//    - nil if service has been stopped either by the caller or due to an error
	Receive() interface{}
	// Stop stops the delivery service
	Stop()
	// Error returns any accumulated error
	Error() error
}

type delivery struct {
	sp                  view2.ServiceProvider
	network             Network
	waitForEventTimeout time.Duration
	callback            Callback
	vault               Vault
	me                  string
	networkName         string
	stop                chan bool
}

func New(
	sp view2.ServiceProvider,
	network Network,
	callback Callback,
	vault Vault,
	waitForEventTimeout time.Duration,
) (*delivery, error) {
	d := &delivery{
		sp:                  sp,
		network:             network,
		waitForEventTimeout: waitForEventTimeout,
		callback:            callback,
		vault:               vault,
		me:                  network.IdentityManager().Me(),
		networkName:         network.Name(),
		stop:                make(chan bool),
	}
	return d, nil
}

// StartDelivery runs the delivery service in a goroutine
func (d *delivery) StartDelivery(ctx context.Context) error {
	go d.Run(ctx)
	return nil
}

func (d *delivery) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var df DeliverStream
	var err error
	for {
		select {
		case <-d.stop:
			// Time to stop
			return nil
		case <-ctx.Done():
			// Time to cancel
			return errors.New("context done")
		default:
			if df == nil {
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("deliver service [%s:%s], connecting...", d.networkName, d.me)
				}
				df, err = d.connect()
				if err != nil {
					logger.Errorf("failed connecting to delivery service [%s:%s] [%s]. Wait 10 sec before reconnecting", d.networkName, d.me, err)
					time.Sleep(10 * time.Second)
					if logger.IsEnabledFor(zapcore.DebugLevel) {
						logger.Debugf("reconnecting to delivery service [%s:%s]", d.networkName, d.me)
					}
					continue
				}
			}

			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("deliver service [%s:%s], receiving...", d.networkName, d.me)
			}
			resp := df.Receive()
			if resp == nil {
				df = nil
				logger.Errorf("delivery service [%s:%s], failed receiving response ", d.networkName, d.me,
					errors.WithMessagef(err, "error receiving deliver response from orion"))
				continue
			}

			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("deliver service [%s:%s], received response [%+v]", d.networkName, d.me, resp)
			}
			switch r := resp.(type) {
			case *types.AugmentedBlockHeader:
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("delivery service [%s:%s], commit block [%d]", d.networkName, d.me, r.Header.BaseHeader.Number)
				}

				stop, err := d.callback(r)
				if err != nil {
					switch errors.Cause(err) {
					case ErrComm:
						logger.Errorf("error occurred when processing block [%s], retry", err)
						// retry
						time.Sleep(10 * time.Second)
						df = nil
					default:
						// Stop here
						logger.Errorf("error occurred when processing block [%s]", err)
						return err
					}
				}
				if stop {
					return nil
				}
			default:
				df = nil
				logger.Errorf("delivery service [%s:%s], got [%s]", d.networkName, d.me, r)
			}
		}
	}
}

func (d *delivery) Stop() {
	d.stop <- true
}

func (d *delivery) connect() (DeliverStream, error) {
	session, err := d.network.SessionManager().NewSession(d.me)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to create session with identity [%s]", d.me)
	}
	ledger, err := session.Ledger()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get ledger from session")
	}
	conf := &bcdb.BlockHeaderDeliveryConfig{
		StartBlockNumber: 1,
		RetryInterval:    1 * time.Second,
		Capacity:         5,
		IncludeTxIDs:     true,
	}
	return ledger.NewBlockHeaderDeliveryService(conf), nil
}
