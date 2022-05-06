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

func (o *Options) Organizations() []Organization {
	boxed, ok := o.Mapping["Organization"]
	if !ok {
		return nil
	}
	var res []Organization
	list, ok := boxed.([]interface{})
	if ok {
		for _, entry := range list {
			m := entry.(map[interface{}]interface{})
			res = append(res, Organization{
				Network: m["Network"].(string),
				Org:     m["Org"].(string),
			})
		}
		return res
	}

	list2 := boxed.([]map[string]string)
	for _, entry := range list2 {
		m := entry
		res = append(res, Organization{
			Network: m["Network"],
			Org:     m["Org"],
		})
	}
	return res

}

func (o *Options) AddOrganization(org string) {
	o.AddNetworkOrganization("", org)
}

func (o *Options) AddNetworkOrganization(network, org string) {
	var m []map[string]string
	boxed, ok := o.Mapping["Organization"]
	if !ok {
		m = []map[string]string{}
	} else {
		m = boxed.([]map[string]string)
	}
	m = append(m, map[string]string{
		"Network": network,
		"Org":     org,
	})
	o.Mapping["Organization"] = m
}

func (o *Options) DefaultNetwork() string {
	res := o.Mapping["DefaultNetwork"]
	if res == nil {
		return ""
	}
	return res.(string)
}

func (o *Options) SetDefaultNetwork(defaultNetwork string) {
	o.Mapping["DefaultNetwork"] = defaultNetwork
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

func (o *Options) SetDeliveryEnabled(enabled bool) {
	o.Mapping["DeliveryEnabled"] = enabled
}

func (o *Options) DeliveryEnabled() bool {
	res, ok := o.Mapping["DeliveryEnabled"]
	if res == nil || !ok {
		return true
	}
	return res.(bool)
}

func Get(o *node.Options) *Options {
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
