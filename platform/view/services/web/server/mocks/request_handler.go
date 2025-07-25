// Code generated by counterfeiter. DO NOT EDIT.
package mocks

import (
	"sync"

	web2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
)

type FakeRequestHandler struct {
	HandleRequestStub        func(*web2.ReqContext) (interface{}, int)
	handleRequestMutex       sync.RWMutex
	handleRequestArgsForCall []struct {
		arg1 *web2.ReqContext
	}
	handleRequestReturns struct {
		result1 interface{}
		result2 int
	}
	handleRequestReturnsOnCall map[int]struct {
		result1 interface{}
		result2 int
	}
	ParsePayloadStub        func([]byte) (interface{}, error)
	parsePayloadMutex       sync.RWMutex
	parsePayloadArgsForCall []struct {
		arg1 []byte
	}
	parsePayloadReturns struct {
		result1 interface{}
		result2 error
	}
	parsePayloadReturnsOnCall map[int]struct {
		result1 interface{}
		result2 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *FakeRequestHandler) HandleRequest(arg1 *web2.ReqContext) (interface{}, int) {
	fake.handleRequestMutex.Lock()
	ret, specificReturn := fake.handleRequestReturnsOnCall[len(fake.handleRequestArgsForCall)]
	fake.handleRequestArgsForCall = append(fake.handleRequestArgsForCall, struct {
		arg1 *web2.ReqContext
	}{arg1})
	fake.recordInvocation("HandleRequest", []interface{}{arg1})
	fake.handleRequestMutex.Unlock()
	if fake.HandleRequestStub != nil {
		return fake.HandleRequestStub(arg1)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	fakeReturns := fake.handleRequestReturns
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *FakeRequestHandler) HandleRequestCallCount() int {
	fake.handleRequestMutex.RLock()
	defer fake.handleRequestMutex.RUnlock()
	return len(fake.handleRequestArgsForCall)
}

func (fake *FakeRequestHandler) HandleRequestCalls(stub func(*web2.ReqContext) (interface{}, int)) {
	fake.handleRequestMutex.Lock()
	defer fake.handleRequestMutex.Unlock()
	fake.HandleRequestStub = stub
}

func (fake *FakeRequestHandler) HandleRequestArgsForCall(i int) *web2.ReqContext {
	fake.handleRequestMutex.RLock()
	defer fake.handleRequestMutex.RUnlock()
	argsForCall := fake.handleRequestArgsForCall[i]
	return argsForCall.arg1
}

func (fake *FakeRequestHandler) HandleRequestReturns(result1 interface{}, result2 int) {
	fake.handleRequestMutex.Lock()
	defer fake.handleRequestMutex.Unlock()
	fake.HandleRequestStub = nil
	fake.handleRequestReturns = struct {
		result1 interface{}
		result2 int
	}{result1, result2}
}

func (fake *FakeRequestHandler) HandleRequestReturnsOnCall(i int, result1 interface{}, result2 int) {
	fake.handleRequestMutex.Lock()
	defer fake.handleRequestMutex.Unlock()
	fake.HandleRequestStub = nil
	if fake.handleRequestReturnsOnCall == nil {
		fake.handleRequestReturnsOnCall = make(map[int]struct {
			result1 interface{}
			result2 int
		})
	}
	fake.handleRequestReturnsOnCall[i] = struct {
		result1 interface{}
		result2 int
	}{result1, result2}
}

func (fake *FakeRequestHandler) ParsePayload(arg1 []byte) (interface{}, error) {
	var arg1Copy []byte
	if arg1 != nil {
		arg1Copy = make([]byte, len(arg1))
		copy(arg1Copy, arg1)
	}
	fake.parsePayloadMutex.Lock()
	ret, specificReturn := fake.parsePayloadReturnsOnCall[len(fake.parsePayloadArgsForCall)]
	fake.parsePayloadArgsForCall = append(fake.parsePayloadArgsForCall, struct {
		arg1 []byte
	}{arg1Copy})
	fake.recordInvocation("ParsePayload", []interface{}{arg1Copy})
	fake.parsePayloadMutex.Unlock()
	if fake.ParsePayloadStub != nil {
		return fake.ParsePayloadStub(arg1)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	fakeReturns := fake.parsePayloadReturns
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *FakeRequestHandler) ParsePayloadCallCount() int {
	fake.parsePayloadMutex.RLock()
	defer fake.parsePayloadMutex.RUnlock()
	return len(fake.parsePayloadArgsForCall)
}

func (fake *FakeRequestHandler) ParsePayloadCalls(stub func([]byte) (interface{}, error)) {
	fake.parsePayloadMutex.Lock()
	defer fake.parsePayloadMutex.Unlock()
	fake.ParsePayloadStub = stub
}

func (fake *FakeRequestHandler) ParsePayloadArgsForCall(i int) []byte {
	fake.parsePayloadMutex.RLock()
	defer fake.parsePayloadMutex.RUnlock()
	argsForCall := fake.parsePayloadArgsForCall[i]
	return argsForCall.arg1
}

func (fake *FakeRequestHandler) ParsePayloadReturns(result1 interface{}, result2 error) {
	fake.parsePayloadMutex.Lock()
	defer fake.parsePayloadMutex.Unlock()
	fake.ParsePayloadStub = nil
	fake.parsePayloadReturns = struct {
		result1 interface{}
		result2 error
	}{result1, result2}
}

func (fake *FakeRequestHandler) ParsePayloadReturnsOnCall(i int, result1 interface{}, result2 error) {
	fake.parsePayloadMutex.Lock()
	defer fake.parsePayloadMutex.Unlock()
	fake.ParsePayloadStub = nil
	if fake.parsePayloadReturnsOnCall == nil {
		fake.parsePayloadReturnsOnCall = make(map[int]struct {
			result1 interface{}
			result2 error
		})
	}
	fake.parsePayloadReturnsOnCall[i] = struct {
		result1 interface{}
		result2 error
	}{result1, result2}
}

func (fake *FakeRequestHandler) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.handleRequestMutex.RLock()
	defer fake.handleRequestMutex.RUnlock()
	fake.parsePayloadMutex.RLock()
	defer fake.parsePayloadMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *FakeRequestHandler) recordInvocation(key string, args []interface{}) {
	fake.invocationsMutex.Lock()
	defer fake.invocationsMutex.Unlock()
	if fake.invocations == nil {
		fake.invocations = map[string][][]interface{}{}
	}
	if fake.invocations[key] == nil {
		fake.invocations[key] = [][]interface{}{}
	}
	fake.invocations[key] = append(fake.invocations[key], args)
}

var _ web2.RequestHandler = new(FakeRequestHandler)
