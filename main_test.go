// Copyright 2018 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"strings"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/statsd_exporter/pkg/mapper"
	"github.com/stretchr/testify/assert"

	"github.com/prometheus/graphite_exporter/pkg/line"
)

type mockMapper struct {
	labels  prometheus.Labels
	present bool
	name    string
	action  mapper.ActionType
}

func (m *mockMapper) GetMapping(metricName string, metricType mapper.MetricType) (*mapper.MetricMapping, prometheus.Labels, bool) {
	mapping := mapper.MetricMapping{Name: m.name, Action: m.action}

	return &mapping, m.labels, m.present
}

func (m *mockMapper) InitFromFile(string, int, ...mapper.CacheOption) error {
	return nil
}
func (m *mockMapper) InitCache(int, ...mapper.CacheOption) {

}

func TestProcessLine(t *testing.T) {
	type testCase struct {
		line           string
		name           string
		mappingLabels  prometheus.Labels
		parsedLabels   prometheus.Labels
		value          float64
		mappingPresent bool
		willFail       bool
		action         mapper.ActionType
		strict         bool
	}

	testCases := []testCase{
		{
			line: "my.simple.metric 9001 1534620625",
			name: "my_simple_metric",
			mappingLabels: prometheus.Labels{
				"foo":  "bar",
				"zip":  "zot",
				"name": "alabel",
			},
			mappingPresent: true,
			value:          float64(9001),
		},
		{
			line: "my.simple.metric.baz 9002 1534620625",
			name: "my_simple_metric",
			mappingLabels: prometheus.Labels{
				"baz": "bat",
			},
			mappingPresent: true,
			value:          float64(9002),
		},
		{
			line:           "my.nomap.metric 9001 1534620625",
			name:           "my_nomap_metric",
			value:          float64(9001),
			parsedLabels:   prometheus.Labels{},
			mappingPresent: false,
		},
		{
			line:          "my.nomap.metric.novalue 9001 ",
			name:          "my_nomap_metric_novalue",
			mappingLabels: prometheus.Labels{},
			value:         float64(9001),
			willFail:      true,
		},
		{
			line:           "my.mapped.metric.drop 55 1534620625",
			name:           "my_mapped_metric_drop",
			mappingPresent: true,
			willFail:       true,
			action:         mapper.ActionTypeDrop,
		},
		{
			line:           "my.mapped.strict.metric 55 1534620625",
			name:           "my_mapped_strict_metric",
			value:          float64(55),
			mappingLabels:  prometheus.Labels{},
			mappingPresent: true,
			willFail:       false,
			strict:         true,
		},
		{
			line:           "my.mapped.strict.metric.drop 55 1534620625",
			name:           "my_mapped_strict_metric_drop",
			mappingPresent: false,
			willFail:       true,
			strict:         true,
		},
		{
			line: "my.simple.metric.with.tags;tag1=value1;tag2=value2 9002 1534620625",
			name: "my_simple_metric_with_tags",
			parsedLabels: prometheus.Labels{
				"tag1": "value1",
				"tag2": "value2",
			},
			mappingPresent: false,
			value:          float64(9002),
		},
		{
			// same tags, different values, should parse
			line: "my.simple.metric.with.tags;tag1=value3;tag2=value4 9002 1534620625",
			name: "my_simple_metric_with_tags",
			parsedLabels: prometheus.Labels{
				"tag1": "value3",
				"tag2": "value4",
			},
			mappingPresent: false,
			value:          float64(9002),
		},
		{
			// new tags other than previously used, should drop
			line:     "my.simple.metric.with.tags;tag1=value1;tag3=value2 9002 1534620625",
			name:     "my_simple_metric_with_tags",
			willFail: true,
		},
	}

	c := newGraphiteCollector(log.NewNopLogger())

	for _, testCase := range testCases {
		if testCase.mappingPresent {
			c.mapper = &mockMapper{
				name:    testCase.name,
				labels:  testCase.mappingLabels,
				action:  testCase.action,
				present: testCase.mappingPresent,
			}
		} else {
			c.mapper = &mockMapper{
				present: testCase.mappingPresent,
			}
		}

		c.strictMatch = testCase.strict
		line.ProcessLine(testCase.line, c.mapper, c.sampleCh, c.strictMatch, tagErrors, lastProcessed, invalidMetrics, c.logger)
	}

	c.sampleCh <- nil

	for _, k := range testCases {
		originalName := strings.Split(k.line, " ")[0]
		sample := c.samples[originalName]

		if k.willFail {
			assert.Nil(t, sample, "Found %s", k.name)
		} else {
			if assert.NotNil(t, sample, "Missing %s", k.name) {
				assert.Equal(t, k.name, sample.Name)
				if k.mappingPresent {
					assert.Equal(t, k.mappingLabels, sample.Labels)
				} else {
					assert.Equal(t, k.parsedLabels, sample.Labels)
				}
				assert.Equal(t, k.value, sample.Value)
			}
		}
	}
}
