// Code generated by counterfeiter. DO NOT EDIT.
package mock

import (
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/identity"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
)

type EndpointService struct {
	AddPublicKeyExtractorStub        func(driver.PublicKeyExtractor) error
	addPublicKeyExtractorMutex       sync.RWMutex
	addPublicKeyExtractorArgsForCall []struct {
		arg1 driver.PublicKeyExtractor
	}
	addPublicKeyExtractorReturns struct {
		result1 error
	}
	addPublicKeyExtractorReturnsOnCall map[int]struct {
		result1 error
	}
	AddResolverStub        func(string, string, map[string]string, []string, []byte) (identity.Identity, error)
	addResolverMutex       sync.RWMutex
	addResolverArgsForCall []struct {
		arg1 string
		arg2 string
		arg3 map[string]string
		arg4 []string
		arg5 []byte
	}
	addResolverReturns struct {
		result1 identity.Identity
		result2 error
	}
	addResolverReturnsOnCall map[int]struct {
		result1 identity.Identity
		result2 error
	}
	BindStub        func(identity.Identity, identity.Identity) error
	bindMutex       sync.RWMutex
	bindArgsForCall []struct {
		arg1 identity.Identity
		arg2 identity.Identity
	}
	bindReturns struct {
		result1 error
	}
	bindReturnsOnCall map[int]struct {
		result1 error
	}
	GetIdentityStub        func(string, []byte) (identity.Identity, error)
	getIdentityMutex       sync.RWMutex
	getIdentityArgsForCall []struct {
		arg1 string
		arg2 []byte
	}
	getIdentityReturns struct {
		result1 identity.Identity
		result2 error
	}
	getIdentityReturnsOnCall map[int]struct {
		result1 identity.Identity
		result2 error
	}
	GetResolverStub        func(identity.Identity) (driver.Resolver, error)
	getResolverMutex       sync.RWMutex
	getResolverArgsForCall []struct {
		arg1 identity.Identity
	}
	getResolverReturns struct {
		result1 driver.Resolver
		result2 error
	}
	getResolverReturnsOnCall map[int]struct {
		result1 driver.Resolver
		result2 error
	}
	IsBoundToStub        func(identity.Identity, identity.Identity) bool
	isBoundToMutex       sync.RWMutex
	isBoundToArgsForCall []struct {
		arg1 identity.Identity
		arg2 identity.Identity
	}
	isBoundToReturns struct {
		result1 bool
	}
	isBoundToReturnsOnCall map[int]struct {
		result1 bool
	}
	ResolveStub        func(identity.Identity) (driver.Resolver, []byte, error)
	resolveMutex       sync.RWMutex
	resolveArgsForCall []struct {
		arg1 identity.Identity
	}
	resolveReturns struct {
		result1 driver.Resolver
		result2 []byte
		result3 error
	}
	resolveReturnsOnCall map[int]struct {
		result1 driver.Resolver
		result2 []byte
		result3 error
	}
	SetPublicKeyIDSynthesizerStub        func(driver.PublicKeyIDSynthesizer)
	setPublicKeyIDSynthesizerMutex       sync.RWMutex
	setPublicKeyIDSynthesizerArgsForCall []struct {
		arg1 driver.PublicKeyIDSynthesizer
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *EndpointService) AddPublicKeyExtractor(arg1 driver.PublicKeyExtractor) error {
	fake.addPublicKeyExtractorMutex.Lock()
	ret, specificReturn := fake.addPublicKeyExtractorReturnsOnCall[len(fake.addPublicKeyExtractorArgsForCall)]
	fake.addPublicKeyExtractorArgsForCall = append(fake.addPublicKeyExtractorArgsForCall, struct {
		arg1 driver.PublicKeyExtractor
	}{arg1})
	stub := fake.AddPublicKeyExtractorStub
	fakeReturns := fake.addPublicKeyExtractorReturns
	fake.recordInvocation("AddPublicKeyExtractor", []interface{}{arg1})
	fake.addPublicKeyExtractorMutex.Unlock()
	if stub != nil {
		return stub(arg1)
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *EndpointService) AddPublicKeyExtractorCallCount() int {
	fake.addPublicKeyExtractorMutex.RLock()
	defer fake.addPublicKeyExtractorMutex.RUnlock()
	return len(fake.addPublicKeyExtractorArgsForCall)
}

func (fake *EndpointService) AddPublicKeyExtractorCalls(stub func(driver.PublicKeyExtractor) error) {
	fake.addPublicKeyExtractorMutex.Lock()
	defer fake.addPublicKeyExtractorMutex.Unlock()
	fake.AddPublicKeyExtractorStub = stub
}

func (fake *EndpointService) AddPublicKeyExtractorArgsForCall(i int) driver.PublicKeyExtractor {
	fake.addPublicKeyExtractorMutex.RLock()
	defer fake.addPublicKeyExtractorMutex.RUnlock()
	argsForCall := fake.addPublicKeyExtractorArgsForCall[i]
	return argsForCall.arg1
}

func (fake *EndpointService) AddPublicKeyExtractorReturns(result1 error) {
	fake.addPublicKeyExtractorMutex.Lock()
	defer fake.addPublicKeyExtractorMutex.Unlock()
	fake.AddPublicKeyExtractorStub = nil
	fake.addPublicKeyExtractorReturns = struct {
		result1 error
	}{result1}
}

func (fake *EndpointService) AddPublicKeyExtractorReturnsOnCall(i int, result1 error) {
	fake.addPublicKeyExtractorMutex.Lock()
	defer fake.addPublicKeyExtractorMutex.Unlock()
	fake.AddPublicKeyExtractorStub = nil
	if fake.addPublicKeyExtractorReturnsOnCall == nil {
		fake.addPublicKeyExtractorReturnsOnCall = make(map[int]struct {
			result1 error
		})
	}
	fake.addPublicKeyExtractorReturnsOnCall[i] = struct {
		result1 error
	}{result1}
}

func (fake *EndpointService) AddResolver(arg1 string, arg2 string, arg3 map[string]string, arg4 []string, arg5 []byte) (identity.Identity, error) {
	var arg4Copy []string
	if arg4 != nil {
		arg4Copy = make([]string, len(arg4))
		copy(arg4Copy, arg4)
	}
	var arg5Copy []byte
	if arg5 != nil {
		arg5Copy = make([]byte, len(arg5))
		copy(arg5Copy, arg5)
	}
	fake.addResolverMutex.Lock()
	ret, specificReturn := fake.addResolverReturnsOnCall[len(fake.addResolverArgsForCall)]
	fake.addResolverArgsForCall = append(fake.addResolverArgsForCall, struct {
		arg1 string
		arg2 string
		arg3 map[string]string
		arg4 []string
		arg5 []byte
	}{arg1, arg2, arg3, arg4Copy, arg5Copy})
	stub := fake.AddResolverStub
	fakeReturns := fake.addResolverReturns
	fake.recordInvocation("AddResolver", []interface{}{arg1, arg2, arg3, arg4Copy, arg5Copy})
	fake.addResolverMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3, arg4, arg5)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *EndpointService) AddResolverCallCount() int {
	fake.addResolverMutex.RLock()
	defer fake.addResolverMutex.RUnlock()
	return len(fake.addResolverArgsForCall)
}

func (fake *EndpointService) AddResolverCalls(stub func(string, string, map[string]string, []string, []byte) (identity.Identity, error)) {
	fake.addResolverMutex.Lock()
	defer fake.addResolverMutex.Unlock()
	fake.AddResolverStub = stub
}

func (fake *EndpointService) AddResolverArgsForCall(i int) (string, string, map[string]string, []string, []byte) {
	fake.addResolverMutex.RLock()
	defer fake.addResolverMutex.RUnlock()
	argsForCall := fake.addResolverArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3, argsForCall.arg4, argsForCall.arg5
}

func (fake *EndpointService) AddResolverReturns(result1 identity.Identity, result2 error) {
	fake.addResolverMutex.Lock()
	defer fake.addResolverMutex.Unlock()
	fake.AddResolverStub = nil
	fake.addResolverReturns = struct {
		result1 identity.Identity
		result2 error
	}{result1, result2}
}

func (fake *EndpointService) AddResolverReturnsOnCall(i int, result1 identity.Identity, result2 error) {
	fake.addResolverMutex.Lock()
	defer fake.addResolverMutex.Unlock()
	fake.AddResolverStub = nil
	if fake.addResolverReturnsOnCall == nil {
		fake.addResolverReturnsOnCall = make(map[int]struct {
			result1 identity.Identity
			result2 error
		})
	}
	fake.addResolverReturnsOnCall[i] = struct {
		result1 identity.Identity
		result2 error
	}{result1, result2}
}

func (fake *EndpointService) Bind(arg1 identity.Identity, arg2 identity.Identity) error {
	fake.bindMutex.Lock()
	ret, specificReturn := fake.bindReturnsOnCall[len(fake.bindArgsForCall)]
	fake.bindArgsForCall = append(fake.bindArgsForCall, struct {
		arg1 identity.Identity
		arg2 identity.Identity
	}{arg1, arg2})
	stub := fake.BindStub
	fakeReturns := fake.bindReturns
	fake.recordInvocation("Bind", []interface{}{arg1, arg2})
	fake.bindMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2)
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *EndpointService) BindCallCount() int {
	fake.bindMutex.RLock()
	defer fake.bindMutex.RUnlock()
	return len(fake.bindArgsForCall)
}

