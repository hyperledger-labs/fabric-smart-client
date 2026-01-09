/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"context"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
)

type BlockDispatcher struct {
	callbacks []driver.BlockCallback
	mu        sync.RWMutex
}

func (s *BlockDispatcher) AddCallback(f driver.BlockCallback) {
	s.mu.Lock()
	s.callbacks = append(s.callbacks, f)
	s.mu.Unlock()
}

func (s *BlockDispatcher) OnBlock(ctx context.Context, block *cb.Block) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var signalStop bool
	var errs error
	for _, callback := range s.callbacks {
		stop, err := callback(ctx, block)
		if err != nil {
			errs = errors.Join(errs, err)
		}

		signalStop = signalStop || stop
	}

	return signalStop, errs
}
