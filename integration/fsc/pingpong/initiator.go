/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = logging.MustGetLogger()

// PingView sends a single ping and waits for the pong over the context's session.
type PingView struct {
	responder view.Identity
	session   view.Session
}

func (p *PingView) Call(viewCtx view.Context) (any, error) {
	// session, err := viewCtx.GetSession(viewCtx.Initiator(), p.responder)
	// assert.NoError(err)
	session := p.session

	logger.Infof("send ping")

	if err := session.SendWithContext(viewCtx.Context(), []byte("ping")); err != nil {
		return nil, errors.Wrap(err, "failed to send ping")
	}

	ch := session.Receive()
	select {
	case msg := <-ch:
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}
		if m := string(msg.Payload); m != "pong" {
			return nil, errors.Errorf("expected pong, got %s", m)
		}
		logger.Infof("pong received")
	case <-viewCtx.Context().Done():
		return nil, errors.Errorf("expected ping, got %s", viewCtx.Context().Err())
	case <-time.After(1 * time.Minute):
		return nil, errors.New("responder didn't pong in time")
	}

	return nil, nil
}

func ping(viewCtx view.Context, responder view.Identity) (any, error) {
	session, err := viewCtx.GetSession(viewCtx.Initiator(), responder)
	assert.NoError(err)

	ctx, cancel := context.WithTimeout(viewCtx.Context(), 10*time.Minute)
	runCtx := view2.WrapContext(viewCtx, ctx)
	defer func() {
		logger.Infof("call cancel on view context [%s:%s]", runCtx.ID(), viewCtx.ID())
		cancel()
	}()

	return runCtx.RunView(&PingView{
		responder: responder,
		session:   session,
	})
}

type Initiator struct{}

func (p *Initiator) Call(viewCtx view.Context) (any, error) {
	// Retrieve responder identity
	identityProvider, err := id.GetProvider(viewCtx)
	assert.NoError(err, "failed getting identity provider")
	responder := identityProvider.Identity("responder")

	// Open a session to the responder
	for range 3 {
		_, err = ping(viewCtx, responder)
		assert.NoError(err, "ping round failed")
	}

	// Return
	return "OK", nil
}
