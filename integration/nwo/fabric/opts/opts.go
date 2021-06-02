/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package opts

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

type Options struct {
	Mapping map[string]interface{}
}

func (o *Options) Organization() string {
	res := o.Mapping["Organization"]
	if res == nil {
		return ""
	}
	return res.(string)
}

func (o *Options) SetOrganization(org string) {
	o.Mapping["Organization"] = org
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

func (o *Options) AnonymousIdentity() bool {
	res := o.Mapping["AnonymousIdentity"]
	if res == nil {
		return false
	}
	return res.(bool)
}

func (o *Options) SetAnonymousIdentity(v bool) {
	o.Mapping["AnonymousIdentity"] = v
}

func (o *Options) X509Identities() []string {
	boxed := o.Mapping["X509Identities"]
	if boxed == nil {
		return nil
	}
	res, ok := boxed.([]string)
	if ok {
		return res
	}
	res = []string{}
	for _, v := range boxed.([]interface{}) {
		res = append(res, v.(string))
	}
	return res
}

func (o *Options) SetX509Identities(ids []string) {
	o.Mapping["X509Identities"] = ids
}

func (o *Options) IdemixIdentities() []string {
	boxed := o.Mapping["IdemixIdentities"]
	if boxed == nil {
		return nil
	}
	res, ok := boxed.([]string)
	if ok {
		return res
	}
	res = []string{}
	for _, v := range boxed.([]interface{}) {
		res = append(res, v.(string))
	}
	return res
}

func (o *Options) SetIdemixIdentities(ids []string) {
	o.Mapping["IdemixIdentities"] = ids
}

func Get(o *fsc.Options) *Options {
	opt, ok := o.Mapping["fabric"]
	if !ok {
		opt = &Options{Mapping: map[string]interface{}{}}
		o.Mapping["fabric"] = opt
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
