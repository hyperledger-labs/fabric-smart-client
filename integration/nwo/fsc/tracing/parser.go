/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dmetrics

import (
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var (
	buffSize = 2
	logger   = flogging.MustGetLogger("fsc.tracing")
)

type RawEventParser interface {
	Parse(re *RawEvent) *Event
}

type Parser struct {
	ch             <-chan *RawEvent
	logger         *flogging.FabricLogger
	rawEventParser RawEventParser
}

func NewParser(rawEventParser RawEventParser, ch <-chan *RawEvent) *Parser {
	return &Parser{rawEventParser: rawEventParser, ch: ch, logger: flogging.MustGetLogger(logModuleName)}
}

func (p *Parser) Read() <-chan *Event {
	out := make(chan *Event, buffSize)
	go func() {
		for re := range p.ch {
			e := p.parse(re)
			if e == nil {
				continue
			}
			logger.Debugf("new parsed event [%s][%v]\n", e.Host, e.Keys)
			out <- e
		}
	}()
	return out
}

func (p *Parser) parse(re *RawEvent) *Event {
	return p.rawEventParser.Parse(re)
}

type UDPRawEvenParser struct {
}

func (U *UDPRawEvenParser) Parse(re *RawEvent) *Event {
	re.Payload = strings.TrimSpace(re.Payload)
	li := strings.LastIndex(re.Payload, "|")
	if li == -1 {
		logger.Warning("Badly formatted event, missing |", re.Payload)
		return nil
	}
	et := re.Payload[li+1:]
	ep := re.Payload[:li]
	i := strings.LastIndex(ep, ":")
	if i == -1 {
		logger.Warning("Badly formatted event, missing :", re.Payload)
		return nil
	}
	valStr := ep[i+1:]
	val, err := strconv.ParseFloat(valStr, 32)
	if err != nil {
		logger.Warning("Badly formatted event, value", valStr, "isn't a float", re.Payload)
		return nil
	}
	keys := strings.Split(ep[:i], ".")
	if len(keys) < 2 {
		logger.Warning("Badly formatted event, not enough keys(", keys, ")", re.Payload)
		return nil
	}
	return &Event{
		Raw:   re,
		Type:  et,
		Value: float32(val),
		Keys:  keys[1:],
		Host:  strings.Replace(keys[0], "!", ".", -1),
	}
}

type FileRawEvenParser struct {
	MinTS, MaxTs int64
}

func (U *FileRawEvenParser) Parse(re *RawEvent) *Event {
	re.Payload = strings.TrimSpace(re.Payload)
	keys := strings.Split(re.Payload, ", ")
	if len(keys) < 6 {
		logger.Warning("Badly formatted event, not enough keys(", keys, ")", re.Payload)
		return nil
	}

	subkeys := strings.Split(keys[2], " ")
	subkeys[0] = subkeys[0][1:]
	subkeys[len(subkeys)-1] = subkeys[len(subkeys)-1][:len(subkeys[len(subkeys)-1])-1]

	// timestamp is the last key of subkeys
	ts, err := strconv.ParseInt(subkeys[len(subkeys)-1], 10, 64)
	if err != nil {
		logger.Warning("Badly formatted event, timestamp", keys[4], "isn't an int", re.Payload)
		return nil
	}
	re.Timestamp = time.Unix(0, ts)
	// fmt.Println(ts, re.Timestamp.Nanosecond(), re.Timestamp.String())
	if U.MinTS == 0 || ts < U.MinTS {
		U.MinTS = ts
	}
	if ts > U.MaxTs {
		U.MaxTs = ts
	}

	return &Event{
		Raw:       re,
		Type:      "",
		Value:     0,
		Keys:      subkeys,
		Host:      keys[3],
		Timestamp: time.Unix(0, ts),
	}
}
