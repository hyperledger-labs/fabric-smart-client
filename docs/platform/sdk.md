# SDK

## Overview

An SDK is a set of services that an FSC node can use to implement its logic. An SDK is installed in the `main.go` file of a node in the following manner:
```go
func main() {
	n := fscnode.New()
	if err := n.InstallSDK(iou.NewSDK(n)); err != nil {
		log.Fatalf("failed installing SDK: %v", err)
	}

	n.Execute(func() error {
		registry := viewregistry.GetRegistry(n)
		if err := registry.RegisterFactory("init", &views.ApproverInitViewFactory{}); err != nil {
			return err
		}
		if err := registry.RegisterResponder(&views.ApproverView{}, &views.CreateIOUView{}); err != nil {
			return err
		}
		return registry.RegisterResponder(&views.ApproverView{}, &views.UpdateIOUView{})
	})
}
```

`iou` is `integration/fabric/common/iou`, `views` is its `views` subpackage, `fscnode` is
`node` and `viewregistry` is `platform/view/services/view`.

In this example the services included in the `iou.SDK` can be used by
* the views of the node, i.e. `ApproverView`, `ApproverInitViewFactory`
* other services

The services contained in an SDK depend on other services defined in other SDKs. Hence, each SDK is built on top of other SDKs. As a result, installing and starting an SDK will install and instantiate the whole chain up to the root, so a node does not need to install multiple SDKs.

## Base SDKs

This project provides the following SDKs as a base:
* **View SDK:**
  * Config service
  * View registry
  * Communication with other FSC nodes
  * Identity resolution
  * Signers and verifiers
  * KVS
  * Logging
  * Metrics
  * Event publisher/subscriber
* **Fabric SDK:** Built on top of ViewSDK, it adds following functionality:
  * Config service for Fabric
  * Network provider for connection to the Fabric network
  * Finality handlers
  * Identity providers
  * Endorser transaction handling
  * Vault

## Developing new SDKs

If new functionality is needed for the purposes of an application, this can be added with the definition of a new SDK that builds on top of the SDK(s) that contain the services we depend on.

Hence, by defining a new SDK, we achieve following goals:
* Leverage services provided by other SDKs by combining them and using them as a base
* Define new services that will be used by our views
* Overwrite existing services and modify the behavior of the base SDKs
* Register further drivers, handlers, etc.
* Registering new or existing services to the Service Provider to make them available to the views

A general structure for a new SDK, based on `integration/fabric/common/iou/sdk.go`, is:
```go
package myapp

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	fabricsdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

// Service is a service of myapp that its views use.
type Service interface {
	Do() error
}

type service struct{}

func (*service) Do() error { return nil }

func newService() Service { return &service{} }

type SDK struct {
	dig.SDK
}

func NewSDK(registry services.Registry) *SDK {
	return &SDK{SDK: fabricsdk.NewSDK(registry)}
}

func (p *SDK) Install() error {
	// Optional: provide the services of myapp.
	if err := p.Container().Provide(newService); err != nil {
		return err
	}
	// Install the services of the parent SDKs.
	if err := p.SDK.Install(); err != nil {
		return err
	}
	// Optional: register services, new or from a parent SDK, so that views can get them.
	return errors.Join(
		digutils.Register[Service](p.Container()),
		digutils.Register[state.VaultService](p.Container()),
	)
}

// Optional: extend Start.
func (p *SDK) Start(ctx context.Context) error {
	if err := p.SDK.Start(ctx); err != nil {
		return err
	}
	// Further logic, such as registering handlers or drivers, or calling init views.
	return nil
}

// Optional: extend PostStart.
func (p *SDK) PostStart(ctx context.Context) error {
	if err := p.SDK.PostStart(ctx); err != nil {
		return err
	}
	// Further logic, such as starting services or listeners.
	return nil
}
```

Now the new services can be used from within the views:
```go
func (*MyView) Call(viewCtx view.Context) (any, error) {
	s, err := GetService(viewCtx)
	if err != nil {
		return nil, err
	}
	return nil, s.Do()
}

// GetService returns the Service registered by the SDK.
func GetService(sp services.Provider) (Service, error) {
	s, err := sp.GetService(reflect.TypeFor[Service]())
	if err != nil {
		return nil, err
	}
	service, ok := s.(Service)
	if !ok {
		return nil, errors.Errorf("unexpected service type [%T]", s)
	}
	return service, nil
}
```

Note that multiple SDKs can also be combined and used as a base.
The order in which the SDKs are installed is not relevant to dependency injection, but it may be important for the logic of `Start` and `PostStart`.

For further supported functionality and details on dependency injection, consult the [`dig` documentation](https://github.com/uber-go/dig).