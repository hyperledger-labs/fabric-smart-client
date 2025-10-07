package finality

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
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
	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var resp *protonotify.NotificationResponse

		select {
		case <-gCtx.Done():
		case resp = <-n.responseQueue:
		}

		// TODO:
		_ = resp.GetTimeoutTxIds()

		for _, r := range resp.TxStatusEvents {
			txID := r.GetTxId()
			//status := r.GetStatusWithHeight().GetCode()

			n.lock.Lock()
			handlers, ok := n.handlers[txID]
			if !ok {
				// nobody registered
				n.lock.Unlock()
				continue
			}
			// delete
			delete(n.handlers, txID)
			n.lock.Unlock()

			// now it is time to call the handlers
			logger.Warnf("calling handlers for txID=%v", txID)
			for _, h := range handlers {
				// TODO fix status
				h.OnStatus(gCtx, txID, 1, "TODO")
			}
		}
		return nil
	})

	return g.Wait()
}

func (n *notificationListenerManager) OpenNotificationStream(stream protonotify.Notifier_OpenNotificationStreamClient) error {
	g, gCtx := errgroup.WithContext(stream.Context())

	g.Go(func() error {
		for gCtx.Err() == nil {
			res, err := stream.Recv()
			if err != nil {
				return err
			}
			select {
			case <-gCtx.Done():
				return gCtx.Err()
			case n.responseQueue <- res:
			}
		}
		return gCtx.Err()
	})

	g.Go(func() error {
		for gCtx.Err() == nil {
			var req *protonotify.NotificationRequest
			select {
			case <-gCtx.Done():
			case req = <-n.requestQueue:
			}

			if err := stream.Send(req); err != nil {
				return err
			}
		}
		return gCtx.Err()
	})

	return g.Wait()
}

func (n *notificationListenerManager) AddFinalityListener(txID driver.TxID, listener fabric.FinalityListener) error {
	if listener == nil {
		return errors.New("listener nil")
	}

	n.lock.Lock()
	defer n.lock.Unlock()

	handlers := n.handlers[txID]
	n.handlers[txID] = append(handlers, listener)

	if len(handlers) > 1 {
		logger.Warnf("callback for txID=%v already exists", txID)

		// there is someone already requested a notification for this txID
		return nil
	}

	logger.Warnf("register new callback for txID=%v", txID)

	// this is our first listener registered for the given txID
	txIDs := []string{txID}
	n.requestQueue <- &protonotify.NotificationRequest{
		TxStatusRequest: &protonotify.TxStatusRequest{
			TxIds: txIDs,
		},
		Timeout: durationpb.New(3 * time.Minute),
	}

	return nil
}

func (n *notificationListenerManager) RemoveFinalityListener(txID string, listener fabric.FinalityListener) error {
	// TODO: implement me
	return nil
}
