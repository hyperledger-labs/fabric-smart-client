# SDK

## Overview

An SDK is a set of services that an FSC node can use to implement its logic. An SDK is installed in the `main.go` file of a node in the following manner:
```go
func main() {
	n := fscnode.NewEmpty("")
	n.InstallSDK(iou.NewSDK(n))
	
	n.Execute(func() error {
		registry := viewregistry.GetRegistry(n)
		if err := registry.RegisterFactory("init", &views.ApproverInitViewFactory{}); err != nil {
			return err
		}
		registry.RegisterResponder(&views.ApproverView{}, &views.CreateIOUView{})
		registry.RegisterResponder(&views.ApproverView{}, &views.UpdateIOUView{})
		
		return nil
	})
}
```

In this example the services included in the `iou.SDK` can be used by
* the views of the node, i.e. `ApproverView`, `ApproverInitViewFactory`
* other services

The services contained in an SDK depend on other services defined in other SDKs. Hence, each SDK is built on top of other SDKs. As a result, installing and starting an SDK will install and instantiate the whole chain up to the root, and as a result we do not need to install multiple SDKs.

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
* **Orion SDK:** Built on top of ViewSDK, it adds following functionality:
  * Config service for Orion
  * Network provider for connection to the Orion network
  * Finality handlers
  * Identity providers

## Developing new SDKs

If new functionality is needed for the purposes of an application, this can be added with the definition of a new SDK that builds on top of the SDK(s) that contain the services we depend on.

Hence, by defining a new SDK, we achieve following goals:
* Leverage services provided by other SDKs by combining them and using them as a base
* Define new services that will be used by our views
* Overwrite existing services and modify the behavior of the base SDKs
* Register further drivers, handlers, etc.
* Registering new or existing services to the Service Provider to make them available to the views

A general structure for a new SDK would be as follows:
```go
package myapp

import (
	"errors"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fabricsdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/weaver"
)

type SDK struct {
	dig.SDK
}

func NewSDK(registry node.Registry) *SDK {
	return &SDK{SDK: fabricsdk.NewSDK(registry)}
}

func (p *SDK) Install() error {
	// Optional: Install new services relevant to myapp
	if err := errors.Join(
		p.Container().Provide(newService1),
		p.Container().Provide(newService2),
	); err != nil {
		return err
	}
	// Install services from parent SDKs
	if err := p.SDK.Install(); err != nil {
		return err
	}

	// Optional: Register new or existing services (from parent SDKs that haven't been registered), so they can be used by the views.
	return errors.Join(
		digutils.Register[NewService1](p.Container()),
		digutils.Register[ExistingService1](p.Container()),
	)
}

// Optional: Adapt Start logic

func (p *SDK) Start(ctx context.Context) error {
    //Call Start of parent SDKs
    if err := p.SDK.Start(ctx); err != nil {
      return err
    }
  
    // Implement optional further logic, such as registering handlers, drivers, calling init views.
}

// Optional: Adapt PostStart logic

func (p *SDK) PostStart(ctx context.Context) error {
      //Call PostStart of parent SDKs
    if err := p.SDK.PostStart(ctx); err != nil {
      return err
    }
  
    // Implement optional further logic, such as starting services or listeners.
}


func newService1() NewService1 {...}
func newService2() NewService2 {...}
```

Now the new services can be used from within the views:
```go
func (a *MyNewView) Call(context view.Context) (interface{}, error) {
    service1, err := GetService1(context)
    // Further logic
}

func GetService1(ctx view.ServiceProvider) (myapp.Service1, error) {
    s, err := ctx.GetService(reflect.TypeOf((*myapp.Service1)(nil)))
    if err != nil {
    return nil, err
    }
    return s.(myapp.Service1), nil
}
```

Note that multiple SDKs can also be combined and used as a base:
```go
func NewSDK(registry node.Registry) *SDK {
	return &SDK{SDK: fabricsdk.NewFrom(orionsdk.NewFrom(viewsdk.NewSDK(registry)))}
}
```
The order in which the SDKs is not relevant when it comes to dependency injection, but it may be important for the logic of `Start` and `PostStart`.

For further supported functionality and details on dependency injection, consult the [`dig` documentation](https://github.com/uber-go/dig).