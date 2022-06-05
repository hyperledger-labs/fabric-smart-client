/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package events

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/pkg/errors"
)

type EventService interface {
	GetPublisher() Publisher
	GetSubscriber() Subscriber
}

type Service struct {
	EventSystem EventSystem
}

func (s *Service) GetSubscriber() Subscriber {
	return s.EventSystem
}

func (s *Service) GetPublisher() Publisher {
	return s.EventSystem
}

var eventServiceLookUp = &Service{}

func getService(sp view.ServiceProvider) (EventService, error) {
	s, err := sp.GetService(eventServiceLookUp)
	if err != nil {
		return nil, errors.Wrap(err, "cannot get event Service from registry")
	}
	return s.(EventService), nil
}

func GetSubscriber(sp view.ServiceProvider) (Subscriber, error) {
	s, err := getService(sp)
	if err != nil {
		return nil, errors.Wrap(err, "cannot get subscriber")
	}
	return s.GetSubscriber(), nil
}

func GetPublisher(sp view.ServiceProvider) (Publisher, error) {
	s, err := getService(sp)
	if err != nil {
		return nil, errors.Wrap(err, "cannot get publisher")
	}
	return s.GetPublisher(), nil
}
