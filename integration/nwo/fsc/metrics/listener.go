/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dmetrics

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/op/go-logging"
)

const (
	// statsdMaxLen is the maximum size of a packet
	// to send to statsd
	statsdMaxLen         = 1400
	subscriptionBuffSize = 200
	logModuleName        = "fsc.metrics"
)

func init() {
	logging.SetLevel(logging.INFO, logModuleName)
}

type udpListener struct {
	udp    *net.UDPConn
	logger *logging.Logger
	sync.RWMutex
	subscriptions []chan *RawEvent
}

func NewUDPListener(port int) (*udpListener, error) {
	a := &udpListener{
		subscriptions: []chan *RawEvent{},
		logger:        logging.MustGetLogger(logModuleName),
	}
	if err := a.listen(port); err != nil {
		return nil, err
	}
	go a.handleMessages()
	return a, nil
}

func (a *udpListener) listen(port int) error {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	a.udp, err = net.ListenUDP("udp", addr)
	return err
}

func (a *udpListener) handleMessages() {
	buf := make([]byte, statsdMaxLen)
	for {
		a.logger.Debugf("Waiting for message...")
		n, addr, err := a.udp.ReadFromUDP(buf)
		if err != nil {
			a.logger.Warning("Failed reading from socket:", err)
			continue
		}
		data := buf[0:n]
		a.logger.Debugf("Received %d bytes from %s, [%s]", n, addr.IP.String(), string(data))
		// if len(addr.IP.String()) < 7 {
		// 	a.logger.Warningf("Invalid IP address [%s], skipping", addr.IP.String())
		// 	continue
		// }
		for _, s := range strings.Split(string(data), "\n") {
			if len(s) == 0 {
				continue
			}
			event := &RawEvent{
				Payload:   s,
				Timestamp: time.Now(),
			}
			a.demux(event)
		}

	}
}

func (a *udpListener) Subscribe() <-chan *RawEvent {
	ch := make(chan *RawEvent, subscriptionBuffSize)
	a.Lock()
	defer a.Unlock()
	a.subscriptions = append(a.subscriptions, ch)
	return ch
}

func (a *udpListener) demux(e *RawEvent) {
	a.RLock()
	defer a.RUnlock()
	a.logger.Debugf("demuxing new event to [%d] subscribers", len(a.subscriptions))
	for _, ch := range a.subscriptions {
		ch <- e
	}
}

type fileListener struct {
	path   string
	logger *logging.Logger
	sync.RWMutex
	subscriptions []chan *RawEvent
	numEvents     int
}

func NewFileListener(path string) *fileListener {
	return &fileListener{
		path:   path,
		logger: logging.MustGetLogger(logModuleName),
	}
}

func (a *fileListener) Subscribe() <-chan *RawEvent {
	ch := make(chan *RawEvent, subscriptionBuffSize)
	a.Lock()
	defer a.Unlock()
	a.subscriptions = append(a.subscriptions, ch)
	return ch
}

func (a *fileListener) Start() {
	// open file
	f, err := os.Open(a.path)
	if err != nil {
		fmt.Println("Failed to open file:", err)
		return
	}
	defer f.Close()
	// read each line and send to subscribers
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		event := &RawEvent{
			Payload: scanner.Text(),
		}
		a.demux(event)
	}
}

func (a *fileListener) demux(e *RawEvent) {
	a.RLock()
	defer a.RUnlock()
	a.logger.Debugf("demuxing new event: [%d]", a.numEvents)
	a.numEvents++
	for _, ch := range a.subscriptions {
		ch <- e
	}

}

func (a *fileListener) NumEvents() int {
	return a.numEvents
}
