/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chat

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	viewservice "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = logging.MustGetLogger()

// Call sends one Request to the server and returns its Reply.
type Call struct {
	Request
}

func (c *Call) Call(ctx view.Context) (any, error) {
	server, err := lookup(ctx, ServerName)
	if err != nil {
		return nil, err
	}
	return request(ctx, c, server, c.Request, callTimeout)
}

// DM sends a direct message to another client. The server is not involved.
type DM struct {
	To   string `json:"to"`
	Text string `json:"text"`
}

func (d *DM) Call(ctx view.Context) (any, error) {
	to, err := lookup(ctx, d.To)
	if err != nil {
		return nil, err
	}
	return nil, exchange(ctx, d, to, d)
}

// Deliver receives a channel message the server pushes.
type Deliver struct {
	inbox *Inbox
}

func (d *Deliver) Call(ctx view.Context) (any, error) {
	js := session.JSON(ctx)
	var msg Message
	if err := js.ReceiveWithTimeout(&msg, exchangeTimeout); err != nil {
		return nil, errors.Wrap(err, "failed receiving message")
	}
	d.inbox.Put(msg)
	return nil, sendWithTimeout(ctx.Context(), js, &Reply{})
}

// ReceiveDM receives a direct message. The sender is the node FSC authenticated, not a name
// in the message.
type ReceiveDM struct {
	inbox *Inbox
}

func (r *ReceiveDM) Call(ctx view.Context) (any, error) {
	js := session.JSON(ctx)
	var dm DM
	if err := js.ReceiveWithTimeout(&dm, exchangeTimeout); err != nil {
		return nil, errors.Wrap(err, "failed receiving direct message")
	}
	from, _, err := whoIs(ctx, js.Info().Caller)
	if err != nil {
		return nil, err
	}
	r.inbox.Put(Message{From: from, Text: dm.Text})
	return nil, sendWithTimeout(ctx.Context(), js, &Reply{})
}

// ReadInbox returns and removes the messages in the inbox.
type ReadInbox struct {
	inbox *Inbox
}

func (r *ReadInbox) Call(view.Context) (any, error) {
	return r.inbox.Drain(), nil
}

// Inbox buffers the messages a client received until it reads them.
type Inbox struct {
	mu     sync.Mutex
	msgs   []Message
	notify chan struct{}
}

// NewInbox returns an empty Inbox.
func NewInbox() *Inbox {
	return &Inbox{notify: make(chan struct{}, 1)}
}

// Put adds m and signals Notify.
func (i *Inbox) Put(m Message) {
	i.mu.Lock()
	i.msgs = append(i.msgs, m)
	i.mu.Unlock()
	select {
	case i.notify <- struct{}{}:
	default:
	}
}

// Drain returns the buffered messages, oldest first, and empties the inbox.
func (i *Inbox) Drain() []Message {
	i.mu.Lock()
	defer i.mu.Unlock()
	msgs := i.msgs
	i.msgs = nil
	return msgs
}

// Notify returns a channel that receives a value after Put.
func (i *Inbox) Notify() <-chan struct{} {
	return i.notify
}

// OnlyFrom admits only the node with identity id.
func OnlyFrom(id view.Identity) viewservice.ResponderAuthorizer {
	return viewservice.AuthorizerFunc(func(_ context.Context, caller view.Identity) error {
		if !caller.Equal(id) {
			return errors.Wrapf(viewservice.ErrDenied, "only the server delivers channel messages, not [%s]", caller)
		}
		return nil
	})
}

type factory func(in []byte) (view.View, error)

func (f factory) NewView(in []byte) (view.View, error) { return f(in) }

// RegisterClient registers the client's views on the node sp: a view per server operation
// (login, logout, who, join, leave, post, each taking a Request), dm (taking a DM) and inbox,
// and the responders for channel and direct messages.
func RegisterClient(sp services.Provider, inbox *Inbox) error {
	registry := viewservice.GetRegistry(sp)
	for _, op := range []Op{OpLogin, OpLogout, OpWho, OpJoin, OpLeave, OpPost} {
		err := registry.RegisterFactory(string(op), factory(func(in []byte) (view.View, error) {
			c := &Call{}
			if err := json.Unmarshal(in, &c.Request); err != nil {
				return nil, errors.Wrap(err, "invalid request")
			}
			c.Op = op
			return c, nil
		}))
		if err != nil {
			return err
		}
	}
	err := registry.RegisterFactory("dm", factory(func(in []byte) (view.View, error) {
		d := &DM{}
		return d, errors.Wrap(json.Unmarshal(in, d), "invalid direct message")
	}))
	if err != nil {
		return err
	}
	if err := registry.RegisterFactory("inbox", factory(func([]byte) (view.View, error) { return &ReadInbox{inbox: inbox}, nil })); err != nil {
		return err
	}
	server, err := lookup(sp, ServerName)
	if err != nil {
		return err
	}
	return registerResponders(registry, inbox, server)
}

// registerResponders registers the responders for channel messages, which only server may
// start, and for direct messages.
func registerResponders(registry *viewservice.Registry, inbox *Inbox, server view.Identity) error {
	if err := registry.RegisterResponderWithOptions(&Deliver{inbox: inbox}, &Push{}, viewservice.WithAuthorizer(OnlyFrom(server))); err != nil {
		return err
	}
	return registry.RegisterResponder(&ReceiveDM{inbox: inbox}, &DM{})
}

// lookup returns the identity of the node called name.
func lookup(sp services.Provider, name string) (view.Identity, error) {
	provider, err := id.GetProvider(sp)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting identity provider")
	}
	identity := provider.Identity(name)
	if identity.IsNone() {
		return nil, errors.Errorf("unknown user [%s]", name)
	}
	return identity, nil
}
