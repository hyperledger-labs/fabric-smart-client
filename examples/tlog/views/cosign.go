/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"bytes"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog/translog"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Why a peer refuses to cosign a checkpoint.
const (
	// RefusedMismatch: the peer's tree has a different root at that size.
	RefusedMismatch = "mismatch"
	// RefusedBehind: the peer's tree did not reach that size in time.
	RefusedBehind = "behind"
)

const (
	cosignTimeout   = 30 * time.Second
	catchUpTimeout  = 10 * time.Second
	catchUpInterval = 500 * time.Millisecond
)

// CosignRequest asks a peer to cosign the checkpoint (Size, Root).
type CosignRequest struct {
	Size uint64
	Root []byte
}

// CosignResponse carries the peer's signed note, or the reason it refused.
type CosignResponse struct {
	Note    []byte
	Refused string
}

// CosignView asks Peer to cosign a checkpoint and returns its
// *CosignResponse.
type CosignView struct {
	Peer view.Identity
	CosignRequest
}

func (c *CosignView) Call(viewCtx view.Context) (any, error) {
	s, err := session.NewJSON(viewCtx, c, c.Peer)
	if err != nil {
		return nil, errors.Wrap(err, "failed opening a session")
	}
	if err := s.Send(viewCtx.Context(), &c.CosignRequest); err != nil {
		return nil, errors.Wrap(err, "failed sending the cosign request")
	}
	resp := &CosignResponse{}
	if err := s.ReceiveWithTimeout(resp, cosignTimeout); err != nil {
		return nil, errors.Wrap(err, "failed receiving the cosign response")
	}
	return resp, nil
}

// CosignResponderView cosigns a checkpoint if its own tree, read from the
// ledger, has the same root at that size. It waits a while for its tree to
// catch up first.
type CosignResponderView struct{}

func (*CosignResponderView) Call(viewCtx view.Context) (any, error) {
	s := session.JSON(viewCtx)
	req := &CosignRequest{}
	if err := s.ReceiveWithTimeout(req, cosignTimeout); err != nil {
		return nil, errors.Wrap(err, "failed receiving the cosign request")
	}
	ls, err := logServiceOf(viewCtx)
	if err != nil {
		return nil, err
	}
	qs, err := queryServiceOf(viewCtx)
	if err != nil {
		return nil, err
	}

	var decision string
	deadline := time.Now().Add(catchUpTimeout)
	for {
		if err := ls.Sync(qs); err != nil {
			return nil, err
		}
		decision, err = cosignDecision(ls.Tree(), req.Size, req.Root)
		if err != nil {
			return nil, err
		}
		if decision != RefusedBehind || time.Now().After(deadline) {
			break
		}
		time.Sleep(catchUpInterval)
	}

	resp := &CosignResponse{Refused: decision}
	if decision == "" {
		if resp.Note, err = ls.sign(req.Size, req.Root); err != nil {
			return nil, err
		}
	}
	if err := s.Send(viewCtx.Context(), resp); err != nil {
		return nil, errors.Wrap(err, "failed sending the cosign response")
	}
	return nil, nil
}

// cosignDecision returns "" if t has root at size, else why not to cosign.
func cosignDecision(t *translog.Tree, size uint64, root []byte) (string, error) {
	if t.Size() < size {
		return RefusedBehind, nil
	}
	own, err := t.Root(size)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(own, root) {
		return RefusedMismatch, nil
	}
	return "", nil
}
