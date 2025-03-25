/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package newiou

import (
	"errors"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/newiou/views"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	dig2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/vfsdk"
)

type SDK struct {
	dig2.SDK
}

func NewSDK(registry node.Registry) *SDK {
	return NewFrom(sdk.NewFrom(vfsdk.NewSDK(registry)))
}

func NewFrom(sdk dig2.SDK) *SDK {
	return &SDK{SDK: sdk}
}

func (p *SDK) Install() error {
	err := errors.Join(
		p.Container().Provide(endorser.NewEndorseViewFactory),
		p.Container().Provide(endorser.NewCollectEndorsementsViewFactory),
		p.Container().Provide(state.NewRespondExchangeRecipientIdentitiesViewFactory),
		p.Container().Provide(state.NewExchangeRecipientIdentitiesViewFactory),
		p.Container().Provide(endorser.NewOrderingAndFinalityViewFactory),
		p.Container().Provide(state.NewReceiveTransactionViewFactory),
		p.Container().Provide(state.NewViewFactory),
	)
	if err != nil {
		return err
	}

	return p.SDK.Install()
}

func NewApproverSDK(registry node.Registry) *ApproverSDK {
	return NewApproverSDKFrom(NewSDK(registry))
}

func NewApproverSDKFrom(sdk dig2.SDK) *ApproverSDK {
	return &ApproverSDK{SDK: sdk}
}

type ApproverSDK struct {
	dig2.SDK
}

func (p *ApproverSDK) Install() error {
	err := errors.Join(
		p.Container().Provide(views.NewApproverViewFactory, vfsdk.WithInitiators(&views.CreateIOUView{}, &views.UpdateIOUView{})),
		p.Container().Provide(views.NewApproverInitViewFactory, vfsdk.WithFactoryId("init")),
		p.Container().Provide(endorser.NewFinalityViewFactory, vfsdk.WithFactoryId("finality")),
	)
	if err != nil {
		return err
	}
	return p.SDK.Install()
}

func NewBorrowerSDK(registry node.Registry) *BorrowerSDK {
	return NewBorrowerSDKFrom(NewSDK(registry))
}

func NewBorrowerSDKFrom(sdk dig2.SDK) *BorrowerSDK {
	return &BorrowerSDK{SDK: sdk}
}

type BorrowerSDK struct {
	dig2.SDK
}

func (p *BorrowerSDK) Install() error {
	err := errors.Join(
		p.Container().Provide(views.NewCreateIOUViewFactory, vfsdk.WithFactoryId("create")),
		p.Container().Provide(views.NewUpdateIOUViewFactory, vfsdk.WithFactoryId("update")),
		p.Container().Provide(views.NewQueryViewFactory, vfsdk.WithFactoryId("query")),
		p.Container().Provide(endorser.NewFinalityViewFactory, vfsdk.WithFactoryId("finality")),
	)
	if err != nil {
		return err
	}
	return p.SDK.Install()
}

func NewLenderSDK(registry node.Registry) *LenderSDK {
	return NewLenderSDKFrom(NewSDK(registry))
}

func NewLenderSDKFrom(sdk dig2.SDK) *LenderSDK {
	return &LenderSDK{SDK: sdk}
}

type LenderSDK struct {
	dig2.SDK
}

func (p *LenderSDK) Install() error {
	err := errors.Join(
		p.Container().Provide(views.NewCreateIOUResponderViewFactory, vfsdk.WithInitiators(&views.CreateIOUView{})),
		p.Container().Provide(views.NewUpdateIOUResponderViewFactory, vfsdk.WithInitiators(&views.UpdateIOUView{})),
		p.Container().Provide(views.NewQueryViewFactory, vfsdk.WithFactoryId("query")),
		p.Container().Provide(endorser.NewFinalityViewFactory, vfsdk.WithFactoryId("finality")),
	)
	if err != nil {
		return err
	}
	return p.SDK.Install()
}