func (fake *EndpointService) BindCalls(stub func(identity.Identity, identity.Identity) error) {
	fake.bindMutex.Lock()
	defer fake.bindMutex.Unlock()
	fake.BindStub = stub
}

func (fake *EndpointService) BindArgsForCall(i int) (identity.Identity, identity.Identity) {
	fake.bindMutex.RLock()
	defer fake.bindMutex.RUnlock()
	argsForCall := fake.bindArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2
}

func (fake *EndpointService) BindReturns(result1 error) {
	fake.bindMutex.Lock()
	defer fake.bindMutex.Unlock()
	fake.BindStub = nil
	fake.bindReturns = struct {
		result1 error
	}{result1}
}

func (fake *EndpointService) BindReturnsOnCall(i int, result1 error) {
	fake.bindMutex.Lock()
	defer fake.bindMutex.Unlock()
	fake.BindStub = nil
	if fake.bindReturnsOnCall == nil {
		fake.bindReturnsOnCall = make(map[int]struct {
			result1 error
		})
	}
	fake.bindReturnsOnCall[i] = struct {
		result1 error
	}{result1}
}

func (fake *EndpointService) GetIdentity(arg1 string, arg2 []byte) (identity.Identity, error) {
	var arg2Copy []byte
	if arg2 != nil {
		arg2Copy = make([]byte, len(arg2))
		copy(arg2Copy, arg2)
	}
	fake.getIdentityMutex.Lock()
	ret, specificReturn := fake.getIdentityReturnsOnCall[len(fake.getIdentityArgsForCall)]
	fake.getIdentityArgsForCall = append(fake.getIdentityArgsForCall, struct {
		arg1 string
		arg2 []byte
	}{arg1, arg2Copy})
	stub := fake.GetIdentityStub
	fakeReturns := fake.getIdentityReturns
	fake.recordInvocation("GetIdentity", []interface{}{arg1, arg2Copy})
	fake.getIdentityMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *EndpointService) GetIdentityCallCount() int {
	fake.getIdentityMutex.RLock()
	defer fake.getIdentityMutex.RUnlock()
	return len(fake.getIdentityArgsForCall)
}

