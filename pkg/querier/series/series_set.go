// Some of the code in this file was adapted from Prometheus (https://github.com/prometheus/prometheus).
// The original license header is included below:
//
// Copyright 2017 The Prometheus Authors
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

package series

import (
	"context"
	"slices"
	"sort"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"

	"github.com/cortexproject/cortex/pkg/querier/iterators"
	"github.com/cortexproject/cortex/pkg/util"
)

type labelsSetSeriesSet struct {
	cur       int
	labelsSet []labels.Labels
}

// LabelsSetToSeriesSet creates a storage.SeriesSet from a []labels.Labels.
func LabelsSetToSeriesSet(sortSeries bool, labelsSet []labels.Labels) storage.SeriesSet {
	if sortSeries {
		slices.SortFunc(labelsSet, func(a, b labels.Labels) int { return labels.Compare(a, b) })
	}
	return &labelsSetSeriesSet{
		cur:       -1,
		labelsSet: labelsSet,
	}
}

// Next iterates through a series set and implements storage.SeriesSet.
func (c *labelsSetSeriesSet) Next() bool {
	c.cur++
	return c.cur < len(c.labelsSet)
}

// At returns the current series and implements storage.SeriesSet.
func (c *labelsSetSeriesSet) At() storage.Series {
	lset := c.labelsSet[c.cur]
	return &ConcreteSeries{labels: lset}
}

// Err implements storage.SeriesSet.
func (c *labelsSetSeriesSet) Err() error {
	return nil
}

// Warnings implements storage.SeriesSet.
func (c *labelsSetSeriesSet) Warnings() annotations.Annotations {
	return nil
}

// ConcreteSeriesSet implements storage.SeriesSet.
type ConcreteSeriesSet struct {
	cur    int
	series []storage.Series
}

// NewConcreteSeriesSet instantiates an in-memory series set from a series
// Series will be sorted by labels if sortSeries is set.
func NewConcreteSeriesSet(sortSeries bool, series []storage.Series) storage.SeriesSet {
	if sortSeries {
		sort.Sort(byLabels(series))
	}
	return &ConcreteSeriesSet{
		cur:    -1,
		series: series,
	}
}

// Next iterates through a series set and implements storage.SeriesSet.
func (c *ConcreteSeriesSet) Next() bool {
	c.cur++
	return c.cur < len(c.series)
}

// At returns the current series and implements storage.SeriesSet.
func (c *ConcreteSeriesSet) At() storage.Series {
	return c.series[c.cur]
}

// Err implements storage.SeriesSet.
func (c *ConcreteSeriesSet) Err() error {
	return nil
}

// Warnings implements storage.SeriesSet.
func (c *ConcreteSeriesSet) Warnings() annotations.Annotations {
	return nil
}

// ConcreteSeries implements storage.Series.
type ConcreteSeries struct {
	labels  labels.Labels
	samples []model.SamplePair
}

// NewConcreteSeries instantiates an in memory series from a list of samples & labels
func NewConcreteSeries(ls labels.Labels, samples []model.SamplePair) *ConcreteSeries {
	return &ConcreteSeries{
		labels:  ls,
		samples: samples,
	}
}

// Labels implements storage.Series
func (c *ConcreteSeries) Labels() labels.Labels {
	return c.labels
}

// Iterator implements storage.Series
func (c *ConcreteSeries) Iterator(chunkenc.Iterator) chunkenc.Iterator {
	return NewConcreteSeriesIterator(c)
}

// concreteSeriesIterator implements chunkenc.Iterator.
type concreteSeriesIterator struct {
	cur    int
	series *ConcreteSeries
}

// NewConcreteSeriesIterator instaniates an in memory chunkenc.Iterator
func NewConcreteSeriesIterator(series *ConcreteSeries) chunkenc.Iterator {
	return iterators.NewCompatibleChunksIterator(&concreteSeriesIterator{
		cur:    -1,
		series: series,
	})
}

func (c *concreteSeriesIterator) Seek(t int64) bool {
	c.cur = sort.Search(len(c.series.samples), func(n int) bool {
		return c.series.samples[n].Timestamp >= model.Time(t)
	})
	return c.cur < len(c.series.samples)
}

func (c *concreteSeriesIterator) At() (t int64, v float64) {
	s := c.series.samples[c.cur]
	return int64(s.Timestamp), float64(s.Value)
}

func (c *concreteSeriesIterator) Next() bool {
	c.cur++
	return c.cur < len(c.series.samples)
}

func (c *concreteSeriesIterator) Err() error {
	return nil
}

// MatrixToSeriesSet creates a storage.SeriesSet from a model.Matrix
// Series will be sorted by labels if sortSeries is set.
func MatrixToSeriesSet(sortSeries bool, m model.Matrix) storage.SeriesSet {
	series := make([]storage.Series, 0, len(m))
	for _, ss := range m {
		series = append(series, &ConcreteSeries{
			labels:  metricToLabels(ss.Metric),
			samples: ss.Values,
		})
	}
	return NewConcreteSeriesSet(sortSeries, series)
}

// MetricsToSeriesSet creates a storage.SeriesSet from a []metric.Metric
func MetricsToSeriesSet(ctx context.Context, sortSeries bool, ms []model.Metric) storage.SeriesSet {
	series := make([]storage.Series, 0, len(ms))
	for i, m := range ms {
		if (i+1)%util.CheckContextEveryNIterations == 0 && ctx.Err() != nil {
			return storage.ErrSeriesSet(ctx.Err())
		}
		series = append(series, &ConcreteSeries{
			labels: metricToLabels(m),
		})
	}
	return NewConcreteSeriesSet(sortSeries, series)
}

func metricToLabels(m model.Metric) labels.Labels {
	builder := labels.NewBuilder(labels.EmptyLabels())
	for k, v := range m {
		builder.Set(string(k), string(v))

	}
	return builder.Labels()
}

type byLabels []storage.Series

func (b byLabels) Len() int           { return len(b) }
func (b byLabels) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byLabels) Less(i, j int) bool { return labels.Compare(b[i].Labels(), b[j].Labels()) < 0 }

type seriesSetWithWarnings struct {
	wrapped  storage.SeriesSet
	warnings annotations.Annotations
}

func NewSeriesSetWithWarnings(wrapped storage.SeriesSet, warnings annotations.Annotations) storage.SeriesSet {
	return seriesSetWithWarnings{
		wrapped:  wrapped,
		warnings: warnings,
	}
}

func (s seriesSetWithWarnings) Next() bool {
	return s.wrapped.Next()
}

func (s seriesSetWithWarnings) At() storage.Series {
	return s.wrapped.At()
}

func (s seriesSetWithWarnings) Err() error {
	return s.wrapped.Err()
}

func (s seriesSetWithWarnings) Warnings() annotations.Annotations {
	w := s.wrapped.Warnings()
	return w.Merge(s.warnings)
}
