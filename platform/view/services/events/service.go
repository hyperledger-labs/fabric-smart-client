/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package events

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
