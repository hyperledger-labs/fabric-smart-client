/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dmetrics

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/armon/go-metrics"
	"github.com/stretchr/testify/assert"
)

var start int64
var maxBlock = int64(1)

var receivedCounter = map[int64]int32{}
var l = sync.Mutex{}
var o = sync.Once{}

func MeasureBlock(shim Shim, events ...Event) (remaining []Event) {
	var sentEvent *Event
	// Find sent event
	for _, e := range events {
		if e.Keys[0] == "sent" {
			sentEvent = &e
			break
		}
	}

	if sentEvent == nil {
		fmt.Println("Could not find sent event for", events)
		return events
	}

	// Emit events
	for _, e := range events {
		blockSeq, _ := strconv.ParseInt(e.Keys[2], 10, 32)
		if start == 0 && blockSeq > 1 {
			fmt.Println("starting measuring", events)
			start = time.Now().UnixNano()
			o.Do(func() {
				time.AfterFunc(time.Second*5, measure)
			})
		}
		if e.Keys[0] != "sent" {
			receivedCounter[blockSeq]++
			latency := e.Raw.Timestamp.Sub(sentEvent.Raw.Timestamp).Nanoseconds() / (1000 * 1000)
			if blockSeq > maxBlock {
				maxBlock = blockSeq
				totalTime := time.Since(time.Unix(0, start)).Nanoseconds() / (1000 * 1000)
				if totalTime == 0 {
					totalTime = 1
				}
				fmt.Println("block", maxBlock, "totalTime:", totalTime, "ms", "avg throughput:", totalTime/maxBlock, "ms per block")
			}
			shim.AddSample([]string{sentEvent.Host, e.Host}, float32(latency))
		}
	}
	return []Event{*sentEvent}
}

func measure() {
	l.Lock()
	defer l.Unlock()
	failure := false
	for blockSeq, count := range receivedCounter {
		if count < 7 {
			fmt.Println(blockSeq, "was received only", count, "times")
			failure = true
		}
	}
	exitStatus := 0
	if failure {
		exitStatus = 1
	}
	os.Exit(exitStatus)
}

func TestNewAggregatorFakeInput(t *testing.T) {
	t.Skip()
	var l Listener
	var err error
	go func() {
		l, err = NewUDPListener(8125)
		if err != nil {
			panic(err)
		}
	}()
	time.Sleep(time.Second)

	events := createEvents()
	eventAmount := len(events)
	aggr, _ := NewAggregator(&sinkMock{}, nil, &UDPRawEvenParser{}, l)
	de := aggr.NewDistributedEvent(func(e *Event) []string {
		if e.Keys[1] == "block" {
			return []string{e.Keys[1] + "-" + e.Keys[2]}
		}
		return []string{}
	}, 2, time.Second)
	de.AddSample(MeasureBlock)

	time.Sleep(5 * time.Second)
	assert.Equal(t, aggr.sink.(*sinkMock).n, eventAmount/4*3)
}

type listenerMock struct {
	events <-chan *RawEvent
}

func (l *listenerMock) Subscribe() <-chan *RawEvent {
	return l.events
}

func createEvents() <-chan *RawEvent {
	c := make(chan *RawEvent, 44)
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138646794613984), Payload: "9!37!201!211_7051.sent.block.1:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138646808723178), Payload: "9!37!249!45_7051.received.block.1:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138646808947918), Payload: "9!37!134!235_7051.received.block.1:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138646810062691), Payload: "9!37!220!210_7051.received.block.1:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138656776572215), Payload: "9!37!201!211_7051.sent.block.2:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138656795716067), Payload: "9!37!249!45_7051.received.block.2:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138656795856369), Payload: "9!37!134!235_7051.received.block.2:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138656796015458), Payload: "9!37!220!210_7051.received.block.2:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138657747507614), Payload: "9!37!201!211_7051.sent.block.3:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138657768705341), Payload: "9!37!249!45_7051.received.block.3:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138657769054138), Payload: "9!37!220!210_7051.received.block.3:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138657769639020), Payload: "9!37!134!235_7051.received.block.3:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138658684532596), Payload: "9!37!201!211_7051.sent.block.4:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138658702700271), Payload: "9!37!249!45_7051.received.block.4:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138658702862369), Payload: "9!37!134!235_7051.received.block.4:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138658703052997), Payload: "9!37!220!210_7051.received.block.4:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138659634511738), Payload: "9!37!201!211_7051.sent.block.5:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138659655718577), Payload: "9!37!249!45_7051.received.block.5:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138659656021234), Payload: "9!37!220!210_7051.received.block.5:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138659656835183), Payload: "9!37!134!235_7051.received.block.5:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138660575504679), Payload: "9!37!201!211_7051.sent.block.6:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138660595870690), Payload: "9!37!134!235_7051.received.block.6:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138660596700953), Payload: "9!37!249!45_7051.received.block.6:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138660599082775), Payload: "9!37!220!210_7051.received.block.6:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138661515100744), Payload: "9!37!201!211_7051.sent.block.7:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138661529760439), Payload: "9!37!249!45_7051.received.block.7:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138661530005212), Payload: "9!37!220!210_7051.received.block.7:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138661530825991), Payload: "9!37!134!235_7051.received.block.7:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138662451948832), Payload: "9!37!201!211_7051.sent.block.8:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138662470877428), Payload: "9!37!134!235_7051.received.block.8:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138662471044963), Payload: "9!37!220!210_7051.received.block.8:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138662471681773), Payload: "9!37!249!45_7051.received.block.8:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138663395495271), Payload: "9!37!201!211_7051.sent.block.9:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138663412703962), Payload: "9!37!249!45_7051.received.block.9:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138663412873804), Payload: "9!37!134!235_7051.received.block.9:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138663413039663), Payload: "9!37!220!210_7051.received.block.9:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138664346536111), Payload: "9!37!201!211_7051.sent.block.10:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138664367849756), Payload: "9!37!134!235_7051.received.block.10:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138664369660357), Payload: "9!37!249!45_7051.received.block.10:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138664371002487), Payload: "9!37!220!210_7051.received.block.10:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138665317519592), Payload: "9!37!201!211_7051.sent.block.11:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138665338700915), Payload: "9!37!249!45_7051.received.block.11:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138665338792492), Payload: "9!37!134!235_7051.received.block.11:1.000000|kv"}
	c <- &RawEvent{Timestamp: time.Unix(0, 1496138665339095726), Payload: "9!37!220!210_7051.received.block.11:1.000000|kv"}
	return c
}

type sinkMock struct {
	n int
}

func (s *sinkMock) SetGaugeWithLabels(key []string, val float32, labels []metrics.Label) {
	// TODO implement me
	panic("implement me")
}

func (s *sinkMock) IncrCounterWithLabels(key []string, val float32, labels []metrics.Label) {
	// TODO implement me
	panic("implement me")
}

func (s *sinkMock) AddSampleWithLabels(key []string, val float32, labels []metrics.Label) {
	// TODO implement me
	panic("implement me")
}

func (s *sinkMock) SetGauge(key []string, val float32) {
	s.n++
	fmt.Println("SetGauge", key, val)
}

func (s *sinkMock) EmitKey(key []string, val float32) {
	s.n++
	fmt.Println("EmitKey", key, val)
}

func (s *sinkMock) IncrCounter(key []string, val float32) {
	s.n++
	fmt.Println("IncrCounter", key, val)
}

func (s *sinkMock) AddSample(key []string, val float32) {
	s.n++
	fmt.Println("AddSample", key, val)
}
