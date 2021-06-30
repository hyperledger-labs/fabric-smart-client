/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type CreateAsset struct {
	AssetProperties   *AssetProperties
	PublicDescription string
}

type CreateAssetView struct {
	*CreateAsset
}

func (c *CreateAssetView) Call(context view.Context) (interface{}, error) {
	apRaw, err := c.AssetProperties.Bytes()
	assert.NoError(err, "failed marshalling asset properties struct")

	_, err = context.RunView(
		chaincode.NewInvokeView(
			"asset_transfer",
			"CreateAsset",
			c.AssetProperties.ID,
			c.PublicDescription,
		).WithTransientEntry("asset_properties", apRaw).WithEndorsersFromMyOrg(),
	)
	assert.NoError(err, "failed creating asset")

	return nil, nil
}

type CreateAssetViewFactory struct{}

func (c *CreateAssetViewFactory) NewView(in []byte) (view.View, error) {
	f := &CreateAssetView{CreateAsset: &CreateAsset{}}
	err := json.Unmarshal(in, f.CreateAsset)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}

type ReadAsset struct {
	ID string
}

type ReadAssetView struct {
	*ReadAsset
}

func (r *ReadAssetView) Call(context view.Context) (interface{}, error) {
	res, err := context.RunView(chaincode.NewQueryView("asset_transfer", "ReadAsset", r.ID).WithEndorsersFromMyOrg())
	assert.NoError(err, "failed reading asset")
	return res, nil
}

type ReadAssetViewFactory struct{}

func (p *ReadAssetViewFactory) NewView(in []byte) (view.View, error) {
	f := &ReadAssetView{ReadAsset: &ReadAsset{}}
	err := json.Unmarshal(in, f.ReadAsset)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}

type ReadAssetPrivateProperties struct {
	ID string
}

type ReadAssetPrivatePropertiesView struct {
	*ReadAssetPrivateProperties
}

func (r *ReadAssetPrivatePropertiesView) Call(context view.Context) (interface{}, error) {
	res, err := context.RunView(chaincode.NewQueryView("asset_transfer", "GetAssetPrivateProperties", r.ID).WithEndorsersFromMyOrg())
	assert.NoError(err, "failed getting asset private properties")
	return res, nil
}

type ReadAssetPrivatePropertiesViewFactory struct{}

func (p *ReadAssetPrivatePropertiesViewFactory) NewView(in []byte) (view.View, error) {
	f := &ReadAssetPrivatePropertiesView{ReadAssetPrivateProperties: &ReadAssetPrivateProperties{}}
	err := json.Unmarshal(in, f.ReadAssetPrivateProperties)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}

type ChangePublicDescription struct {
	ID                string
	PublicDescription string
}

type ChangePublicDescriptionView struct {
	*ChangePublicDescription
}

func (r *ChangePublicDescriptionView) Call(context view.Context) (interface{}, error) {
	_, err := context.RunView(
		chaincode.NewInvokeView(
			"asset_transfer",
			"ChangePublicDescription",
			r.ID,
			r.PublicDescription,
		).WithEndorsersFromMyOrg(),
	)
	assert.NoError(err, "failed changing public description")
	return nil, nil
}

type ChangePublicDescriptionViewFactory struct{}

func (p *ChangePublicDescriptionViewFactory) NewView(in []byte) (view.View, error) {
	f := &ChangePublicDescriptionView{ChangePublicDescription: &ChangePublicDescription{}}
	err := json.Unmarshal(in, f.ChangePublicDescription)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}
