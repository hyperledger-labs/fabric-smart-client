/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package opts

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
)

type Organization struct {
	Network string
	Org     string
}

type Options struct {
	Mapping map[string]interface{}
}

func (o *Options) Role() string {
	res := o.Mapping["Role"]
	if res == nil {
		return ""
	}
	return res.(string)
}

func (o *Options) SetRole(org string) {
	o.Mapping["Role"] = org
}

func Get(o *node.Options) *Options {
	opt, ok := o.Mapping["orion"]
	if !ok {
		opt = &Options{Mapping: map[string]interface{}{}}
		o.Mapping["orion"] = opt
	}
	res, ok := opt.(*Options)
	if ok {
		return res
	}
	mapping, ok := opt.(map[interface{}]interface{})
	if ok {
		return convert(mapping)
	}
	panic("invalid options")
}

func convert(m map[interface{}]interface{}) *Options {
	opts := &Options{
		Mapping: map[string]interface{}{},
	}
	for k, v := range m["mapping"].(map[interface{}]interface{}) {
		opts.Mapping[k.(string)] = v
	}
	return opts
}
