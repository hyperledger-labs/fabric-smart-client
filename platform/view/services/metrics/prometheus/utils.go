/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package prometheus

import (
	"bufio"
	"io"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type MetricName = string

type MetricValue struct {
	Attributes map[string]string
	Value      float64
}

type MetricsResult map[MetricName][]MetricValue

type MetricsFilter func(MetricName, MetricValue) bool

var All = func(MetricName, MetricValue) bool { return true }

func ReadAll(reader io.Reader) (MetricsResult, error) {
	return ReadWithFilter(reader, All)
}

func ReadWithFilter(reader io.Reader, filter MetricsFilter) (MetricsResult, error) {
	scanner := bufio.NewScanner(reader)
	if !scanner.Scan() {
		return nil, io.EOF
	}
	r := MetricsResult{}
	for line := scanner.Text(); scanner.Scan(); line = scanner.Text() {
		if name, value, err := readLine(line); err != nil {
			return nil, err
		} else if len(name) > 0 && filter(name, value) {
			if r[name] == nil {
				r[name] = []MetricValue{}
			}
			r[name] = append(r[name], value)
		}
	}
	return r, nil
}

func readLine(line string) (MetricName, MetricValue, error) {
	if strings.HasPrefix(line, "#") {
		return "", MetricValue{}, nil
	}
	typeValue := strings.Split(line, " ")
	if len(typeValue) < 2 {
		return "", MetricValue{}, errors.Errorf("invalid metric type [%s]", line)
	}
	typ := strings.TrimSpace(strings.Join(typeValue[:len(typeValue)-1], " "))
	val, err := strconv.ParseFloat(strings.TrimSpace(typeValue[len(typeValue)-1]), 64)
	if err != nil {
		return "", MetricValue{}, errors.Wrapf(err, "invalid metric value: %s [%s]", val, line)
	}
	nameAttrs := strings.SplitN(strings.TrimRight(typ, "}"), "{", 2)
	if len(nameAttrs) == 0 {
		return "", MetricValue{}, errors.Errorf("invalid metric name: %s [%s]", typ, line)
	}
	name := strings.TrimSpace(nameAttrs[0])
	attrs := make(map[string]string)
	if len(nameAttrs) > 1 {
		for _, attr := range strings.Split(strings.TrimSpace(nameAttrs[1]), ",") {
			keyVal := strings.SplitN(attr, "=", 2)
			attrs[keyVal[0]] = keyVal[1]
		}
	}
	return name, MetricValue{Value: val, Attributes: attrs}, nil

}
