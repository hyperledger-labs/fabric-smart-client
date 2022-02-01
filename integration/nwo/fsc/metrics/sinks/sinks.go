/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sinks

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/armon/go-metrics"
)

type ConsoleSink struct {
}

func NewConsole() *ConsoleSink {
	return &ConsoleSink{}
}

func (s *ConsoleSink) SetGaugeWithLabels(key []string, val float32, labels []metrics.Label) {
	// TODO implement me
	panic("implement me")
}

func (s *ConsoleSink) IncrCounterWithLabels(key []string, val float32, labels []metrics.Label) {
	// TODO implement me
	panic("implement me")
}

func (s *ConsoleSink) AddSampleWithLabels(key []string, val float32, labels []metrics.Label) {
	// TODO implement me
	panic("implement me")
}

func (*ConsoleSink) SetGauge(key []string, val float32) {
	fmt.Println(">>>", key, val)
}

func (*ConsoleSink) EmitKey(key []string, val float32) {
	fmt.Println(">>>", key, val)
}

func (*ConsoleSink) IncrCounter(key []string, val float32) {
	fmt.Println(">>>", key, val)
}

func (*ConsoleSink) AddSample(key []string, val float32) {
	fmt.Println(">>>", key, val)
}

type GraphiteSink struct {
	sync.RWMutex
	net.Conn
	c chan int
}

func (s *GraphiteSink) SetGaugeWithLabels(key []string, val float32, labels []metrics.Label) {
	// TODO implement me
	panic("implement me")
}

func (s *GraphiteSink) IncrCounterWithLabels(key []string, val float32, labels []metrics.Label) {
	// TODO implement me
	panic("implement me")
}

func (s *GraphiteSink) AddSampleWithLabels(key []string, val float32, labels []metrics.Label) {
	// TODO implement me
	panic("implement me")
}

func (s *GraphiteSink) checkConnection() {
	s.RLock()
	if s.Conn != nil {
		s.RUnlock()
		return
	}
	s.RUnlock()

	s.Lock()
	defer s.Unlock()

	if s.Conn != nil {
		return
	}

	conn, err := net.Dial("tcp", "9.42.1.166:2003")
	if err != nil {
		panic(err)
	}
	s.Conn = conn
	s.c = make(chan int, 1000)
	go func() {
		for latency := range s.c {
			s.send(latency)
		}
	}()
}

func (s *GraphiteSink) send(latency int) {
	t := time.Now().UnixNano() / time.Second.Nanoseconds()
	text2Send := fmt.Sprintf("servers.orderer.metrics %d %d\n", latency, t)
	// fmt.Println(text2Send)
	_, err := s.Conn.Write([]byte(text2Send))
	if err != nil {
		fmt.Println("Write failed:", err)
	}
}

func (s *GraphiteSink) SetGauge(key []string, val float32) {
	s.checkConnection()
	s.c <- int(val)
}

func (s *GraphiteSink) EmitKey(key []string, val float32) {
	s.checkConnection()
	s.c <- int(val)
}

func (s *GraphiteSink) IncrCounter(key []string, val float32) {
	s.checkConnection()
	s.c <- int(val)
}

func (s *GraphiteSink) AddSample(key []string, val float32) {
	s.checkConnection()
	s.c <- int(val)
}
