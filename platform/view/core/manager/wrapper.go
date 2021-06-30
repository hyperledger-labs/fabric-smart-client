/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package manager

type wrappedContext struct {
	*ctx

	errorCallbackFuncs []func()
}

func (w *wrappedContext) OnError(f func()) {
	w.errorCallbackFuncs = append(w.errorCallbackFuncs, f)
}

func (w *wrappedContext) cleanup() {
	for _, callbackFunc := range w.errorCallbackFuncs {
		w.safeInvoke(callbackFunc)
	}
}

func (w *wrappedContext) safeInvoke(f func()) {
	defer func() {
		if r := recover(); r != nil {
			logger.Debugf("function [%s] panicked [%s]", f, r)
		}
	}()
	f()
}
