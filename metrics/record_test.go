/*
Copyright 2019 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package metrics

import (
	"context"
	"fmt"
	"testing"

	"knative.dev/pkg/metrics/metricskey"
	"knative.dev/pkg/metrics/metricstest"

	"github.com/google/go-cmp/cmp"
	"go.opencensus.io/resource"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

type cases struct {
	name          string
	metricsConfig *metricsConfig
	measurement   stats.Measurement
	resource      resource.Resource
}

func TestRecordBatch(t *testing.T) {
	ctx := context.Background()
	measure1 := stats.Int64("count1", "First counter", stats.UnitDimensionless)
	measure2 := stats.Int64("count2", "Second counter", stats.UnitDimensionless)
	v := []*view.View{{
		Measure:     measure1,
		Aggregation: view.LastValue(),
	}, {
		Measure:     measure2,
		Aggregation: view.LastValue(),
	}}
	view.Register(v...)
	t.Cleanup(func() { view.Unregister(v...) })
	metricsConfig := &metricsConfig{}
	measurement1 := measure1.M(1984)
	measurement2 := measure2.M(42)
	setCurMetricsConfig(metricsConfig)
	RecordBatch(ctx, measurement1, measurement2)
	metricstest.CheckLastValueData(t, measurement1.Measure().Name(), map[string]string{}, 1984)
	metricstest.CheckLastValueData(t, measurement2.Measure().Name(), map[string]string{}, 42)
}

func TestBucketsNBy10(t *testing.T) {
	tests := []struct {
		base float64
		n    int
		want []float64
	}{{
		base: 0.001,
		n:    5,
		want: []float64{0.001, 0.01, 0.1, 1, 10},
	}, {
		base: 1,
		n:    2,
		want: []float64{1, 10},
	}, {
		base: 0.5,
		n:    4,
		want: []float64{0.5, 5, 50, 500},
	}}

	for _, test := range tests {
		t.Run(fmt.Sprintf("base=%f,n=%d", test.base, test.n), func(t *testing.T) {
			got := BucketsNBy10(test.base, test.n)
			if diff := cmp.Diff(got, test.want); diff != "" {
				t.Error("BucketsNBy10 (-want, +got) =", diff)
			}
		})
	}
}

func TestMeter(t *testing.T) {
	measure := stats.Int64("request_count", "Number of reconcile operations", stats.UnitDimensionless)
	// Increase the measurement value for each test case so that checking
	// the last value ensures the measurement has been recorded.
	meterTestCases := []cases{{
		name:          "resource 1",
		metricsConfig: &metricsConfig{},
		measurement:   measure.M(1),
		resource:      resource.Resource{Type: "resource 1", Labels: map[string]string{"bar": "foo"}},
	}, {
		name:          "resource 2",
		metricsConfig: &metricsConfig{},
		measurement:   measure.M(2),
		resource:      resource.Resource{Type: "resource 2", Labels: map[string]string{"bar": "foo", "bar1": "foo1"}},
	}}
	v := &view.View{
		Measure:     measure,
		Aggregation: view.LastValue(),
	}
	RegisterResourceView(v)
	t.Cleanup(func() { UnregisterResourceView(v) })

	for _, test := range meterTestCases {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = metricskey.WithResource(ctx, test.resource)
			meter := meterExporterForResource(metricskey.GetResource(ctx)).m
			setCurMetricsConfig(test.metricsConfig)
			Record(ctx, test.measurement)
			metricstest.CheckLastValueDataWithMeter(t, test.measurement.Measure().Name(), map[string]string{}, test.measurement.Value(), meter)
		})
	}
}

func BenchmarkMetricsRecording(b *testing.B) {
	requestKey := tag.MustNewKey("request")
	requestStatus := tag.MustNewKey("requestStatus")
	requestURL := tag.MustNewKey("requestUrl")
	tagKeys := []tag.Key{requestKey, requestStatus, requestURL}
	measure1 := stats.Int64("count1", "First counter", stats.UnitDimensionless)
	measure2 := stats.Int64("count2", "Second counter", stats.UnitDimensionless)
	v := []*view.View{{
		Measure:     measure1,
		Aggregation: view.LastValue(),
		TagKeys:     tagKeys,
	}, {
		Measure:     measure2,
		Aggregation: view.LastValue(),
		TagKeys:     tagKeys,
	}}
	err := RegisterResourceView(v...)
	if err != nil {
		b.Error("Failed to register resource view")
	}
	defer UnregisterResourceView(v...)
	metricsConfig := &metricsConfig{}
	measurement1 := measure1.M(1000)
	measurement2 := measure2.M(1)
	setCurMetricsConfig(metricsConfig)
	getTagCtx := func() (context.Context, error) {
		ctx := context.Background()
		ctx, err := tag.New(
			ctx,
			tag.Insert(requestKey, "login"),
			tag.Insert(requestStatus, "status_ok"),
			tag.Insert(requestURL, "localhost"),
		)
		return ctx, err
	}
	if err != nil {
		b.Error("Failed to create tags")
	}
	b.Run("sequential", func(b *testing.B) {
		for range b.N {
			ctx, err := getTagCtx()
			if err != nil {
				b.Error("Failed to get context")
			}
			Record(ctx, measurement1)
		}
	})
	b.Run("parallel", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				ctx, err := getTagCtx()
				if err != nil {
					b.Error("Failed to get context")
				}
				Record(ctx, measurement1)
			}
		})
	})
	b.Run("sequential-batch", func(b *testing.B) {
		for range b.N {
			ctx, err := getTagCtx()
			if err != nil {
				b.Error("Failed to get context")
			}
			RecordBatch(ctx, measurement1, measurement2)
		}
	})
	b.Run("parallel-batch", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				ctx, err := getTagCtx()
				if err != nil {
					b.Error("Failed to get context")
				}
				RecordBatch(ctx, measurement1, measurement2)
			}
		})
	})
}
