/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// NamespacePoliciesResult is the (JSON-serializable) output of NamespacePoliciesView.
// It reports, per namespace, the version of the registered endorsement policy.
type NamespacePoliciesResult struct {
	// Versions maps a namespace to the version of its registered policy.
	Versions map[string]uint64
}

// NamespacePoliciesView queries the endorsement policies currently registered
// for every namespace via the query service and returns their versions keyed
// by namespace.
type NamespacePoliciesView struct{}

func (v *NamespacePoliciesView) Call(viewCtx view.Context) (any, error) {
	network, ch, err := fabric.GetDefaultChannel(viewCtx)
	if err != nil {
		return nil, err
	}

	qs, err := queryservice.GetQueryService(viewCtx, network.Name(), ch.Name())
	if err != nil {
		return nil, err
	}

	policies, err := qs.GetNamespacePolicies()
	if err != nil {
		return nil, err
	}

	res := NamespacePoliciesResult{Versions: make(map[string]uint64, len(policies.GetPolicies()))}
	for _, p := range policies.GetPolicies() {
		res.Versions[p.GetNamespace()] = p.GetVersion()
	}

	return res, nil
}

type NamespacePoliciesViewFactory struct{}

func (c *NamespacePoliciesViewFactory) NewView([]byte) (view.View, error) {
	return &NamespacePoliciesView{}, nil
}
