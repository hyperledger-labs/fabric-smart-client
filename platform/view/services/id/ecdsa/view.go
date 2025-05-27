/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ecdsa

import (
	"log"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"

	session2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/session"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type twoPartyCollectEphemeralKeyView struct {
	Other view.Identity
}

func NewTwoPartyCollectEphemeralKeysView(other view.Identity) *twoPartyCollectEphemeralKeyView {
	return &twoPartyCollectEphemeralKeyView{Other: other}
}

func (f twoPartyCollectEphemeralKeyView) Call(context view.Context) (interface{}, error) {
	session, err := context.GetSession(context.Initiator(), f.Other)
	if err != nil {
		return nil, err
	}

	// Wait to receive a public key back
	ch := session.Receive()

	// Create ephemeral key and store it in the context
	id, signer, verifier, err := NewSigner()
	if err != nil {
		return nil, err
	}
	sigService := driver.GetSigRegistry(context)
	err = sigService.RegisterSigner(id, signer, verifier)
	if err != nil {
		return nil, err
	}

	// send the public key
	err = session.Send(id)
	if err != nil {
		return nil, err
	}

	timeout := time.NewTimer(time.Second * 30)
	defer timeout.Stop()

	select {
	case msg := <-ch:
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}
		log.Printf("twoPartyCollectEphemeralKeyView [%s]\n", msg.Payload)

		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}

		id2, verifier, err := NewIdentityFromBytes(msg.Payload)
		if err != nil {
			return nil, err
		}
		err = sigService.RegisterVerifier(id2, verifier)
		if err != nil {
			return nil, err
		}

		// Update the Endpoint Resolver
		resolver := view2.GetEndpointService(context)
		err = resolver.Bind(context.Context(), context.Me(), id)
		if err != nil {
			return nil, err
		}
		err = resolver.Bind(context.Context(), f.Other, id2)
		if err != nil {
			return nil, err
		}

		return []view.Identity{id, id2}, nil
	case <-timeout.C:
		return nil, errors.New("timeout reading from session")
	}
}

type twoPartyEphemeralKeyResponderView struct{}

func (s *twoPartyEphemeralKeyResponderView) Call(context view.Context) (interface{}, error) {
	session, payload, err := session2.ReadFirstMessage(context)
	if err != nil {
		return nil, err
	}

	sigService := driver.GetSigRegistry(context)

	// Parse received identity
	id2, verifier, err := NewIdentityFromBytes(payload)
	if err != nil {
		return nil, err
	}
	err = sigService.RegisterVerifier(id2, verifier)
	if err != nil {
		return nil, err
	}

	// Create ephemeral key, store it in the context, and send it back
	id, signer, verifier, err := NewSigner()
	if err != nil {
		return nil, err
	}
	err = sigService.RegisterSigner(id, signer, verifier)
	if err != nil {
		return nil, err
	}

	// Step 3: send the public key back to the invoker
	err = session.Send(id)
	if err != nil {
		return nil, err
	}

	// Update the Endpoint Resolver
	resolver := view2.GetEndpointService(context)
	err = resolver.Bind(context.Context(), context.Me(), id)
	if err != nil {
		return nil, err
	}
	err = resolver.Bind(context.Context(), session.Info().Caller, id2)
	if err != nil {
		return nil, err
	}

	return []view.Identity{id, id2}, nil
}

func NewTwoPartyEphemeralKeyResponderView() *twoPartyEphemeralKeyResponderView {
	return &twoPartyEphemeralKeyResponderView{}
}
