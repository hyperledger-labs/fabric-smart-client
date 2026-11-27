/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// GetRequest is the input of the get view: the index of the entry to fetch.
type GetRequest struct {
	Index uint64
}

// GetView returns the entry at Index as a protojson TransparencyLogEntry,
// with an inclusion proof against a checkpoint all three orgs signed.
type GetView struct {
	GetRequest
}

func (g *GetView) Call(viewCtx view.Context) (any, error) {
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
	if size := ls.Tree().Size(); g.Index >= size {
		return nil, errors.Errorf("index %d is beyond the log size %d", g.Index, size)
	}
	return buildEntry(viewCtx, ls, g.Index)
}

// GetViewFactory creates a GetView from a JSON GetRequest.
type GetViewFactory struct{}

func (*GetViewFactory) NewView(in []byte) (view.View, error) {
	f := &GetView{}
	if err := json.Unmarshal(in, &f.GetRequest); err != nil {
		return nil, errors.Wrap(err, "failed decoding the input")
	}
	return f, nil
}

// CheckpointRequest is the input of the checkpoint view: the size of a
// previously seen checkpoint to prove consistency against, 0 for none.
type CheckpointRequest struct {
	OldSize uint64
}

// CheckpointResponse carries the latest checkpoint, a consistency proof
// against OldSize when one was requested, and the reasons any peer refused
// to cosign (always empty from an honest node).
type CheckpointResponse struct {
	Checkpoint  []byte
	Consistency [][]byte
	Refusals    []string
}

// CheckpointView syncs the log, cosigns its current checkpoint with the
// other two orgs, and returns it together with a consistency proof against
// OldSize if OldSize is nonzero.
type CheckpointView struct {
	CheckpointRequest
}

func (c *CheckpointView) Call(viewCtx view.Context) (any, error) {
	ls, err := logServiceOf(viewCtx)
	if err != nil {
		return nil, err
	}
	cp, t, refusals, err := ls.checkpointAt(viewCtx)
	if err != nil {
		return nil, err
	}
	if c.OldSize > t.Size() {
		return nil, errors.Errorf("old size %d is beyond the log size %d", c.OldSize, t.Size())
	}
	resp := &CheckpointResponse{Checkpoint: cp, Refusals: refusals}
	if c.OldSize > 0 {
		if resp.Consistency, err = t.ConsistencyProof(c.OldSize, t.Size()); err != nil {
			return nil, err
		}
	}
	return resp, nil
}

// CheckpointViewFactory creates a CheckpointView from a JSON
// CheckpointRequest.
type CheckpointViewFactory struct{}

func (*CheckpointViewFactory) NewView(in []byte) (view.View, error) {
	f := &CheckpointView{}
	if err := json.Unmarshal(in, &f.CheckpointRequest); err != nil {
		return nil, errors.Wrap(err, "failed decoding the input")
	}
	return f, nil
}
