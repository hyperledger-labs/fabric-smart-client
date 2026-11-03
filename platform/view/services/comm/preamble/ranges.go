/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package preamble

import (
	"maps"
	"regexp"
	"slices"
	"sync/atomic"

	"google.golang.org/protobuf/proto"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// Range is the range of versions of one component's formats, Min to Max inclusive.
type Range struct{ Min, Max uint32 }

// Contains reports whether v is in r.
func (r Range) Contains(v uint32) bool { return r.Min <= v && v <= r.Max }

// Component is a named part of a binary that owns wire formats, with the range of their
// versions the binary supports. FSC owns view, fabric and fabricx; an application adds its own
// for its own protocols.
type Component struct {
	Name  string
	Range Range
}

// DefaultRange is what a peer supports of a component it does not list: version 1. A peer
// without a preamble runs an older release, and a component missing from a peer's preamble is
// newer than the peer's release, so both are taken to speak the first version.
var DefaultRange = Range{Min: 1, Max: 1}

// FSCComponents are the components FSC owns, at the ranges this release supports.
var FSCComponents = []Component{
	{Name: "view", Range: Range{1, 1}},
	{Name: "fabric", Range: Range{1, 1}},
	{Name: "fabricx", Range: Range{1, 1}},
}

// componentName is the form of a component name. It never contains '/', so a protocol name
// "<component>/<name>" splits unambiguously.
var componentName = regexp.MustCompile(`^[a-z0-9-]+$`)

// local is what SetComponents installed: this node's components and its encoded preamble.
var local atomic.Pointer[localState]

type localState struct {
	components []Component
	raw        []byte
}

func init() {
	if err := SetComponents(FSCComponents); err != nil {
		panic(err) // FSCComponents are valid; failing here is a programming error.
	}
}

// SetComponents makes cs the components this node publishes in its preamble and negotiates
// with. It fails, keeping the current components, if two share a name, a name does not match
// [a-z0-9-]+, or a range is empty or starts at 0.
//
// Components belong to the binary, not to one node: every node of the process publishes the
// same, as with the FSC version. The view SDK calls SetComponents once, before comm starts;
// until then the components are FSCComponents. A stream that opened earlier keeps the preamble
// it sent.
func SetComponents(cs []Component) error {
	seen := map[string]Range{}
	for _, c := range cs {
		if !componentName.MatchString(c.Name) || len(c.Name) > MaxStringSize {
			return errors.Errorf("invalid component name [%s]: want [a-z0-9-]+", c.Name)
		}
		if c.Range.Min == 0 || c.Range.Min > c.Range.Max {
			return errors.Errorf("invalid range [%d, %d] for component [%s]", c.Range.Min, c.Range.Max, c.Name)
		}
		if old, ok := seen[c.Name]; ok {
			return errors.Errorf("component [%s] registered twice: [%d, %d] and [%d, %d]",
				c.Name, old.Min, old.Max, c.Range.Min, c.Range.Max)
		}
		seen[c.Name] = c.Range
	}
	s := &localState{components: slices.Clone(cs)}
	raw, err := Encode(build(s.components))
	if err != nil {
		return err
	}
	s.raw = raw
	local.Store(s)
	return nil
}

// Components returns this node's components. The result must not be modified.
func Components() []Component { return local.Load().components }

// LocalRange returns this node's range for the named component, and false if it has none.
func LocalRange(name string) (Range, bool) {
	for _, c := range Components() {
		if c.Name == name {
			return c.Range, true
		}
	}
	return Range{}, false
}

// PeerRange returns the range for the named component in p, a peer's preamble. A nil p, or one
// that does not list the component, yields DefaultRange.
func PeerRange(p *Preamble, name string) Range {
	for _, r := range p.GetRanges() {
		if r.GetComponent() == name {
			return Range{Min: r.GetMin(), Max: r.GetMax()}
		}
	}
	return DefaultRange
}

// Highest returns the highest version in both ranges, and false if they do not overlap.
func Highest(a, b Range) (uint32, bool) {
	lo, hi := max(a.Min, b.Min), min(a.Max, b.Max)
	return hi, lo <= hi
}

// MaxVersionsSize bounds encoded versions, in bytes, as MaxSize bounds the ranges they are
// chosen from. Like MaxSize, it never shrinks.
const MaxVersionsSize = MaxSize

// EncodeVersions encodes versions, by component, in component order, failing if the result
// exceeds MaxVersionsSize.
func EncodeVersions(versions map[string]uint32) ([]byte, error) {
	vs := &ComponentVersions{}
	for _, c := range slices.Sorted(maps.Keys(versions)) {
		vs.Versions = append(vs.Versions, &ComponentVersion{Component: c, Version: versions[c]})
	}
	raw, err := proto.Marshal(vs)
	if err != nil {
		return nil, errors.Wrap(err, "failed marshalling versions")
	}
	if len(raw) > MaxVersionsSize {
		return nil, errors.Errorf("versions of %d bytes exceed %d", len(raw), MaxVersionsSize)
	}
	return raw, nil
}

// DecodeVersions decodes the versions a peer sent, by component. It refuses raw over
// MaxVersionsSize before it decodes it. Empty raw yields nil: the peer stated none.
func DecodeVersions(raw []byte) (map[string]uint32, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > MaxVersionsSize {
		return nil, errors.Errorf("versions of %d bytes exceed %d", len(raw), MaxVersionsSize)
	}
	vs := &ComponentVersions{}
	if err := proto.Unmarshal(raw, vs); err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling versions")
	}
	m := make(map[string]uint32, len(vs.GetVersions()))
	for _, v := range vs.GetVersions() {
		m[v.GetComponent()] = v.GetVersion()
	}
	return m, nil
}
