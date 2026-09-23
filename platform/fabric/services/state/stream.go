/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"slices"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/rwset"
)

func IDHasPrefixFilter(prefix string) func(k ID) bool {
	return func(k ID) bool {
		return k.HasPrefix(prefix)
	}
}

func InputHasIDPrefixFilter(prefix string) func(i *input) bool {
	return func(i *input) bool {
		return i.key.HasPrefix(prefix)
	}
}

type ID string

func (k ID) HasPrefix(prefix string) bool {
	p, attrs, err := rwset.SplitCompositeKey(string(k))
	if err != nil {
		return false
	}
	if len(attrs) == 0 {
		return false
	}

	return p == prefix
}

type IDs []ID

func (k IDs) Count() int {
	return len(k)
}

func (k IDs) Match(keys IDs) bool {
	if len(k) != len(keys) {
		return false
	}
	for _, id := range k {
		found := slices.Contains(keys, id)
		if !found {
			return false
		}
	}

	return true
}

func (k IDs) Filter(f func(k ID) bool) IDs {
	var filtered IDs
	for _, output := range k {
		if f(output) {
			filtered = append(filtered, output)
		}
	}
	return filtered
}

type Namespaces []string

func (k Namespaces) Count() int {
	return len(k)
}

func (k Namespaces) Match(keys Namespaces) bool {
	if len(k) != len(keys) {
		return false
	}
	for _, id := range k {
		found := slices.Contains(keys, id)
		if !found {
			return false
		}
	}

	return true
}

func (k Namespaces) Filter(f func(k string) bool) Namespaces {
	var filtered Namespaces
	for _, output := range k {
		if f(output) {
			filtered = append(filtered, output)
		}
	}
	return filtered
}

type output struct {
	namespace *Namespace
	key       ID
	index     int
	delete    bool
}

// State unmarshals the output into state. A nil receiver, as returned by
// outputStream.At for an out-of-range position, is reported as an error.
func (o *output) State(state any) error {
	if o == nil {
		return errors.New("output not found")
	}
	return o.namespace.GetOutputAt(o.index, state)
}

func (o *output) IsDelete() bool {
	return o.delete
}

func (o *output) ID() ID {
	return o.key
}

type outputStream struct {
	namespace *Namespace
	outputs   []*output
}

// Filter returns a stream of output filtered applying the passed filter
func (o *outputStream) Filter(f func(t *output) bool) *outputStream {
	var filtered []*output
	for _, output := range o.outputs {
		if f(output) {
			filtered = append(filtered, output)
		}
	}
	return &outputStream{namespace: o.namespace, outputs: filtered}
}

// Deleted returns the outputs that are deletes
func (o *outputStream) Deleted() *outputStream {
	var filtered []*output
	for _, output := range o.outputs {
		if output.delete {
			filtered = append(filtered, output)
		}
	}
	return &outputStream{namespace: o.namespace, outputs: filtered}
}

// Written returns the outputs that are not deletes
func (o *outputStream) Written() *outputStream {
	var filtered []*output
	for _, output := range o.outputs {
		if !output.delete {
			filtered = append(filtered, output)
		}
	}
	return &outputStream{namespace: o.namespace, outputs: filtered}
}

// Count returns the number of outputs in this stream
func (o *outputStream) Count() int {
	return len(o.outputs)
}

// At returns the output at the passed position, or nil if it is out of range.
// State reports a nil receiver as an error, but ID and IsDelete dereference it,
// so check the result before calling those.
func (o *outputStream) At(index int) *output {
	if index < 0 || index >= len(o.outputs) {
		return nil
	}
	return o.outputs[index]
}

// IDs returns the IDs of the outputs in this stream
func (o *outputStream) IDs() IDs {
	var filtered []ID
	for _, output := range o.outputs {
		filtered = append(filtered, output.key)
	}
	return filtered
}

type input struct {
	namespace *Namespace
	index     int
	key       ID
}

// VerifyCertification verifies this input's certification. A nil receiver is
// reported as an error.
func (i *input) VerifyCertification() error {
	if i == nil {
		return errors.New("input not found")
	}
	return i.namespace.VerifyInputCertificationAt(i.index, string(i.key))
}

// State unmarshals the input into state. A nil receiver is reported as an error.
func (i *input) State(state any) error {
	if i == nil {
		return errors.New("input not found")
	}
	return i.namespace.GetInputAt(i.index, state)
}

func (i *input) ID() ID {
	return i.key
}

type inputStream struct {
	namespace *Namespace
	inputs    []*input
}

// Filter returns a stream of inputs filtered applying the passed filter
func (o *inputStream) Filter(f func(t *input) bool) *inputStream {
	var filtered []*input
	for _, output := range o.inputs {
		if f(output) {
			filtered = append(filtered, output)
		}
	}
	return &inputStream{namespace: o.namespace, inputs: filtered}
}

// Count returns the number of inputs in this stream
func (o *inputStream) Count() int {
	return len(o.inputs)
}

// At returns the input at the passed position, or nil if it is out of range.
// See outputStream.At.
func (o *inputStream) At(i int) *input {
	if i < 0 || i >= len(o.inputs) {
		return nil
	}
	return o.inputs[i]
}

// IDs returns the IDs of the inputs in this stream
func (o *inputStream) IDs() IDs {
	var filtered []ID
	for _, i := range o.inputs {
		filtered = append(filtered, i.key)
	}
	return filtered
}

type commandStream struct {
	namespace *Namespace
	commands  []*Command
}

// Filter returns a stream of commands filtered applying the passed filter
func (o *commandStream) Filter(f func(t *Command) bool) *commandStream {
	var filtered []*Command
	for _, command := range o.commands {
		if f(command) {
			filtered = append(filtered, command)
		}
	}
	return &commandStream{namespace: o.namespace, commands: filtered}
}

// Count returns the number of commands in this stream
func (o *commandStream) Count() int {
	return len(o.commands)
}

// At returns the command at the passed position, or nil if it is out of range.
// See outputStream.At.
func (o *commandStream) At(i int) *Command {
	if i < 0 || i >= len(o.commands) {
		return nil
	}
	return o.commands[i]
}
