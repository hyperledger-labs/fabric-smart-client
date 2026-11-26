/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"sync"

	rekorv1 "github.com/sigstore/protobuf-specs/gen/pb-go/rekor/v1"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog/translog"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// The cheat modes of a node that registers the cheating views, org3 in the
// demo. Honest nodes register none of this file.
const (
	// CheatHonest serves the honest log.
	CheatHonest = ""
	// CheatRewriteEntry serves an entry with one byte of its body flipped,
	// together with the honest inclusion proof.
	CheatRewriteEntry = "rewrite-entry"
	// CheatFork serves, and asks the peers to cosign, a checkpoint of the
	// log with the entry at index 1 dropped.
	CheatFork = "fork"
)

// cheatMode is the mode of this node's cheating views, process-wide.
var cheatMode struct {
	sync.Mutex
	mode string
}

func currentCheat() string {
	cheatMode.Lock()
	defer cheatMode.Unlock()
	return cheatMode.mode
}

// CheatRequest is the input of the cheat view: the mode to switch to.
type CheatRequest struct {
	Mode string
}

// CheatView sets the mode the cheating views of this node run in.
type CheatView struct {
	CheatRequest
}

func (c *CheatView) Call(view.Context) (any, error) {
	switch c.Mode {
	case CheatHonest, CheatRewriteEntry, CheatFork:
	default:
		return nil, errors.Errorf("unknown cheat mode [%s]", c.Mode)
	}
	cheatMode.Lock()
	defer cheatMode.Unlock()
	cheatMode.mode = c.Mode
	return nil, nil
}

// CheatViewFactory creates a CheatView from a JSON CheatRequest.
type CheatViewFactory struct{}

func (*CheatViewFactory) NewView(in []byte) (view.View, error) {
	f := &CheatView{}
	if err := json.Unmarshal(in, &f.CheatRequest); err != nil {
		return nil, errors.Wrap(err, "failed decoding the input")
	}
	return f, nil
}

// CheatingGetView is GetView, except in mode CheatRewriteEntry, where it
// flips one byte of the entry's body and keeps the honest proof.
type CheatingGetView struct {
	GetView
}

func (g *CheatingGetView) Call(viewCtx view.Context) (any, error) {
	res, err := g.GetView.Call(viewCtx)
	if err != nil || currentCheat() != CheatRewriteEntry {
		return res, err
	}
	raw, ok := res.([]byte)
	if !ok {
		return nil, errors.Errorf("unexpected get result type [%T]", res)
	}
	entry := &rekorv1.TransparencyLogEntry{}
	if err := protojson.Unmarshal(raw, entry); err != nil {
		return nil, errors.Wrap(err, "failed decoding the entry")
	}
	entry.CanonicalizedBody[0] ^= 0x01
	return protojson.Marshal(entry)
}

// CheatingGetViewFactory creates a CheatingGetView from a JSON GetRequest.
type CheatingGetViewFactory struct{}

func (*CheatingGetViewFactory) NewView(in []byte) (view.View, error) {
	f := &CheatingGetView{}
	if err := json.Unmarshal(in, &f.GetRequest); err != nil {
		return nil, errors.Wrap(err, "failed decoding the input")
	}
	return f, nil
}

// CheatingCheckpointView is CheckpointView, except in mode CheatFork, where
// it asks the peers to cosign the log without the entry at index 1 and
// returns whatever note that gives, with the peers' refusals.
type CheatingCheckpointView struct {
	CheckpointView
}

func (c *CheatingCheckpointView) Call(viewCtx view.Context) (any, error) {
	if currentCheat() != CheatFork {
		return c.CheckpointView.Call(viewCtx)
	}
	ls, err := logServiceOf(viewCtx)
	if err != nil {
		return nil, err
	}
	qs, err := queryServiceOf(viewCtx)
	if err != nil {
		return nil, err
	}
	if err := ls.Sync(qs); err != nil {
		return nil, err
	}
	// Dropping the last entry would give the honest, smaller tree, which
	// the peers would rightly cosign; drop one from the middle instead.
	size := ls.Tree().Size()
	if size < 3 {
		return nil, errors.Errorf("cannot fork a log of size %d, need at least 3", size)
	}
	var leaves [][]byte
	for i := range size {
		if i == 1 {
			continue
		}
		body, err := readEntry(qs, i)
		if err != nil {
			return nil, err
		}
		leaves = append(leaves, body)
	}
	fork, err := translog.NewTree(leaves)
	if err != nil {
		return nil, err
	}
	root, err := fork.Root(fork.Size())
	if err != nil {
		return nil, err
	}
	cp, refusals, err := ls.Cosign(viewCtx, fork.Size(), root)
	if err != nil {
		return nil, err
	}
	// No consistency proof: the client rejects the note before asking for one.
	return &CheckpointResponse{Checkpoint: cp, Refusals: refusals}, nil
}

// CheatingCheckpointViewFactory creates a CheatingCheckpointView from a
// JSON CheckpointRequest.
type CheatingCheckpointViewFactory struct{}

func (*CheatingCheckpointViewFactory) NewView(in []byte) (view.View, error) {
	f := &CheatingCheckpointView{}
	if err := json.Unmarshal(in, &f.CheckpointRequest); err != nil {
		return nil, errors.Wrap(err, "failed decoding the input")
	}
	return f, nil
}
