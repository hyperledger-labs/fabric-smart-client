/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-x-committer/api/protoblocktx"
	"github.com/hyperledger/fabric-x-committer/api/protonotify"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/durationpb"
)

var logger = logging.MustGetLogger()

type notificationListenerManager struct {
	notifyStream  protonotify.Notifier_OpenNotificationStreamClient
	requestQueue  chan *protonotify.NotificationRequest
	responseQueue chan *protonotify.NotificationResponse

	handlers map[driver.TxID][]fabric.FinalityListener
	lock     sync.RWMutex
}

func (n *notificationListenerManager) Listen(ctx context.Context) error {
	g, gCtx := errgroup.WithContext(n.notifyStream.Context())
	// spawn stream receiver
	g.Go(func() error {
		for {
			res, err := n.notifyStream.Recv()
			if err != nil {
				return err
			}
			select {
			case <-gCtx.Done():
				return gCtx.Err()
			case n.responseQueue <- res:
			}
		}
	})

	// spawn stream sender
	g.Go(func() error {
		var req *protonotify.NotificationRequest
		for {
			select {
			case <-gCtx.Done():
				return gCtx.Err()
			case req = <-n.requestQueue:
			}

			if err := n.notifyStream.Send(req); err != nil {
				return err
			}
		}
	})

	// spawn notification dispatcher
	g.Go(func() error {
		var resp *protonotify.NotificationResponse
		for {
			select {
			case <-gCtx.Done():
				return gCtx.Err()
			case resp = <-n.responseQueue:
			}

			res := parseResponse(resp)

			n.lock.Lock()
			for txID, v := range res {
				handlers, ok := n.handlers[txID]
				if !ok {
					// nobody registered
					continue
				}
				// delete
				delete(n.handlers, txID)

				// now it is time to call the handlers
				for _, h := range handlers {
					h.OnStatus(gCtx, txID, v, "")
				}
			}
			n.lock.Unlock()
		}
	})

	return g.Wait()
}

func parseResponse(resp *protonotify.NotificationResponse) map[string]int {
	res := make(map[string]int)

	// first parse all timeouts
	for _, txID := range resp.GetTimeoutTxIds() {
		res[txID] = fdriver.Unknown
	}

	var s int
	// next we parse the status events
	for _, r := range resp.GetTxStatusEvents() {
		txID := r.GetTxId()
		status := r.GetStatusWithHeight()

		switch status.GetCode() {
		case protoblocktx.Status_COMMITTED:
			s = fdriver.Valid
		case protoblocktx.Status_NOT_VALIDATED:
			s = fdriver.Unknown
		default:
			s = fdriver.Invalid
		}

		res[txID] = s
	}

	return res
}

func (n *notificationListenerManager) AddFinalityListener(txID driver.TxID, listener fabric.FinalityListener) error {
	if listener == nil {
		return errors.New("listener nil")
	}

	n.lock.Lock()
	defer n.lock.Unlock()

	handlers := n.handlers[txID]
	for _, h := range handlers {
		if h == listener {
			logger.Warnf("The exact same listener is already registered for txID=%v. Skipping.", txID)
			// Do not register the same instance twice
			return nil
		}
	}
	n.handlers[txID] = append(handlers, listener)

	if len(handlers) > 0 {
		logger.Debugf("Additional listener registered for txID=%v. Request already sent.", txID)
		return nil
	}

	// this is our first listener registered for the given txID
	txIDs := []string{txID}
	n.requestQueue <- &protonotify.NotificationRequest{
		TxStatusRequest: &protonotify.TxStatusRequest{
			TxIds: txIDs,
		},
		// TODO: set a proper timeout
		Timeout: durationpb.New(10 * time.Second),
	}

	return nil
}

func (n *notificationListenerManager) RemoveFinalityListener(txID string, listener fabric.FinalityListener) error {
	if listener == nil {
		return errors.New("listener nil")
	}

	n.lock.Lock()
	defer n.lock.Unlock()

	handlers, ok := n.handlers[txID]
	if !ok || len(handlers) == 0 {
		// no handlers registered for this txID, nothing to remove
		logger.Debugf("RemoveFinalityListener called for unknown txID: %s", txID)
		return nil
	}

	initialLength := len(handlers)

	newHandlers := slices.DeleteFunc(handlers, func(h fabric.FinalityListener) bool {
		return h == listener
	})

	if len(newHandlers) == initialLength {
		// if the length is the same, no listener was removed.
		logger.Warnf("Listener not found for txID=%s, cannot remove.", txID)
		return nil
	}

	// check if the list of handlers is now empty
	if len(newHandlers) == 0 {
		// this was the last listener. Clean up our local map entry.
		logger.Debugf("Last finality listener removed for txID=%s.", txID)
		delete(n.handlers, txID)
	} else {
		n.handlers[txID] = newHandlers
		logger.Debugf("Removed listener for txID=%s. %d listeners remaining.", txID, len(newHandlers))
	}

	return nil
}
