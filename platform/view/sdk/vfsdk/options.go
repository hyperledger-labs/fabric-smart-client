/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vfsdk

type FactoryOption func(*factoryEntry)

func WithFactoryId(fid string) FactoryOption {
	return func(f *factoryEntry) {
		f.fids = append(f.fids, fid)
	}
}
func WithInitiators(initiators ...any) FactoryOption {
	return func(f *factoryEntry) {
		f.initiators = append(f.initiators, initiators...)
	}
}
