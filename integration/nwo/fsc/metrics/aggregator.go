/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dmetrics

import (
	"sync"
	"time"

	"github.com/armon/go-metrics"
	"github.com/op/go-logging"
)

type eventType string

type ClassExtractorFunc func(*Event) []string

const (
	key     = eventType("kv")
	gauge   = eventType("g")
	counter = eventType("c")
	sample  = eventType("ms")
)

type Storage interface {
	Store(event *Event)
}

type Listener interface {
	Subscribe() <-chan *RawEvent
}

type distEvent struct {
	sync.RWMutex
	classExtractor ClassExtractorFunc
	minSize        int
	hooks          []*eventHook
	ttl            time.Duration
}

type eventHook struct {
	eType eventType
	f     func(Shim, ...Event) (remaining []Event)
}

func (de *distEvent) AddGauge(f func(Shim, ...Event) (remaining []Event)) {
	de.Lock()
	defer de.Unlock()
	de.hooks = append(de.hooks, &eventHook{eType: gauge, f: f})
}

func (de *distEvent) AddKeyEmission(f func(Shim, ...Event) (remaining []Event)) {
	de.Lock()
	defer de.Unlock()
	de.hooks = append(de.hooks, &eventHook{eType: key, f: f})
}

func (de *distEvent) AddCounterInc(f func(Shim, ...Event) (remaining []Event)) {
	de.Lock()
	defer de.Unlock()
	de.hooks = append(de.hooks, &eventHook{eType: counter, f: f})
}

func (de *distEvent) AddSample(f func(Shim, ...Event) (remaining []Event)) {
	de.Lock()
	defer de.Unlock()
	de.hooks = append(de.hooks, &eventHook{eType: sample, f: f})
}

type Aggregator struct {
	sync.RWMutex
	p             *Parser
	dEvents       []*distEvent
	eventHandlers map[string]*dEventHandler
	sink          metrics.MetricSink
	storage       Storage
	logger        *logging.Logger
	numEvents     uint64
}

func NewAggregator(sink metrics.MetricSink, storage Storage, rawEventParser RawEventParser, l Listener) (*Aggregator, error) {
	p := NewParser(rawEventParser, l.Subscribe())

	a := &Aggregator{
		eventHandlers: make(map[string]*dEventHandler),
		p:             p,
		dEvents:       []*distEvent{},
		sink:          sink,
		storage:       storage,
		logger:        logging.MustGetLogger(logModuleName),
	}
	go a.run()
	return a, nil
}

func (a *Aggregator) NewDistributedEvent(ce ClassExtractorFunc, minSize int, ttl time.Duration) DistributedEvent {
	de := &distEvent{classExtractor: ce, minSize: minSize, ttl: ttl}
	a.Lock()
	defer a.Unlock()
	a.dEvents = append(a.dEvents, de)
	return de
}

func (a *Aggregator) run() {
	for e := range a.p.Read() {
		a.logger.Debugf("Received event: %s", e.String())
		a.processEvent(e)
		a.Lock()
		a.numEvents++
		a.Unlock()
		if a.storage != nil {
			a.storage.Store(e)
		}
	}
}

func (a *Aggregator) processEvent(e *Event) {
	a.RLock()
	defer a.RUnlock()
	for _, de := range a.dEvents {
		for _, cls := range de.classExtractor(e) {
			if cls == "" {
				continue
			}
			handler := a.getHandler(cls, de)
			handler.handleEvent(e)
		}
	}
}

func (a *Aggregator) getHandler(eventClass string, de *distEvent) *dEventHandler {
	handler, exists := a.eventHandlers[eventClass]
	if !exists {
		handler = &dEventHandler{de: de, sink: a.sink}
		a.eventHandlers[eventClass] = handler
		time.AfterFunc(de.ttl, func() {
			a.Lock()
			defer a.Unlock()
			delete(a.eventHandlers, eventClass)
		})
	}
	return handler
}

func (a *Aggregator) NumEvents() uint64 {
	a.RLock()
	defer a.RUnlock()
	return a.numEvents
}

type dEventHandler struct {
	sync.RWMutex
	events []Event
	de     *distEvent
	sink   metrics.MetricSink
}

func (h *dEventHandler) handleEvent(e *Event) {
	h.events = append(h.events, *e)
	if h.de.minSize > len(h.events) {
		return
	}
	h.de.RLock()
	hooks := h.de.hooks
	h.de.RUnlock()
	for _, eventHook := range hooks {
		remaining := eventHook.f(h.sink, h.events...)
		h.events = remaining
	}
}

func (h *dEventHandler) sendEvent(input SinkInput, eType eventType) {
	switch eType {
	case gauge:
		h.sink.SetGauge(input.Key, input.Val)
	case sample:
		h.sink.AddSample(input.Key, input.Val)
	case counter:
		h.sink.IncrCounter(input.Key, input.Val)
	case key:
		h.sink.EmitKey(input.Key, input.Val)
	}
}
