/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tss

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	mpc "github.com/IBM/TSS/mpc/binance/ecdsa"
	"github.com/IBM/TSS/threshold"
	tss "github.com/IBM/TSS/types"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"go.uber.org/zap/zapcore"
)

type Signer struct {
	ts               tss.MpcParty
	cf               ContextFactory
	ip               IdentityProvider
	uidToNode        map[tss.UniversalID]string
	contextSessionID string
	SelfID           tss.UniversalID

	context  view.Context
	sessions map[tss.UniversalID]view.Session
}

func NewSigner(
	cf ContextFactory,
	ip IdentityProvider,
	opts MSPOpts,
	uidToNode map[tss.UniversalID]string,
	mapping map[tss.UniversalID]tss.PartyID,
	storedData []byte,
	contextSessionID string,
) *Signer {
	signer := &Signer{
		cf:               cf,
		ip:               ip,
		uidToNode:        uidToNode,
		contextSessionID: contextSessionID,
		SelfID:           opts.SelfID,
	}
	signer.ts = threshold.LoudScheme(
		uint16(opts.SelfID),
		&wrappedLogger{flogging.MustGetLogger("fabric-sdk.msp.tss." + opts.ID)},
		func(id uint16) tss.KeyGenerator {
			node := uidToNode[tss.UniversalID(id)]
			if len(node) == 0 {
				panic(fmt.Sprintf("cannot find node name for uid [%d]", id))
			}
			return mpc.NewParty(id, flogging.MustGetLogger("fabric-sdk.msp.tss."+node))
		},
		func(id uint16) tss.Signer {
			node := uidToNode[tss.UniversalID(id)]
			if len(node) == 0 {
				panic(fmt.Sprintf("cannot find node name for uid [%d]", id))
			}
			return mpc.NewParty(id, flogging.MustGetLogger("fabric-sdk.msp.tss."+node))
		},
		opts.Threshold,
		signer.send,
		func() map[tss.UniversalID]tss.PartyID {
			return mapping
		},
	)
	signer.ts.SetStoredData(storedData)
	signer.InitSessions()
	return signer
}

func (s *Signer) send(msgType uint8, topic []byte, msg []byte, to ...uint16) {
	logger.Debugf("tss send message to [%v], topic [%s]", to, string(topic))
	// send an IncMessage
	for _, destination := range to {
		id := tss.UniversalID(destination)
		session := s.sessions[id]
		incMessage := &tss.IncMessage{
			Data:    msg,
			Source:  uint16(s.SelfID),
			MsgType: msgType,
			Topic:   topic,
		}
		v, err := json.Marshal(incMessage)
		if err != nil {
			panic(err)
		}
		logger.Debugf("tss send message [%s], topic [%s]", hash.Hashable(v).String(), string(topic))
		if err := session.Send(v); err != nil {
			panic(err)
		}
	}
}

func (s *Signer) Sign(message []byte) ([]byte, error) {
	hash := sha256.Sum256(message)
	topic := string(hash[:])
	ctx := context.Background()
	logger.Debugf("tss sign message with topic [%s]...", topic)
	return s.ts.Sign(ctx, hash[:], topic)
}

func (s *Signer) InitSessions() {
	logger.Debugf("open comm session for the tss sign process with context [%s]...", s.contextSessionID)
	// Get context
	var err error
	s.context, err = s.cf.InitiateContextWithIdentityAndID(nil, nil, s.contextSessionID)
	if err != nil {
		panic(fmt.Sprintf("failed to get context with identity [%s]", s.contextSessionID))
	}

	// open sessions to all business parties
	s.sessions = map[tss.UniversalID]view.Session{}
	for id, node := range s.uidToNode {
		logger.Debugf("opening session to [%s]", node)
		session, err := s.context.GetSessionByID(s.contextSessionID, s.ip.Identity(node))
		if err != nil {
			panic(fmt.Sprintf("failed to get session to [%s]", node))
		}
		s.sessions[id] = session
	}

	for _, session := range s.sessions {
		session := session
		go func() {
			ch := session.Receive()
			for {
				// receive message
				timeout := time.NewTimer(1 * time.Minute)
				var raw []byte
				select {
				case msg := <-ch:
					timeout.Stop()
					if msg.Status == view.ERROR {
						panic(fmt.Sprintf("received error from remote [%s]", string(msg.Payload)))
					}
					raw = msg.Payload
				case <-timeout.C:
					timeout.Stop()
					continue
				}
				logger.Debugf("tss received message [%s]", hash.Hashable(raw).String())

				incMessage := &tss.IncMessage{}
				if err := json.Unmarshal(raw, incMessage); err != nil {
					panic(err)
				}
				logger.Debugf("tss received message [%s] for topic [%s]", hash.Hashable(raw).String(), base64.StdEncoding.EncodeToString(incMessage.Topic))

				// dispatch
				s.ts.HandleMessage(incMessage)
			}
		}()
	}
}

type wrappedLogger struct {
	*flogging.FabricLogger
}

func (w *wrappedLogger) DebugEnabled() bool {
	return w.IsEnabledFor(zapcore.DebugLevel)
}