func (fake *EndpointService) GetIdentityCalls(stub func(string, []byte) (identity.Identity, error)) {
	fake.getIdentityMutex.Lock()
	defer fake.getIdentityMutex.Unlock()
	fake.GetIdentityStub = stub
}

func (fake *EndpointService) GetIdentityArgsForCall(i int) (string, []byte) {
	fake.getIdentityMutex.RLock()
	defer fake.getIdentityMutex.RUnlock()
	argsForCall := fake.getIdentityArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2
}

func (fake *EndpointService) GetIdentityReturns(result1 identity.Identity, result2 error) {
	fake.getIdentityMutex.Lock()
	defer fake.getIdentityMutex.Unlock()
	fake.GetIdentityStub = nil
	fake.getIdentityReturns = struct {
		result1 identity.Identity
		result2 error
	}{result1, result2}
}

func (fake *EndpointService) GetIdentityReturnsOnCall(i int, result1 identity.Identity, result2 error) {
	fake.getIdentityMutex.Lock()
	defer fake.getIdentityMutex.Unlock()
	fake.GetIdentityStub = nil
	if fake.getIdentityReturnsOnCall == nil {
		fake.getIdentityReturnsOnCall = make(map[int]struct {
			result1 identity.Identity
			result2 error
		})
	}
	fake.getIdentityReturnsOnCall[i] = struct {
		result1 identity.Identity
		result2 error
	}{result1, result2}
}

