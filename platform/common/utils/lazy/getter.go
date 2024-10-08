/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lazy

import "sync"

type Getter[V any] interface {
	Get() (V, error)
}

type lazyGetter[V any] struct {
	v        V
	err      error
	provider func() (V, error)
	once     sync.Once
}

func NewGetter[V any](provider func() (V, error)) *lazyGetter[V] {
	return &lazyGetter[V]{provider: provider}
}

func (g *lazyGetter[V]) Get() (V, error) {
	g.once.Do(func() {
		g.v, g.err = g.provider()
	})
	return g.v, g.err
}
