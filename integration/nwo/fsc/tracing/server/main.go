/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	dmetrics "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/tracing/sinks"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/tracing/storage"
)

func main() {
	// start listener
	l, err := dmetrics.NewUDPListener(8125)
	if err != nil {
		panic(err)
	}
	// start aggregator, only store events on disk
	storage, err := storage.NewFileStorage("events.dat")
	if err != nil {
		panic(err)
	}
	if _, err = dmetrics.NewAggregator(
		sinks.NewConsole(),
		storage,
		&dmetrics.UDPRawEvenParser{},
		l,
	); err != nil {
		panic(err)
	}
	fmt.Println("Started aggregator")

	serve := make(chan error, 10)
	go handleSignals(map[os.Signal]func(){
		syscall.SIGINT:  func() { storage.Close(); serve <- nil },
		syscall.SIGTERM: func() { storage.Close(); serve <- nil },
		syscall.SIGSTOP: func() { storage.Close(); serve <- nil },
	})
	<-serve
}

func handleSignals(handlers map[os.Signal]func()) {
	var signals []os.Signal
	for sig := range handlers {
		signals = append(signals, sig)
	}
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, signals...)
	for sig := range signalChan {
		handlers[sig]()
	}
}