func (fake *EndpointService) GetResolver(arg1 identity.Identity) (driver.Resolver, error) {
	fake.getResolverMutex.Lock()
	ret, specificReturn := fake.getResolverReturnsOnCall[len(fake.getResolverArgsForCall)]
	fake.getResolverArgsForCall = append(fake.getResolverArgsForCall, struct {
		arg1 identity.Identity
	}{arg1})
	stub := fake.GetResolverStub
	fakeReturns := fake.getResolverReturns
	fake.recordInvocation("GetResolver", []interface{}{arg1})
	fake.getResolverMutex.Unlock()
	if stub != nil {
		return stub(arg1)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fakeReturns.result1, fakeReturns.result2
}

func (fake *EndpointService) GetResolverCallCount() int {
	fake.getResolverMutex.RLock()
	defer fake.getResolverMutex.RUnlock()
	return len(fake.getResolverArgsForCall)
}

func (fake *EndpointService) GetResolverCalls(stub func(identity.Identity) (driver.Resolver, error)) {
	fake.getResolverMutex.Lock()
	defer fake.getResolverMutex.Unlock()
	fake.GetResolverStub = stub
}

func (fake *EndpointService) GetResolverArgsForCall(i int) identity.Identity {
	fake.getResolverMutex.RLock()
	defer fake.getResolverMutex.RUnlock()
	argsForCall := fake.getResolverArgsForCall[i]
	return argsForCall.arg1
}

func (fake *EndpointService) GetResolverReturns(result1 driver.Resolver, result2 error) {
	fake.getResolverMutex.Lock()
	defer fake.getResolverMutex.Unlock()
	fake.GetResolverStub = nil
	fake.getResolverReturns = struct {
		result1 driver.Resolver
		result2 error
	}{result1, result2}
}

func (fake *EndpointService) GetResolverReturnsOnCall(i int, result1 driver.Resolver, result2 error) {
	fake.getResolverMutex.Lock()
	defer fake.getResolverMutex.Unlock()
	fake.GetResolverStub = nil
	if fake.getResolverReturnsOnCall == nil {
		fake.getResolverReturnsOnCall = make(map[int]struct {
			result1 driver.Resolver
			result2 error
		})
	}
	fake.getResolverReturnsOnCall[i] = struct {
		result1 driver.Resolver
		result2 error
	}{result1, result2}
}

func (fake *EndpointService) IsBoundTo(arg1 identity.Identity, arg2 identity.Identity) bool {
	fake.isBoundToMutex.Lock()
	ret, specificReturn := fake.isBoundToReturnsOnCall[len(fake.isBoundToArgsForCall)]
	fake.isBoundToArgsForCall = append(fake.isBoundToArgsForCall, struct {
		arg1 identity.Identity
		arg2 identity.Identity
	}{arg1, arg2})
	stub := fake.IsBoundToStub
	fakeReturns := fake.isBoundToReturns
	fake.recordInvocation("IsBoundTo", []interface{}{arg1, arg2})
	fake.isBoundToMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2)
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *EndpointService) IsBoundToCallCount() int {
	fake.isBoundToMutex.RLock()
	defer fake.isBoundToMutex.RUnlock()
	return len(fake.isBoundToArgsForCall)
}

func (fake *EndpointService) IsBoundToCalls(stub func(identity.Identity, identity.Identity) bool) {
	fake.isBoundToMutex.Lock()
	defer fake.isBoundToMutex.Unlock()
	fake.IsBoundToStub = stub
}

func (fake *EndpointService) IsBoundToArgsForCall(i int) (identity.Identity, identity.Identity) {
	fake.isBoundToMutex.RLock()
	defer fake.isBoundToMutex.RUnlock()
	argsForCall := fake.isBoundToArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2
}

func (fake *EndpointService) IsBoundToReturns(result1 bool) {
	fake.isBoundToMutex.Lock()
	defer fake.isBoundToMutex.Unlock()
	fake.IsBoundToStub = nil
	fake.isBoundToReturns = struct {
		result1 bool
	}{result1}
}

func (fake *EndpointService) IsBoundToReturnsOnCall(i int, result1 bool) {
	fake.isBoundToMutex.Lock()
	defer fake.isBoundToMutex.Unlock()
	fake.IsBoundToStub = nil
	if fake.isBoundToReturnsOnCall == nil {
		fake.isBoundToReturnsOnCall = make(map[int]struct {
			result1 bool
		})
	}
	fake.isBoundToReturnsOnCall[i] = struct {
		result1 bool
	}{result1}
}

func (fake *EndpointService) Resolve(arg1 identity.Identity) (driver.Resolver, []byte, error) {
	fake.resolveMutex.Lock()
	ret, specificReturn := fake.resolveReturnsOnCall[len(fake.resolveArgsForCall)]
	fake.resolveArgsForCall = append(fake.resolveArgsForCall, struct {
		arg1 identity.Identity
	}{arg1})
	stub := fake.ResolveStub
	fakeReturns := fake.resolveReturns
	fake.recordInvocation("Resolve", []interface{}{arg1})
	fake.resolveMutex.Unlock()
	if stub != nil {
		return stub(arg1)
	}
	if specificReturn {
		return ret.result1, ret.result2, ret.result3
	}
	return fakeReturns.result1, fakeReturns.result2, fakeReturns.result3
}

