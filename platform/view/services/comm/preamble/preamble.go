/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package preamble defines what two FSC nodes tell each other when a stream opens: their FSC
// release, the framing epoch of their view packets, and, per component, the range of wire
// format versions they support. Transports exchange it only after they have authenticated
// the peer; see docs/platform/view/services/comm/readme.md.
package preamble

import (
	"slices"

	"google.golang.org/protobuf/proto"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/buildinfo"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

const (
	// FramingEpoch is the epoch of the view packet framing on a stream: how packets are
	// delimited and routed. It is raised only when that framing changes. Nodes of different
	// epochs refuse each other's streams.
	FramingEpoch uint32 = 1

	// MaxSize bounds an encoded preamble, in bytes. Like the message format, it never shrinks:
	// a node refuses a larger preamble from a later release.
	MaxSize = 4 << 10
	// MaxStringSize bounds every string field of a preamble, in bytes. It never shrinks either.
	MaxStringSize = 256
)

// ErrEpoch is wrapped by Accept for a peer whose framing epoch differs from FramingEpoch.
var ErrEpoch = errors.New("framing epoch mismatch")

// Local returns the preamble of this binary, with FSCComponents and the registered components.
func Local() *Preamble { return build(Components()) }

func build(cs []Component) *Preamble {
	info := buildinfo.Get()
	p := &Preamble{
		FscVersion:   truncate(info.FSCVersion),
		VcsRevision:  truncate(info.Revision),
		FramingEpoch: FramingEpoch,
	}
	for _, c := range cs {
		p.Ranges = append(p.Ranges, &ComponentRange{Component: c.Name, Min: c.Range.Min, Max: c.Range.Max})
	}
	return p
}

// LocalBytes is Local encoded. It changes only when a component is registered.
func LocalBytes() []byte { return local.Load().raw }

// Encode marshals p. It fails if the result would be refused by decode.
func Encode(p *Preamble) ([]byte, error) {
	if err := checkStrings(p); err != nil {
		return nil, err
	}
	raw, err := proto.Marshal(p)
	if err != nil {
		return nil, errors.Wrap(err, "failed marshalling preamble")
	}
	if len(raw) > MaxSize {
		return nil, errors.Errorf("preamble of %d bytes exceeds %d", len(raw), MaxSize)
	}
	return raw, nil
}

// decode unmarshals a preamble a peer sent. It refuses raw over MaxSize before unmarshalling
// it and a preamble with a string field over MaxStringSize. Fields that a later release adds
// are ignored.
func decode(raw []byte) (*Preamble, error) {
	if len(raw) > MaxSize {
		return nil, errors.Errorf("preamble of %d bytes exceeds %d", len(raw), MaxSize)
	}
	p := &Preamble{}
	if err := proto.Unmarshal(raw, p); err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling preamble")
	}
	if err := checkStrings(p); err != nil {
		return nil, err
	}
	return p, nil
}

// Accept decodes the preamble raw that a peer sent and checks its framing epoch. The error of
// a refused preamble starts with "version: ", the reason a transport reports to the peer and
// counts, and wraps ErrEpoch for an epoch mismatch.
func Accept(raw []byte) (*Preamble, error) {
	p, err := decode(raw)
	if err == nil {
		err = checkEpoch(p)
	}
	if err != nil {
		return nil, errors.WithMessage(err, "version")
	}
	return p, nil
}

// checkEpoch returns an error wrapping ErrEpoch, which names both epochs, if p's framing
// epoch is not FramingEpoch. FSC versions and revisions are not compared: nodes of different
// releases must keep talking to each other.
func checkEpoch(p *Preamble) error {
	if p.GetFramingEpoch() != FramingEpoch {
		return errors.Wrapf(ErrEpoch, "peer framing epoch %d, local %d", p.GetFramingEpoch(), FramingEpoch)
	}
	return nil
}

func checkStrings(p *Preamble) error {
	fields := []string{p.GetFscVersion(), p.GetVcsRevision()}
	for _, r := range p.GetRanges() {
		fields = append(fields, r.GetComponent())
	}
	if slices.ContainsFunc(fields, func(s string) bool { return len(s) > MaxStringSize }) {
		return errors.Errorf("preamble string field exceeds %d bytes", MaxStringSize)
	}
	return nil
}

func truncate(s string) string { return s[:min(len(s), MaxStringSize)] }
