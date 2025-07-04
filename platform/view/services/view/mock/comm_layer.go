// Code generated by counterfeiter. DO NOT EDIT.
package mock

import (
	"context"
	"sync"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type CommLayer struct {
	NewSessionWithIDStub        func(sessionID, contextID, endpoint string, pkid []byte, caller view.Identity, msg *view.Message) (view.Session, error)
	newSessionWithIDMutex       sync.RWMutex
	newSessionWithIDArgsForCall []struct {
		sessionID string
		contextID string
		endpoint  string
		pkid      []byte
		caller    view.Identity
		msg       *view.Message
	}
	newSessionWithIDReturns struct {
		result1 view.Session
		result2 error
	}
	newSessionWithIDReturnsOnCall map[int]struct {
		result1 view.Session
		result2 error
	}
	NewSessionStub        func(caller string, contextID string, endpoint string, pkid []byte) (view.Session, error)
	newSessionMutex       sync.RWMutex
	newSessionArgsForCall []struct {
		caller    string
		contextID string
		endpoint  string
		pkid      []byte
	}
	newSessionReturns struct {
		result1 view.Session
		result2 error
	}
	newSessionReturnsOnCall map[int]struct {
		result1 view.Session
		result2 error
	}
	MasterSessionStub        func() (view.Session, error)
	masterSessionMutex       sync.RWMutex
	masterSessionArgsForCall []struct{}
	masterSessionReturns     struct {
		result1 view.Session
		result2 error
	}
	masterSessionReturnsOnCall map[int]struct {
		result1 view.Session
		result2 error
	}
	DeleteSessionsStub        func(sessionID string)
	deleteSessionsMutex       sync.RWMutex
	deleteSessionsArgsForCall []struct {
		sessionID string
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *CommLayer) NewSessionWithID(sessionID string, contextID string, endpoint string, pkid []byte, caller view.Identity, msg *view.Message) (view.Session, error) {
	var pkidCopy []byte
	if pkid != nil {
		pkidCopy = make([]byte, len(pkid))
		copy(pkidCopy, pkid)
	}
	fake.newSessionWithIDMutex.Lock()
	ret, specificReturn := fake.newSessionWithIDReturnsOnCall[len(fake.newSessionWithIDArgsForCall)]
	fake.newSessionWithIDArgsForCall = append(fake.newSessionWithIDArgsForCall, struct {
		sessionID string
		contextID string
		endpoint  string
		pkid      []byte
		caller    view.Identity
		msg       *view.Message
	}{sessionID, contextID, endpoint, pkidCopy, caller, msg})
	fake.recordInvocation("NewSessionWithID", []interface{}{sessionID, contextID, endpoint, pkidCopy, caller, msg})
	fake.newSessionWithIDMutex.Unlock()
	if fake.NewSessionWithIDStub != nil {
		return fake.NewSessionWithIDStub(sessionID, contextID, endpoint, pkid, caller, msg)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fake.newSessionWithIDReturns.result1, fake.newSessionWithIDReturns.result2
}

func (fake *CommLayer) NewSessionWithIDCallCount() int {
	fake.newSessionWithIDMutex.RLock()
	defer fake.newSessionWithIDMutex.RUnlock()
	return len(fake.newSessionWithIDArgsForCall)
}

func (fake *CommLayer) NewSessionWithIDArgsForCall(i int) (string, string, string, []byte, view.Identity, *view.Message) {
	fake.newSessionWithIDMutex.RLock()
	defer fake.newSessionWithIDMutex.RUnlock()
	return fake.newSessionWithIDArgsForCall[i].sessionID, fake.newSessionWithIDArgsForCall[i].contextID, fake.newSessionWithIDArgsForCall[i].endpoint, fake.newSessionWithIDArgsForCall[i].pkid, fake.newSessionWithIDArgsForCall[i].caller, fake.newSessionWithIDArgsForCall[i].msg
}

func (fake *CommLayer) NewSessionWithIDReturns(result1 view.Session, result2 error) {
	fake.NewSessionWithIDStub = nil
	fake.newSessionWithIDReturns = struct {
		result1 view.Session
		result2 error
	}{result1, result2}
}

func (fake *CommLayer) NewSessionWithIDReturnsOnCall(i int, result1 view.Session, result2 error) {
	fake.NewSessionWithIDStub = nil
	if fake.newSessionWithIDReturnsOnCall == nil {
		fake.newSessionWithIDReturnsOnCall = make(map[int]struct {
			result1 view.Session
			result2 error
		})
	}
	fake.newSessionWithIDReturnsOnCall[i] = struct {
		result1 view.Session
		result2 error
	}{result1, result2}
}

func (fake *CommLayer) NewSession(caller string, contextID string, endpoint string, pkid []byte) (view.Session, error) {
	var pkidCopy []byte
	if pkid != nil {
		pkidCopy = make([]byte, len(pkid))
		copy(pkidCopy, pkid)
	}
	fake.newSessionMutex.Lock()
	ret, specificReturn := fake.newSessionReturnsOnCall[len(fake.newSessionArgsForCall)]
	fake.newSessionArgsForCall = append(fake.newSessionArgsForCall, struct {
		caller    string
		contextID string
		endpoint  string
		pkid      []byte
	}{caller, contextID, endpoint, pkidCopy})
	fake.recordInvocation("NewSession", []interface{}{caller, contextID, endpoint, pkidCopy})
	fake.newSessionMutex.Unlock()
	if fake.NewSessionStub != nil {
		return fake.NewSessionStub(caller, contextID, endpoint, pkid)
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fake.newSessionReturns.result1, fake.newSessionReturns.result2
}

func (fake *CommLayer) NewSessionCallCount() int {
	fake.newSessionMutex.RLock()
	defer fake.newSessionMutex.RUnlock()
	return len(fake.newSessionArgsForCall)
}

func (fake *CommLayer) NewSessionArgsForCall(i int) (string, string, string, []byte) {
	fake.newSessionMutex.RLock()
	defer fake.newSessionMutex.RUnlock()
	return fake.newSessionArgsForCall[i].caller, fake.newSessionArgsForCall[i].contextID, fake.newSessionArgsForCall[i].endpoint, fake.newSessionArgsForCall[i].pkid
}

func (fake *CommLayer) NewSessionReturns(result1 view.Session, result2 error) {
	fake.NewSessionStub = nil
	fake.newSessionReturns = struct {
		result1 view.Session
		result2 error
	}{result1, result2}
}

func (fake *CommLayer) NewSessionReturnsOnCall(i int, result1 view.Session, result2 error) {
	fake.NewSessionStub = nil
	if fake.newSessionReturnsOnCall == nil {
		fake.newSessionReturnsOnCall = make(map[int]struct {
			result1 view.Session
			result2 error
		})
	}
	fake.newSessionReturnsOnCall[i] = struct {
		result1 view.Session
		result2 error
	}{result1, result2}
}

func (fake *CommLayer) MasterSession() (view.Session, error) {
	fake.masterSessionMutex.Lock()
	ret, specificReturn := fake.masterSessionReturnsOnCall[len(fake.masterSessionArgsForCall)]
	fake.masterSessionArgsForCall = append(fake.masterSessionArgsForCall, struct{}{})
	fake.recordInvocation("MasterSession", []interface{}{})
	fake.masterSessionMutex.Unlock()
	if fake.MasterSessionStub != nil {
		return fake.MasterSessionStub()
	}
	if specificReturn {
		return ret.result1, ret.result2
	}
	return fake.masterSessionReturns.result1, fake.masterSessionReturns.result2
}

func (fake *CommLayer) MasterSessionCallCount() int {
	fake.masterSessionMutex.RLock()
	defer fake.masterSessionMutex.RUnlock()
	return len(fake.masterSessionArgsForCall)
}

func (fake *CommLayer) MasterSessionReturns(result1 view.Session, result2 error) {
	fake.MasterSessionStub = nil
	fake.masterSessionReturns = struct {
		result1 view.Session
		result2 error
	}{result1, result2}
}

func (fake *CommLayer) MasterSessionReturnsOnCall(i int, result1 view.Session, result2 error) {
	fake.MasterSessionStub = nil
	if fake.masterSessionReturnsOnCall == nil {
		fake.masterSessionReturnsOnCall = make(map[int]struct {
			result1 view.Session
			result2 error
		})
	}
	fake.masterSessionReturnsOnCall[i] = struct {
		result1 view.Session
		result2 error
	}{result1, result2}
}

func (fake *CommLayer) DeleteSessions(ctx context.Context, sessionID string) {
	fake.deleteSessionsMutex.Lock()
	fake.deleteSessionsArgsForCall = append(fake.deleteSessionsArgsForCall, struct {
		sessionID string
	}{sessionID})
	fake.recordInvocation("DeleteSessions", []interface{}{sessionID})
	fake.deleteSessionsMutex.Unlock()
	if fake.DeleteSessionsStub != nil {
		fake.DeleteSessionsStub(sessionID)
	}
}

func (fake *CommLayer) DeleteSessionsCallCount() int {
	fake.deleteSessionsMutex.RLock()
	defer fake.deleteSessionsMutex.RUnlock()
	return len(fake.deleteSessionsArgsForCall)
}

func (fake *CommLayer) DeleteSessionsArgsForCall(i int) string {
	fake.deleteSessionsMutex.RLock()
	defer fake.deleteSessionsMutex.RUnlock()
	return fake.deleteSessionsArgsForCall[i].sessionID
}

func (fake *CommLayer) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.newSessionWithIDMutex.RLock()
	defer fake.newSessionWithIDMutex.RUnlock()
	fake.newSessionMutex.RLock()
	defer fake.newSessionMutex.RUnlock()
	fake.masterSessionMutex.RLock()
	defer fake.masterSessionMutex.RUnlock()
	fake.deleteSessionsMutex.RLock()
	defer fake.deleteSessionsMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *CommLayer) recordInvocation(key string, args []interface{}) {
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

var _ view2.CommLayer = new(CommLayer)