func (fake *EndpointService) ResolveCallCount() int {
	fake.resolveMutex.RLock()
	defer fake.resolveMutex.RUnlock()
	return len(fake.resolveArgsForCall)
}

func (fake *EndpointService) ResolveCalls(stub func(identity.Identity) (driver.Resolver, []byte, error)) {
	fake.resolveMutex.Lock()
	defer fake.resolveMutex.Unlock()
	fake.ResolveStub = stub
}

func (fake *EndpointService) ResolveArgsForCall(i int) identity.Identity {
	fake.resolveMutex.RLock()
	defer fake.resolveMutex.RUnlock()
	argsForCall := fake.resolveArgsForCall[i]
	return argsForCall.arg1
}

func (fake *EndpointService) ResolveReturns(result1 driver.Resolver, result2 []byte, result3 error) {
	fake.resolveMutex.Lock()
	defer fake.resolveMutex.Unlock()
	fake.ResolveStub = nil
	fake.resolveReturns = struct {
		result1 driver.Resolver
		result2 []byte
		result3 error
	}{result1, result2, result3}
}

func (fake *EndpointService) ResolveReturnsOnCall(i int, result1 driver.Resolver, result2 []byte, result3 error) {
	fake.resolveMutex.Lock()
	defer fake.resolveMutex.Unlock()
	fake.ResolveStub = nil
	if fake.resolveReturnsOnCall == nil {
		fake.resolveReturnsOnCall = make(map[int]struct {
			result1 driver.Resolver
			result2 []byte
			result3 error
		})
	}
	fake.resolveReturnsOnCall[i] = struct {
		result1 driver.Resolver
		result2 []byte
		result3 error
	}{result1, result2, result3}
}

func (fake *EndpointService) SetPublicKeyIDSynthesizer(arg1 driver.PublicKeyIDSynthesizer) {
	fake.setPublicKeyIDSynthesizerMutex.Lock()
	fake.setPublicKeyIDSynthesizerArgsForCall = append(fake.setPublicKeyIDSynthesizerArgsForCall, struct {
		arg1 driver.PublicKeyIDSynthesizer
	}{arg1})
	stub := fake.SetPublicKeyIDSynthesizerStub
	fake.recordInvocation("SetPublicKeyIDSynthesizer", []interface{}{arg1})
	fake.setPublicKeyIDSynthesizerMutex.Unlock()
	if stub != nil {
		fake.SetPublicKeyIDSynthesizerStub(arg1)
	}
}

func (fake *EndpointService) SetPublicKeyIDSynthesizerCallCount() int {
	fake.setPublicKeyIDSynthesizerMutex.RLock()
	defer fake.setPublicKeyIDSynthesizerMutex.RUnlock()
	return len(fake.setPublicKeyIDSynthesizerArgsForCall)
}

func (fake *EndpointService) SetPublicKeyIDSynthesizerCalls(stub func(driver.PublicKeyIDSynthesizer)) {
	fake.setPublicKeyIDSynthesizerMutex.Lock()
	defer fake.setPublicKeyIDSynthesizerMutex.Unlock()
	fake.SetPublicKeyIDSynthesizerStub = stub
}

func (fake *EndpointService) SetPublicKeyIDSynthesizerArgsForCall(i int) driver.PublicKeyIDSynthesizer {
	fake.setPublicKeyIDSynthesizerMutex.RLock()
	defer fake.setPublicKeyIDSynthesizerMutex.RUnlock()
	argsForCall := fake.setPublicKeyIDSynthesizerArgsForCall[i]
	return argsForCall.arg1
}

func (fake *EndpointService) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.addPublicKeyExtractorMutex.RLock()
	defer fake.addPublicKeyExtractorMutex.RUnlock()
	fake.addResolverMutex.RLock()
	defer fake.addResolverMutex.RUnlock()
	fake.bindMutex.RLock()
	defer fake.bindMutex.RUnlock()
	fake.getIdentityMutex.RLock()
	defer fake.getIdentityMutex.RUnlock()
	fake.getResolverMutex.RLock()
	defer fake.getResolverMutex.RUnlock()
	fake.isBoundToMutex.RLock()
	defer fake.isBoundToMutex.RUnlock()
	fake.resolveMutex.RLock()
	defer fake.resolveMutex.RUnlock()
	fake.setPublicKeyIDSynthesizerMutex.RLock()
	defer fake.setPublicKeyIDSynthesizerMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *EndpointService) recordInvocation(key string, args []interface{}) {
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

var _ driver.EndpointService = new(EndpointService)
