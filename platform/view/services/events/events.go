/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package events

//go:generate counterfeiter -o fakes/listener.go -fake-name Listener . Listener
//go:generate counterfeiter -o fakes/event.go -fake-name Event . Event

type Subscriber interface {
	Subscribe(topic string, receiver Listener)
	Unsubscribe(topic string, receiver Listener)
}

type Publisher interface {
	Publish(event Event)
}

type Listener interface {
	OnReceive(event Event)
}

type Event interface {
	Topic() string
	Message() interface{}
}

type EventSystem interface {
	Subscriber
	Publisher
}
