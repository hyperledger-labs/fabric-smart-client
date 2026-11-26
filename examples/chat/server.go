/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chat

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	viewservice "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// ServerName is the name of the chat server node.
const ServerName = "server"

// exchangeTimeout bounds every send and every wait for an answer.
const exchangeTimeout = 10 * time.Second

// callTimeout is how long a client waits for the server's reply. A post waits for the
// slowest member's push, which takes up to two exchanges (send and acknowledgement).
const callTimeout = 3 * exchangeTimeout

// Server answers the Request a client sends. It knows the client from the session: FSC
// authenticated the remote node, and the server's resolvers name it and its organization.
type Server struct {
	rooms *Rooms
}

func (s *Server) Call(ctx view.Context) (any, error) {
	js := session.JSON(ctx)
	req := &Request{}
	if err := js.ReceiveWithTimeout(req, exchangeTimeout); err != nil {
		return nil, errors.Wrap(err, "failed receiving request")
	}
	caller := js.Info().Caller
	name, org, err := whoIs(ctx, caller)
	if err != nil {
		return nil, err
	}
	reply := s.handle(ctx, name, org, caller, req)
	return nil, sendWithTimeout(ctx.Context(), js, reply)
}

func (s *Server) handle(ctx view.Context, name, org string, caller view.Identity, req *Request) *Reply {
	if req.Channel == "" && (req.Op == OpJoin || req.Op == OpLeave || req.Op == OpPost) {
		return &Reply{Error: "no channel given"}
	}
	switch req.Op {
	case OpLogin:
		return &Reply{Users: s.rooms.Login(name, org, caller)}
	case OpLogout:
		s.rooms.Logout(name)
		return &Reply{}
	case OpWho:
		return &Reply{Users: s.rooms.Who()}
	case OpJoin:
		return errorReply(s.rooms.Join(name, req.Channel))
	case OpLeave:
		return errorReply(s.rooms.Leave(name, req.Channel))
	case OpPost:
		return s.post(ctx, name, req)
	default:
		return &Reply{Error: "unknown operation " + string(req.Op)}
	}
}

// post pushes the message to every other member of the channel, all at once with a session
// each, so a member whose node hangs delays nobody else, and reports the members it did not
// reach.
func (s *Server) post(ctx view.Context, name string, req *Request) *Reply {
	recipients, err := s.rooms.Recipients(name, req.Channel)
	if err != nil {
		return errorReply(err)
	}
	manager, err := viewservice.GetManager(ctx)
	if err != nil {
		return errorReply(err)
	}
	msg := Message{Channel: strings.ToLower(req.Channel), From: name, Text: req.Text}
	var (
		mu     sync.Mutex
		failed []string
		wg     sync.WaitGroup
	)
	for member, id := range recipients {
		wg.Go(func() {
			// Each push runs in a view context of its own, since a context is not meant to
			// run views from several goroutines.
			if _, err := manager.InitiateView(ctx.Context(), &Push{To: id, Msg: msg}); err != nil {
				logger.Warnf("failed delivering to [%s]: %s", member, err)
				mu.Lock()
				failed = append(failed, member)
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	slices.Sort(failed)
	return &Reply{Failed: failed}
}

// Push delivers a channel message from the server to one member.
type Push struct {
	To  view.Identity
	Msg Message
}

func (p *Push) Call(ctx view.Context) (any, error) {
	return nil, exchange(ctx, p, p.To, p.Msg)
}

// RegisterServer registers the server's responder on the node sp. The organizations it
// knows, and so the organization channels, are those of the nodes it has resolvers for.
func RegisterServer(sp services.Provider) error {
	var orgs []string
	for _, r := range endpoint.GetService(sp).Resolvers() {
		orgs = append(orgs, orgOf(r.Domain))
	}
	server := &Server{rooms: NewRooms(orgs)}
	return viewservice.GetRegistry(sp).RegisterResponder(server, &Call{})
}

// whoIs returns the name and organization of the node with identity id.
func whoIs(ctx view.Context, id view.Identity) (name, org string, err error) {
	r, err := endpoint.GetService(ctx).GetResolver(ctx.Context(), id)
	if err != nil {
		return "", "", errors.Wrapf(err, "unknown node [%s]", id)
	}
	return r.Name, orgOf(r.Domain), nil
}

// orgOf returns the organization of a domain: org1 for org1.example.com.
func orgOf(domain string) string {
	org, _, _ := strings.Cut(domain, ".")
	return org
}

func errorReply(err error) *Reply {
	if err != nil {
		return &Reply{Error: err.Error()}
	}
	return &Reply{}
}

// exchange opens a session from v to party, sends out, and waits for the Reply.
func exchange(ctx view.Context, v view.View, party view.Identity, out any) error {
	_, err := request(ctx, v, party, out, exchangeTimeout)
	return err
}

// request opens a session from v to party, sends out, and returns the Reply, waiting for it
// at most wait; a Reply with an error is returned as an error.
func request(ctx view.Context, v view.View, party view.Identity, out any, wait time.Duration) (*Reply, error) {
	js, err := session.NewJSON(ctx, v, party)
	if err != nil {
		return nil, errors.Wrap(err, "failed opening session")
	}
	if err := sendWithTimeout(ctx.Context(), js, out); err != nil {
		return nil, err
	}
	reply := &Reply{}
	if err := js.ReceiveWithTimeout(reply, wait); err != nil {
		return nil, errors.Wrap(err, "failed receiving reply")
	}
	if reply.Error != "" {
		return nil, errors.New(reply.Error)
	}
	return reply, nil
}

type sender interface {
	Send(ctx context.Context, state any) error
}

func sendWithTimeout(ctx context.Context, s sender, out any) error {
	ctx, cancel := context.WithTimeout(ctx, exchangeTimeout)
	defer cancel()
	return errors.Wrap(s.Send(ctx, out), "failed sending")
}
