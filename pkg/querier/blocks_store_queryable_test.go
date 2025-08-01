package querier

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/gogo/protobuf/types"
	"github.com/oklog/ulid/v2"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/promql-engine/engine"
	"github.com/thanos-io/promql-engine/logicalplan"
	"github.com/thanos-io/thanos/pkg/pool"
	"github.com/thanos-io/thanos/pkg/store/hintspb"
	"github.com/thanos-io/thanos/pkg/store/labelpb"
	"github.com/thanos-io/thanos/pkg/store/storepb"
	"github.com/weaveworks/common/user"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cortexproject/cortex/pkg/chunk/encoding"
	"github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/cortexproject/cortex/pkg/storage/tsdb/bucketindex"
	"github.com/cortexproject/cortex/pkg/storegateway"
	"github.com/cortexproject/cortex/pkg/storegateway/storegatewaypb"
	"github.com/cortexproject/cortex/pkg/util"
	"github.com/cortexproject/cortex/pkg/util/limiter"
	util_log "github.com/cortexproject/cortex/pkg/util/log"
	"github.com/cortexproject/cortex/pkg/util/services"
	"github.com/cortexproject/cortex/pkg/util/validation"
)

func TestBlocksStoreQuerier_Select(t *testing.T) {
	t.Parallel()

	const (
		metricName = "test_metric"
		minT       = int64(10)
		maxT       = int64(20)
	)

	var (
		block1              = ulid.MustNew(1, nil)
		block2              = ulid.MustNew(2, nil)
		block3              = ulid.MustNew(3, nil)
		block4              = ulid.MustNew(4, nil)
		metricNameLabel     = labels.Label{Name: labels.MetricName, Value: metricName}
		series1Label        = labels.Label{Name: "series", Value: "1"}
		series2Label        = labels.Label{Name: "series", Value: "2"}
		noOpQueryLimiter    = limiter.NewQueryLimiter(0, 0, 0, 0)
		testHistogram1      = tsdbutil.GenerateTestHistogram(1)
		testHistogram2      = tsdbutil.GenerateTestHistogram(2)
		testHistogram3      = tsdbutil.GenerateTestHistogram(3)
		testFloatHistogram1 = tsdbutil.GenerateTestFloatHistogram(1)
		testFloatHistogram2 = tsdbutil.GenerateTestFloatHistogram(2)
		testFloatHistogram3 = tsdbutil.GenerateTestFloatHistogram(3)
	)

	type valueResult struct {
		t  int64
		v  float64
		h  *histogram.Histogram
		fh *histogram.FloatHistogram
	}

	type seriesResult struct {
		lbls   labels.Labels
		values []valueResult
	}

	tests := map[string]struct {
		finderResult      bucketindex.Blocks
		finderErr         error
		storeSetResponses []interface{}
		limits            BlocksStoreLimits
		queryLimiter      *limiter.QueryLimiter
		seriesLimit       int
		expectedSeries    []seriesResult
		expectedErr       error
		expectedMetrics   string
	}{
		"no block in the storage matching the query time range": {
			finderResult: nil,
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedErr:  nil,
		},
		"error while finding blocks matching the query time range": {
			finderErr:    errors.New("unable to find blocks"),
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedErr:  errors.New("unable to find blocks"),
		},
		"error while getting clients to query the store-gateway": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				errors.New("no client found"),
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedErr:  errors.New("no client found"),
		},
		"a single store-gateway instance holds the required blocks (single returned series)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value), []cortexpb.Sample{{Value: 1, TimestampMs: minT}, {Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel),
					values: []valueResult{
						{t: minT, v: 1},
						{t: minT + 1, v: 2},
					},
				},
			},
		},
		"a single store-gateway instance holds the required blocks (single returned histogram series)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value),
							nil,
							[]cortexpb.Histogram{
								cortexpb.HistogramToHistogramProto(minT, testHistogram1),
								cortexpb.HistogramToHistogramProto(minT+1, testHistogram2),
							}, nil,
						),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel),
					values: []valueResult{
						{t: minT, h: testHistogram1},
						{t: minT + 1, h: testHistogram2},
					},
				},
			},
		},
		"a single store-gateway instance holds the required blocks (single returned float histogram series)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value),
							nil, nil,
							[]cortexpb.Histogram{
								cortexpb.FloatHistogramToHistogramProto(minT, testFloatHistogram1),
								cortexpb.FloatHistogramToHistogramProto(minT+1, testFloatHistogram2),
							},
						),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel),
					values: []valueResult{
						{t: minT, fh: testFloatHistogram1},
						{t: minT + 1, fh: testFloatHistogram2},
					},
				},
			},
		},
		"a single store-gateway instance holds the required blocks (multiple returned series)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 1, TimestampMs: minT}, {Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series2Label.Name, series2Label.Value), []cortexpb.Sample{{Value: 3, TimestampMs: minT}}, nil, nil),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel, series1Label),
					values: []valueResult{
						{t: minT, v: 1},
						{t: minT + 1, v: 2},
					},
				}, {
					lbls: labels.New(metricNameLabel, series2Label),
					values: []valueResult{
						{t: minT, v: 3},
					},
				},
			},
		},
		"a single store-gateway instance holds the required blocks (multiple returned histogram series)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value),
							nil,
							[]cortexpb.Histogram{
								cortexpb.HistogramToHistogramProto(minT, testHistogram1),
								cortexpb.HistogramToHistogramProto(minT+1, testHistogram2),
							}, nil,
						),
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series2Label.Name, series2Label.Value),
							nil,
							[]cortexpb.Histogram{
								cortexpb.HistogramToHistogramProto(minT, testHistogram3),
							}, nil,
						),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel, series1Label),
					values: []valueResult{
						{t: minT, h: testHistogram1},
						{t: minT + 1, h: testHistogram2},
					},
				}, {
					lbls: labels.New(metricNameLabel, series2Label),
					values: []valueResult{
						{t: minT, h: testHistogram3},
					},
				},
			},
		},
		"a single store-gateway instance holds the required blocks (multiple returned float histogram series)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value),
							nil, nil,
							[]cortexpb.Histogram{
								cortexpb.FloatHistogramToHistogramProto(minT, testFloatHistogram1),
								cortexpb.FloatHistogramToHistogramProto(minT+1, testFloatHistogram2),
							},
						),
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series2Label.Name, series2Label.Value),
							nil, nil,
							[]cortexpb.Histogram{
								cortexpb.FloatHistogramToHistogramProto(minT, testFloatHistogram3),
							},
						),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel, series1Label),
					values: []valueResult{
						{t: minT, fh: testFloatHistogram1},
						{t: minT + 1, fh: testFloatHistogram2},
					},
				}, {
					lbls: labels.New(metricNameLabel, series2Label),
					values: []valueResult{
						{t: minT, fh: testFloatHistogram3},
					},
				},
			},
		},
		"multiple store-gateway instances holds the required blocks without overlapping series (single returned series)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value), []cortexpb.Sample{{Value: 1, TimestampMs: minT}}, nil, nil),
						mockHintsResponse(block1),
					}}: {block1},
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block2),
					}}: {block2},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel),
					values: []valueResult{
						{t: minT, v: 1},
						{t: minT + 1, v: 2},
					},
				},
			},
		},
		"multiple store-gateway instances holds the required blocks without overlapping series (single returned histogram series)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value),
							nil,
							[]cortexpb.Histogram{
								cortexpb.HistogramToHistogramProto(minT, testHistogram1),
							}, nil,
						),
						mockHintsResponse(block1),
					}}: {block1},
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value),
							nil,
							[]cortexpb.Histogram{
								cortexpb.HistogramToHistogramProto(minT+1, testHistogram2),
							}, nil,
						),
						mockHintsResponse(block2),
					}}: {block2},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel),
					values: []valueResult{
						{t: minT, h: testHistogram1},
						{t: minT + 1, h: testHistogram2},
					},
				},
			},
		},
		"multiple store-gateway instances holds the required blocks without overlapping series (single returned float histogram series)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value),
							nil, nil,
							[]cortexpb.Histogram{
								cortexpb.FloatHistogramToHistogramProto(minT, testFloatHistogram1),
							},
						),
						mockHintsResponse(block1),
					}}: {block1},
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value),
							nil, nil,
							[]cortexpb.Histogram{
								cortexpb.FloatHistogramToHistogramProto(minT+1, testFloatHistogram2),
							},
						),
						mockHintsResponse(block2),
					}}: {block2},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel),
					values: []valueResult{
						{t: minT, fh: testFloatHistogram1},
						{t: minT + 1, fh: testFloatHistogram2},
					},
				},
			},
		},
		"multiple store-gateway instances holds the required blocks with overlapping series (single returned series)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block1),
					}}: {block1},
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value), []cortexpb.Sample{{Value: 1, TimestampMs: minT}, {Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block2),
					}}: {block2},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel),
					values: []valueResult{
						{t: minT, v: 1},
						{t: minT + 1, v: 2},
					},
				},
			},
		},
		"multiple store-gateway instances holds the required blocks with overlapping series (single returned histogram series)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value),
							nil,
							[]cortexpb.Histogram{
								cortexpb.HistogramToHistogramProto(minT+1, testHistogram2),
							}, nil,
						),
						mockHintsResponse(block1),
					}}: {block1},
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value),
							nil,
							[]cortexpb.Histogram{
								cortexpb.HistogramToHistogramProto(minT, testHistogram1),
								cortexpb.HistogramToHistogramProto(minT+1, testHistogram2),
							}, nil,
						),
						mockHintsResponse(block2),
					}}: {block2},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel),
					values: []valueResult{
						{t: minT, h: testHistogram1},
						{t: minT + 1, h: testHistogram2},
					},
				},
			},
		},
		"multiple store-gateway instances holds the required blocks with overlapping series (single returned float histogram series)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value),
							nil, nil,
							[]cortexpb.Histogram{
								cortexpb.FloatHistogramToHistogramProto(minT+1, testFloatHistogram2),
							},
						),
						mockHintsResponse(block1),
					}}: {block1},
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value),
							nil, nil,
							[]cortexpb.Histogram{
								cortexpb.FloatHistogramToHistogramProto(minT, testFloatHistogram1),
								cortexpb.FloatHistogramToHistogramProto(minT+1, testFloatHistogram2),
							},
						),
						mockHintsResponse(block2),
					}}: {block2},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel),
					values: []valueResult{
						{t: minT, fh: testFloatHistogram1},
						{t: minT + 1, fh: testFloatHistogram2},
					},
				},
			},
		},
		"multiple store-gateway instances holds the required blocks with overlapping series (multiple returned series)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series2Label.Name, series2Label.Value), []cortexpb.Sample{{Value: 1, TimestampMs: minT}}, nil, nil),
						mockHintsResponse(block1),
					}}: {block1},
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 1, TimestampMs: minT}, {Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block2),
					}}: {block2},
					&storeGatewayClientMock{remoteAddr: "3.3.3.3", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series2Label.Name, series2Label.Value), []cortexpb.Sample{{Value: 1, TimestampMs: minT}, {Value: 3, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block3),
					}}: {block3},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel, series1Label),
					values: []valueResult{
						{t: minT, v: 1},
						{t: minT + 1, v: 2},
					},
				}, {
					lbls: labels.New(metricNameLabel, series2Label),
					values: []valueResult{
						{t: minT, v: 1},
						{t: minT + 1, v: 3},
					},
				},
			},
			expectedMetrics: `
				# HELP cortex_querier_storegateway_instances_hit_per_query Number of store-gateway instances hit for a single query.
				# TYPE cortex_querier_storegateway_instances_hit_per_query histogram
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="0"} 0
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="1"} 0
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="2"} 0
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="3"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="4"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="5"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="6"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="7"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="8"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="9"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="10"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="+Inf"} 1
				cortex_querier_storegateway_instances_hit_per_query_sum 3
				cortex_querier_storegateway_instances_hit_per_query_count 1

				# HELP cortex_querier_storegateway_refetches_per_query Number of re-fetches attempted while querying store-gateway instances due to missing blocks.
				# TYPE cortex_querier_storegateway_refetches_per_query histogram
				cortex_querier_storegateway_refetches_per_query_bucket{le="0"} 1
				cortex_querier_storegateway_refetches_per_query_bucket{le="1"} 1
				cortex_querier_storegateway_refetches_per_query_bucket{le="2"} 1
				cortex_querier_storegateway_refetches_per_query_bucket{le="4"} 1
				cortex_querier_storegateway_refetches_per_query_bucket{le="8"} 1
				cortex_querier_storegateway_refetches_per_query_bucket{le="+Inf"} 1
				cortex_querier_storegateway_refetches_per_query_sum 0
				cortex_querier_storegateway_refetches_per_query_count 1
			`,
		},
		"multiple store-gateway instances holds the required blocks with overlapping series with limit (multiple returned series)": {
			seriesLimit: 1,
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series2Label.Name, series2Label.Value), []cortexpb.Sample{{Value: 1, TimestampMs: minT}}, nil, nil),
						mockHintsResponse(block1),
					}}: {block1},
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 1, TimestampMs: minT}, {Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block2),
					}}: {block2},
					&storeGatewayClientMock{remoteAddr: "3.3.3.3", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series2Label.Name, series2Label.Value), []cortexpb.Sample{{Value: 1, TimestampMs: minT}, {Value: 3, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block3),
					}}: {block3},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel, series1Label),
					values: []valueResult{
						{t: minT, v: 1},
						{t: minT + 1, v: 2},
					},
				},
			},
		},
		"multiple store-gateway instances holds the required blocks with overlapping series (multiple returned histogram series)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value),
							nil,
							[]cortexpb.Histogram{
								cortexpb.HistogramToHistogramProto(minT+1, testHistogram2),
							}, nil,
						),
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series2Label.Name, series2Label.Value),
							nil,
							[]cortexpb.Histogram{
								cortexpb.HistogramToHistogramProto(minT, testHistogram1),
							}, nil,
						),
						mockHintsResponse(block1),
					}}: {block1},
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value),
							nil,
							[]cortexpb.Histogram{
								cortexpb.HistogramToHistogramProto(minT, testHistogram1),
								cortexpb.HistogramToHistogramProto(minT+1, testHistogram2),
							}, nil,
						),
						mockHintsResponse(block2),
					}}: {block2},
					&storeGatewayClientMock{remoteAddr: "3.3.3.3", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series2Label.Name, series2Label.Value),
							nil,
							[]cortexpb.Histogram{
								cortexpb.HistogramToHistogramProto(minT, testHistogram1),
								cortexpb.HistogramToHistogramProto(minT+1, testHistogram3),
							}, nil,
						),
						mockHintsResponse(block3),
					}}: {block3},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel, series1Label),
					values: []valueResult{
						{t: minT, h: testHistogram1},
						{t: minT + 1, h: testHistogram2},
					},
				}, {
					lbls: labels.New(metricNameLabel, series2Label),
					values: []valueResult{
						{t: minT, h: testHistogram1},
						{t: minT + 1, h: testHistogram3},
					},
				},
			},
		},
		"multiple store-gateway instances holds the required blocks with overlapping series (multiple returned float histogram series)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value),
							nil, nil,
							[]cortexpb.Histogram{
								cortexpb.FloatHistogramToHistogramProto(minT+1, testFloatHistogram2),
							},
						),
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series2Label.Name, series2Label.Value),
							nil, nil,
							[]cortexpb.Histogram{
								cortexpb.FloatHistogramToHistogramProto(minT, testFloatHistogram1),
							},
						),
						mockHintsResponse(block1),
					}}: {block1},
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value),
							nil, nil,
							[]cortexpb.Histogram{
								cortexpb.FloatHistogramToHistogramProto(minT, testFloatHistogram1),
								cortexpb.FloatHistogramToHistogramProto(minT+1, testFloatHistogram2),
							},
						),
						mockHintsResponse(block2),
					}}: {block2},
					&storeGatewayClientMock{remoteAddr: "3.3.3.3", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(
							labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series2Label.Name, series2Label.Value),
							nil, nil,
							[]cortexpb.Histogram{
								cortexpb.FloatHistogramToHistogramProto(minT, testFloatHistogram1),
								cortexpb.FloatHistogramToHistogramProto(minT+1, testFloatHistogram3),
							},
						),
						mockHintsResponse(block3),
					}}: {block3},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel, series1Label),
					values: []valueResult{
						{t: minT, fh: testFloatHistogram1},
						{t: minT + 1, fh: testFloatHistogram2},
					},
				}, {
					lbls: labels.New(metricNameLabel, series2Label),
					values: []valueResult{
						{t: minT, fh: testFloatHistogram1},
						{t: minT + 1, fh: testFloatHistogram3},
					},
				},
			},
		},
		"a single store-gateway instance has some missing blocks (consistency check failed)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				// First attempt returns a client whose response does not include all expected blocks.
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 1, TimestampMs: minT}, {Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block1),
					}}: {block1},
				},
				// Second attempt returns an error because there are no other store-gateways left.
				errors.New("no store-gateway remaining after exclude"),
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedErr:  fmt.Errorf("consistency check failed because some blocks were not queried: %s", block2.String()),
		},
		"multiple store-gateway instances have some missing blocks (consistency check failed)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
				&bucketindex.Block{ID: block3},
				&bucketindex.Block{ID: block4},
			},
			storeSetResponses: []interface{}{
				// First attempt returns a client whose response does not include all expected blocks.
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block1),
					}}: {block1},
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block2),
					}}: {block2},
				},
				// Second attempt returns an error because there are no other store-gateways left.
				errors.New("no store-gateway remaining after exclude"),
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedErr:  fmt.Errorf("consistency check failed because some blocks were not queried: %s %s", block3.String(), block4.String()),
		},
		"multiple store-gateway instances have some missing blocks but queried from a replica during subsequent attempts": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
				&bucketindex.Block{ID: block3},
				&bucketindex.Block{ID: block4},
			},
			storeSetResponses: []interface{}{
				// First attempt returns a client whose response does not include all expected blocks.
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 1, TimestampMs: minT}}, nil, nil),
						mockHintsResponse(block1),
					}}: {block1, block3},
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series2Label.Name, series2Label.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT}}, nil, nil),
						mockHintsResponse(block2),
					}}: {block2, block4},
				},
				// Second attempt returns 1 missing block.
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "3.3.3.3", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block3),
					}}: {block3, block4},
				},
				// Third attempt returns the last missing block.
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "4.4.4.4", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series2Label.Name, series2Label.Value), []cortexpb.Sample{{Value: 3, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block4),
					}}: {block4},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel, series1Label),
					values: []valueResult{
						{t: minT, v: 1},
						{t: minT + 1, v: 2},
					},
				}, {
					lbls: labels.New(metricNameLabel, series2Label),
					values: []valueResult{
						{t: minT, v: 2},
						{t: minT + 1, v: 3},
					},
				},
			},
			expectedMetrics: `
				# HELP cortex_querier_storegateway_instances_hit_per_query Number of store-gateway instances hit for a single query.
				# TYPE cortex_querier_storegateway_instances_hit_per_query histogram
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="0"} 0
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="1"} 0
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="2"} 0
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="3"} 0
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="4"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="5"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="6"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="7"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="8"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="9"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="10"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="+Inf"} 1
				cortex_querier_storegateway_instances_hit_per_query_sum 4
				cortex_querier_storegateway_instances_hit_per_query_count 1

				# HELP cortex_querier_storegateway_refetches_per_query Number of re-fetches attempted while querying store-gateway instances due to missing blocks.
				# TYPE cortex_querier_storegateway_refetches_per_query histogram
				cortex_querier_storegateway_refetches_per_query_bucket{le="0"} 0
				cortex_querier_storegateway_refetches_per_query_bucket{le="1"} 0
				cortex_querier_storegateway_refetches_per_query_bucket{le="2"} 1
				cortex_querier_storegateway_refetches_per_query_bucket{le="4"} 1
				cortex_querier_storegateway_refetches_per_query_bucket{le="8"} 1
				cortex_querier_storegateway_refetches_per_query_bucket{le="+Inf"} 1
				cortex_querier_storegateway_refetches_per_query_sum 2
				cortex_querier_storegateway_refetches_per_query_count 1
			`,
		},
		"max chunks per query limit greater then the number of chunks fetched": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 1, TimestampMs: minT}, {Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{maxChunksPerQuery: 3},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel, series1Label),
					values: []valueResult{
						{t: minT, v: 1},
						{t: minT + 1, v: 2},
					},
				},
			},
		},
		"max chunks per query limit hit while fetching chunks at first attempt": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 1, TimestampMs: minT}, {Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{maxChunksPerQuery: 1},
			queryLimiter: noOpQueryLimiter,
			expectedErr:  validation.LimitError(fmt.Sprintf(errMaxChunksPerQueryLimit, fmt.Sprintf("{__name__=%q}", metricName), 1)),
		},
		"max chunks per query limit hit while fetching histogram chunks at first attempt": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), nil, []cortexpb.Histogram{
							cortexpb.HistogramToHistogramProto(minT, testHistogram1),
							cortexpb.HistogramToHistogramProto(minT+1, testHistogram2),
						}, nil),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{maxChunksPerQuery: 1},
			queryLimiter: noOpQueryLimiter,
			expectedErr:  validation.LimitError(fmt.Sprintf(errMaxChunksPerQueryLimit, fmt.Sprintf("{__name__=%q}", metricName), 1)),
		},
		"max chunks per query limit hit while fetching float histogram chunks at first attempt": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), nil, nil, []cortexpb.Histogram{
							cortexpb.FloatHistogramToHistogramProto(minT, testFloatHistogram1),
							cortexpb.FloatHistogramToHistogramProto(minT+1, testFloatHistogram2),
						}),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{maxChunksPerQuery: 1},
			queryLimiter: noOpQueryLimiter,
			expectedErr:  validation.LimitError(fmt.Sprintf(errMaxChunksPerQueryLimit, fmt.Sprintf("{__name__=%q}", metricName), 1)),
		},
		"max chunks per query limit hit while fetching chunks at first attempt - global limit": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 1, TimestampMs: minT}, {Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: limiter.NewQueryLimiter(0, 0, 1, 0),
			expectedErr:  validation.LimitError(fmt.Sprintf(limiter.ErrMaxChunksPerQueryLimit, 1)),
		},
		"max chunks per query limit hit while fetching histogram chunks at first attempt - global limit": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), nil, []cortexpb.Histogram{
							cortexpb.HistogramToHistogramProto(minT, testHistogram1),
							cortexpb.HistogramToHistogramProto(minT+1, testHistogram2),
						}, nil),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: limiter.NewQueryLimiter(0, 0, 1, 0),
			expectedErr:  validation.LimitError(fmt.Sprintf(limiter.ErrMaxChunksPerQueryLimit, 1)),
		},
		"max chunks per query limit hit while fetching float histogram chunks at first attempt - global limit": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), nil, nil, []cortexpb.Histogram{
							cortexpb.FloatHistogramToHistogramProto(minT, testFloatHistogram1),
							cortexpb.FloatHistogramToHistogramProto(minT+1, testFloatHistogram2),
						}),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: limiter.NewQueryLimiter(0, 0, 1, 0),
			expectedErr:  validation.LimitError(fmt.Sprintf(limiter.ErrMaxChunksPerQueryLimit, 1)),
		},
		"max chunks per query limit hit while fetching chunks during subsequent attempts": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
				&bucketindex.Block{ID: block3},
				&bucketindex.Block{ID: block4},
			},
			storeSetResponses: []interface{}{
				// First attempt returns a client whose response does not include all expected blocks.
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 1, TimestampMs: minT}}, nil, nil),
						mockHintsResponse(block1),
					}}: {block1, block3},
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series2Label.Name, series2Label.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT}}, nil, nil),
						mockHintsResponse(block2),
					}}: {block2, block4},
				},
				// Second attempt returns 1 missing block.
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "3.3.3.3", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block3),
					}}: {block3, block4},
				},
				// Third attempt returns the last missing block.
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "4.4.4.4", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 3, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block4),
					}}: {block4},
				},
			},
			limits:       &blocksStoreLimitsMock{maxChunksPerQuery: 3},
			queryLimiter: noOpQueryLimiter,
			expectedErr:  validation.LimitError(fmt.Sprintf(errMaxChunksPerQueryLimit, fmt.Sprintf("{__name__=%q}", metricName), 3)),
		},
		"max chunks per query limit hit while fetching chunks during subsequent attempts - global": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
				&bucketindex.Block{ID: block3},
				&bucketindex.Block{ID: block4},
			},
			storeSetResponses: []interface{}{
				// First attempt returns a client whose response does not include all expected blocks.
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 1, TimestampMs: minT}}, nil, nil),
						mockHintsResponse(block1),
					}}: {block1, block3},
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series2Label.Name, series2Label.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT}}, nil, nil),
						mockHintsResponse(block2),
					}}: {block2, block4},
				},
				// Second attempt returns 1 missing block.
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "3.3.3.3", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block3),
					}}: {block3, block4},
				},
				// Third attempt returns the last missing block.
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "4.4.4.4", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series2Label.Name, series2Label.Value), []cortexpb.Sample{{Value: 3, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block4),
					}}: {block4},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: limiter.NewQueryLimiter(0, 0, 3, 0),
			expectedErr:  validation.LimitError(fmt.Sprintf(limiter.ErrMaxChunksPerQueryLimit, 3)),
		},
		"max series per query limit hit while fetching chunks": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 1, TimestampMs: minT}}, nil, nil),
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series2Label.Name, series2Label.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: limiter.NewQueryLimiter(1, 0, 0, 0),
			expectedErr:  validation.LimitError(fmt.Sprintf(limiter.ErrMaxSeriesHit, 1)),
		},
		"max series per query limit hit while fetching histogram chunks": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), nil,
							[]cortexpb.Histogram{
								cortexpb.HistogramToHistogramProto(minT, testHistogram1),
							}, nil,
						),
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series2Label.Name, series2Label.Value), nil,
							[]cortexpb.Histogram{
								cortexpb.HistogramToHistogramProto(minT+1, testHistogram2),
							}, nil,
						),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: limiter.NewQueryLimiter(1, 0, 0, 0),
			expectedErr:  validation.LimitError(fmt.Sprintf(limiter.ErrMaxSeriesHit, 1)),
		},
		"max series per query limit hit while fetching float histogram chunks": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), nil, nil,
							[]cortexpb.Histogram{
								cortexpb.FloatHistogramToHistogramProto(minT, testFloatHistogram1),
							},
						),
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series2Label.Name, series2Label.Value), nil, nil,
							[]cortexpb.Histogram{
								cortexpb.FloatHistogramToHistogramProto(minT+1, testFloatHistogram2),
							},
						),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: limiter.NewQueryLimiter(1, 0, 0, 0),
			expectedErr:  validation.LimitError(fmt.Sprintf(limiter.ErrMaxSeriesHit, 1)),
		},
		"max chunk bytes per query limit hit while fetching chunks": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 1, TimestampMs: minT}, {Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{maxChunksPerQuery: 0},
			queryLimiter: limiter.NewQueryLimiter(0, 8, 0, 0),
			expectedErr:  validation.LimitError(fmt.Sprintf(limiter.ErrMaxChunkBytesHit, 8)),
		},
		"max chunk bytes per query limit hit while fetching histogram chunks": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), nil, []cortexpb.Histogram{
							cortexpb.HistogramToHistogramProto(minT, testHistogram1),
						}, nil),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{maxChunksPerQuery: 0},
			queryLimiter: limiter.NewQueryLimiter(0, 8, 0, 0),
			expectedErr:  validation.LimitError(fmt.Sprintf(limiter.ErrMaxChunkBytesHit, 8)),
		},
		"max chunk bytes per query limit hit while fetching float histogram chunks": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), nil, nil, []cortexpb.Histogram{
							cortexpb.FloatHistogramToHistogramProto(minT, testFloatHistogram1),
						}),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{maxChunksPerQuery: 0},
			queryLimiter: limiter.NewQueryLimiter(0, 8, 0, 0),
			expectedErr:  validation.LimitError(fmt.Sprintf(limiter.ErrMaxChunkBytesHit, 8)),
		},
		"max data bytes per query limit hit while fetching chunks": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 1, TimestampMs: minT}, {Value: 2, TimestampMs: minT + 1}}, nil, nil),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{maxChunksPerQuery: 0},
			queryLimiter: limiter.NewQueryLimiter(0, 0, 0, 1),
			expectedErr:  validation.LimitError(fmt.Sprintf(limiter.ErrMaxDataBytesHit, 1)),
		},
		"max data bytes per query limit hit while fetching histogram chunks": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), nil, []cortexpb.Histogram{
							cortexpb.HistogramToHistogramProto(minT, testHistogram1),
						}, nil),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{maxChunksPerQuery: 0},
			queryLimiter: limiter.NewQueryLimiter(0, 0, 0, 1),
			expectedErr:  validation.LimitError(fmt.Sprintf(limiter.ErrMaxDataBytesHit, 1)),
		},
		"max data bytes per query limit hit while fetching float histogram chunks": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), nil, nil, []cortexpb.Histogram{
							cortexpb.FloatHistogramToHistogramProto(minT, testFloatHistogram1),
						}),
						mockHintsResponse(block1, block2),
					}}: {block1, block2},
				},
			},
			limits:       &blocksStoreLimitsMock{maxChunksPerQuery: 0},
			queryLimiter: limiter.NewQueryLimiter(0, 0, 0, 1),
			expectedErr:  validation.LimitError(fmt.Sprintf(limiter.ErrMaxDataBytesHit, 1)),
		},
		"multiple store-gateways has the block, but one of them fails to return": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr:      "1.1.1.1",
						mockedSeriesErr: status.Error(codes.Unavailable, "unavailable"),
					}: {block1},
				},
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT}}, nil, nil),
						mockHintsResponse(block1),
					}}: {block1},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel, series1Label),
					values: []valueResult{
						{t: minT, v: 2},
					},
				},
			},
		},
		"multiple store-gateways has the block, but one of them fails to return due to clientconn closing": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr:      "1.1.1.1",
						mockedSeriesErr: status.Error(codes.Canceled, "grpc: the client connection is closing"),
					}: {block1},
				},
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT}}, nil, nil),
						mockHintsResponse(block1),
					}}: {block1},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel, series1Label),
					values: []valueResult{
						{t: minT, v: 2},
					},
				},
			},
		},
		"multiple store-gateways has the block, but one of them fails to return due to chunk pool exhaustion": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr:      "1.1.1.1",
						mockedSeriesErr: status.Error(codes.Unknown, pool.ErrPoolExhausted.Error()),
					}: {block1},
				},
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT}}, nil, nil),
						mockHintsResponse(block1),
					}}: {block1},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel, series1Label),
					values: []valueResult{
						{t: minT, v: 2},
					},
				},
			},
		},
		"all store-gateways return PermissionDenied": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
			},
			expectedErr: validation.AccessDeniedError("PermissionDenied"),
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr: "1.1.1.1",
						mockedSeriesResponses: []*storepb.SeriesResponse{
							mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT}}, nil, nil),
							mockHintsResponse(block1),
						},
						mockedSeriesStreamErr: status.Error(codes.PermissionDenied, "PermissionDenied"),
					}: {block1},
				},
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr: "2.2.2.2",
						mockedSeriesResponses: []*storepb.SeriesResponse{
							mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT}}, nil, nil),
							mockHintsResponse(block1),
						},
						mockedSeriesStreamErr: status.Error(codes.PermissionDenied, "PermissionDenied"),
					}: {block1},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel, series1Label),
					values: []valueResult{
						{t: minT, v: 2},
					},
				},
			},
		},
		"multiple store-gateways has the block, but one of them fails to return on stream": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr:            "1.1.1.1",
						mockedSeriesStreamErr: status.Error(codes.Unavailable, "unavailable"),
						mockedSeriesResponses: []*storepb.SeriesResponse{
							mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT}}, nil, nil),
							mockHintsResponse(block1),
						}}: {block1},
				},
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT}}, nil, nil),
						mockHintsResponse(block1),
					}}: {block1},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel, series1Label),
					values: []valueResult{
						{t: minT, v: 2},
					},
				},
			},
		},
		"multiple store-gateways has the block, but one of them had too many inflight requests": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr:      "1.1.1.1",
						mockedSeriesErr: storegateway.ErrTooManyInflightRequests,
					}: {block1},
				},
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT}}, nil, nil),
						mockHintsResponse(block1),
					}}: {block1},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel, series1Label),
					values: []valueResult{
						{t: minT, v: 2},
					},
				},
			},
		},
		"store gateway returns resource exhausted error other than max inflight request": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr:      "1.1.1.1",
						mockedSeriesErr: status.Error(codes.ResourceExhausted, "some other resource"),
					}: {block1},
				},
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT}}, nil, nil),
						mockHintsResponse(block1),
					}}: {block1},
				},
			},
			limits:      &blocksStoreLimitsMock{},
			expectedErr: errors.Wrapf(status.Error(codes.ResourceExhausted, "some other resource"), "failed to fetch series from 1.1.1.1"),
		},
		"multiple store-gateways has the block, but one of them had resource exhausted error from resource monitor": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr:      "1.1.1.1",
						mockedSeriesErr: &limiter.ResourceLimitReachedError{},
					}: {block1},
				},
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
						mockSeriesResponse(labels.FromStrings(metricNameLabel.Name, metricNameLabel.Value, series1Label.Name, series1Label.Value), []cortexpb.Sample{{Value: 2, TimestampMs: minT}}, nil, nil),
						mockHintsResponse(block1),
					}}: {block1},
				},
			},
			limits:       &blocksStoreLimitsMock{},
			queryLimiter: noOpQueryLimiter,
			expectedSeries: []seriesResult{
				{
					lbls: labels.New(metricNameLabel, series1Label),
					values: []valueResult{
						{t: minT, v: 2},
					},
				},
			},
		},
	}

	for testName, testData := range tests {
		testData := testData
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			var hints *storage.SelectHints
			if testData.seriesLimit > 0 {
				hints = &storage.SelectHints{
					Limit: testData.seriesLimit,
					Start: minT,
					End:   maxT,
				}
			}

			ctx := user.InjectOrgID(context.Background(), "user-1")
			ctx = limiter.AddQueryLimiterToContext(ctx, testData.queryLimiter)
			reg := prometheus.NewPedanticRegistry()
			stores := &blocksStoreSetMock{mockedResponses: testData.storeSetResponses}
			finder := &blocksFinderMock{}
			finder.On("GetBlocks", mock.Anything, "user-1", minT, maxT).Return(testData.finderResult, map[ulid.ULID]*bucketindex.BlockDeletionMark(nil), testData.finderErr)

			q := &blocksStoreQuerier{
				minT:        minT,
				maxT:        maxT,
				finder:      finder,
				stores:      stores,
				consistency: NewBlocksConsistencyChecker(0, 0, log.NewNopLogger(), nil),
				logger:      log.NewNopLogger(),
				metrics:     newBlocksStoreQueryableMetrics(reg),
				limits:      testData.limits,

				storeGatewayConsistencyCheckMaxAttempts: 3,
			}

			matchers := []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, metricName),
			}

			set := q.Select(ctx, true, hints, matchers...)
			if testData.expectedErr != nil {
				assert.EqualError(t, set.Err(), testData.expectedErr.Error())
				assert.IsType(t, set.Err(), testData.expectedErr)
				assert.False(t, set.Next())
				assert.Nil(t, set.Warnings())
				return
			}

			require.NoError(t, set.Err())
			assert.Len(t, set.Warnings(), 0)

			// Read all returned series and their values.
			var actualSeries []seriesResult
			var it chunkenc.Iterator
			for set.Next() {
				var actualValues []valueResult

				it = set.At().Iterator(it)
				for valType := it.Next(); valType != chunkenc.ValNone; valType = it.Next() {
					switch valType {
					case chunkenc.ValFloat:
						t, v := it.At()
						actualValues = append(actualValues, valueResult{
							t: t,
							v: v,
						})
					case chunkenc.ValHistogram:
						t, v := it.AtHistogram(nil)
						actualValues = append(actualValues, valueResult{
							t: t,
							h: v,
						})
					case chunkenc.ValFloatHistogram:
						t, v := it.AtFloatHistogram(nil)
						actualValues = append(actualValues, valueResult{
							t:  t,
							fh: v,
						})
					default:
					}
				}

				require.NoError(t, it.Err())

				actualSeries = append(actualSeries, seriesResult{
					lbls:   set.At().Labels(),
					values: actualValues,
				})
			}
			require.NoError(t, set.Err())
			assert.Equal(t, testData.expectedSeries, actualSeries)

			// Assert on metrics (optional, only for test cases defining it).
			if testData.expectedMetrics != "" {
				assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(testData.expectedMetrics)))
			}
		})
	}
}

func TestOverrideBlockDiscovery(t *testing.T) {
	block1 := ulid.MustNew(1, nil)
	block2 := ulid.MustNew(2, nil)
	minT := int64(10)
	maxT := int64(20)

	stores := &blocksStoreSetMock{mockedResponses: []interface{}{
		map[BlocksStoreClient][]ulid.ULID{
			&storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
				mockHintsResponse(block1),
			}}: {block1},
		},
	},
	}
	finder := &blocksFinderMock{}
	// return block 1 and 2 on finder but only query block 1
	finder.On("GetBlocks", mock.Anything, "user-1", minT, maxT).Return(bucketindex.Blocks{
		&bucketindex.Block{ID: block1},
		&bucketindex.Block{ID: block2},
	}, map[ulid.ULID]*bucketindex.BlockDeletionMark(nil), nil)

	q := &blocksStoreQuerier{
		minT:        minT,
		maxT:        maxT,
		finder:      finder,
		stores:      stores,
		consistency: NewBlocksConsistencyChecker(0, 0, log.NewNopLogger(), nil),
		logger:      log.NewNopLogger(),
		metrics:     newBlocksStoreQueryableMetrics(prometheus.NewPedanticRegistry()),
		limits:      &blocksStoreLimitsMock{},

		storeGatewayConsistencyCheckMaxAttempts: 3,
	}

	matchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "name"),
	}
	ctx := user.InjectOrgID(context.Background(), "user-1")
	ctx = InjectBlocksIntoContext(ctx, &bucketindex.Block{ID: block1})
	ss := q.Select(ctx, true, nil, matchers...)
	require.NoError(t, ss.Err())
}

func TestBlocksStoreQuerier_Labels(t *testing.T) {
	t.Parallel()

	const (
		metricName = "test_metric"
		minT       = int64(10)
		maxT       = int64(20)
	)

	var (
		block1  = ulid.MustNew(1, nil)
		block2  = ulid.MustNew(2, nil)
		block3  = ulid.MustNew(3, nil)
		block4  = ulid.MustNew(4, nil)
		series1 = labels.FromMap(map[string]string{
			labels.MetricName: metricName + "_1",
			"series1":         "1",
		})
		series2 = labels.FromMap(map[string]string{
			labels.MetricName: metricName + "_2",
			"series2":         "1",
		})
	)

	tests := map[string]struct {
		finderResult        bucketindex.Blocks
		finderErr           error
		limit               int
		storeSetResponses   []interface{}
		expectedLabelNames  []string
		expectedLabelValues []string // For __name__
		expectedErr         string
		expectedMetrics     string
	}{
		"no block in the storage matching the query time range": {
			finderResult: nil,
			expectedErr:  "",
		},
		"error while finding blocks matching the query time range": {
			finderErr:   errors.New("unable to find blocks"),
			expectedErr: "unable to find blocks",
		},
		"error while getting clients to query the store-gateway": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				errors.New("no client found"),
			},
			expectedErr: "no client found",
		},
		"a single store-gateway instance holds the required blocks": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr: "1.1.1.1",
						mockedLabelNamesResponse: &storepb.LabelNamesResponse{
							Names:    namesFromSeries(series1, series2),
							Warnings: []string{},
							Hints:    mockNamesHints(block1, block2),
						},
						mockedLabelValuesResponse: &storepb.LabelValuesResponse{
							Values:   valuesFromSeries(labels.MetricName, series1, series2),
							Warnings: []string{},
							Hints:    mockValuesHints(block1, block2),
						},
					}: {block1, block2},
				},
			},
			expectedLabelNames:  namesFromSeries(series1, series2),
			expectedLabelValues: valuesFromSeries(labels.MetricName, series1, series2),
		},
		"multiple store-gateway instances holds the required blocks without overlapping series": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr: "1.1.1.1",
						mockedLabelNamesResponse: &storepb.LabelNamesResponse{
							Names:    namesFromSeries(series1),
							Warnings: []string{},
							Hints:    mockNamesHints(block1),
						},
						mockedLabelValuesResponse: &storepb.LabelValuesResponse{
							Values:   valuesFromSeries(labels.MetricName, series1),
							Warnings: []string{},
							Hints:    mockValuesHints(block1),
						},
					}: {block1},
					&storeGatewayClientMock{
						remoteAddr: "2.2.2.2",
						mockedLabelNamesResponse: &storepb.LabelNamesResponse{
							Names:    namesFromSeries(series2),
							Warnings: []string{},
							Hints:    mockNamesHints(block2),
						},
						mockedLabelValuesResponse: &storepb.LabelValuesResponse{
							Values:   valuesFromSeries(labels.MetricName, series2),
							Warnings: []string{},
							Hints:    mockValuesHints(block2),
						},
					}: {block2},
				},
			},
			expectedLabelNames:  namesFromSeries(series1, series2),
			expectedLabelValues: valuesFromSeries(labels.MetricName, series1, series2),
		},
		"multiple store-gateway instances holds the required blocks with overlapping series (single returned series)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr: "1.1.1.1",
						mockedLabelNamesResponse: &storepb.LabelNamesResponse{
							Names:    namesFromSeries(series1),
							Warnings: []string{},
							Hints:    mockNamesHints(block1),
						},
						mockedLabelValuesResponse: &storepb.LabelValuesResponse{
							Values:   valuesFromSeries(labels.MetricName, series1),
							Warnings: []string{},
							Hints:    mockValuesHints(block1),
						},
					}: {block1},
					&storeGatewayClientMock{
						remoteAddr: "2.2.2.2",
						mockedLabelNamesResponse: &storepb.LabelNamesResponse{
							Names:    namesFromSeries(series1),
							Warnings: []string{},
							Hints:    mockNamesHints(block2),
						},
						mockedLabelValuesResponse: &storepb.LabelValuesResponse{
							Values:   valuesFromSeries(labels.MetricName, series1),
							Warnings: []string{},
							Hints:    mockValuesHints(block2),
						},
					}: {block2},
				},
			},
			expectedLabelNames:  namesFromSeries(series1),
			expectedLabelValues: valuesFromSeries(labels.MetricName, series1),
		},
		"multiple store-gateway instances holds the required blocks with overlapping series (multiple returned series)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			// Block1 has series1 and series2
			// Block2 has only series1
			// Block3 has only series2
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr: "1.1.1.1",
						mockedLabelNamesResponse: &storepb.LabelNamesResponse{
							Names:    namesFromSeries(series1, series2),
							Warnings: []string{},
							Hints:    mockNamesHints(block1),
						},
						mockedLabelValuesResponse: &storepb.LabelValuesResponse{
							Values:   valuesFromSeries(labels.MetricName, series1, series2),
							Warnings: []string{},
							Hints:    mockValuesHints(block1),
						},
					}: {block1},
					&storeGatewayClientMock{
						remoteAddr: "2.2.2.2",
						mockedLabelNamesResponse: &storepb.LabelNamesResponse{
							Names:    namesFromSeries(series1),
							Warnings: []string{},
							Hints:    mockNamesHints(block2),
						},
						mockedLabelValuesResponse: &storepb.LabelValuesResponse{
							Values:   valuesFromSeries(labels.MetricName, series1),
							Warnings: []string{},
							Hints:    mockValuesHints(block2),
						},
					}: {block2},
					&storeGatewayClientMock{
						remoteAddr: "3.3.3.3",
						mockedLabelNamesResponse: &storepb.LabelNamesResponse{
							Names:    namesFromSeries(series2),
							Warnings: []string{},
							Hints:    mockNamesHints(block3),
						},
						mockedLabelValuesResponse: &storepb.LabelValuesResponse{
							Values:   valuesFromSeries(labels.MetricName, series2),
							Warnings: []string{},
							Hints:    mockValuesHints(block3),
						},
					}: {block3},
				},
			},
			expectedLabelNames:  namesFromSeries(series1, series2),
			expectedLabelValues: valuesFromSeries(labels.MetricName, series1, series2),
			expectedMetrics: `
				# HELP cortex_querier_storegateway_instances_hit_per_query Number of store-gateway instances hit for a single query.
				# TYPE cortex_querier_storegateway_instances_hit_per_query histogram
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="0"} 0
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="1"} 0
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="2"} 0
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="3"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="4"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="5"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="6"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="7"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="8"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="9"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="10"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="+Inf"} 1
				cortex_querier_storegateway_instances_hit_per_query_sum 3
				cortex_querier_storegateway_instances_hit_per_query_count 1

				# HELP cortex_querier_storegateway_refetches_per_query Number of re-fetches attempted while querying store-gateway instances due to missing blocks.
				# TYPE cortex_querier_storegateway_refetches_per_query histogram
				cortex_querier_storegateway_refetches_per_query_bucket{le="0"} 1
				cortex_querier_storegateway_refetches_per_query_bucket{le="1"} 1
				cortex_querier_storegateway_refetches_per_query_bucket{le="2"} 1
				cortex_querier_storegateway_refetches_per_query_bucket{le="4"} 1
				cortex_querier_storegateway_refetches_per_query_bucket{le="8"} 1
				cortex_querier_storegateway_refetches_per_query_bucket{le="+Inf"} 1
				cortex_querier_storegateway_refetches_per_query_sum 0
				cortex_querier_storegateway_refetches_per_query_count 1
			`,
		},
		"multiple store-gateway instances holds the required blocks with overlapping series with limit (multiple returned series)": {
			limit: 2,
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			// Block1 has series1 and series2
			// Block2 has only series1
			// Block3 has only series2
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr: "1.1.1.1",
						mockedLabelNamesResponse: &storepb.LabelNamesResponse{
							Names:    namesFromSeries(series1, series2)[:2],
							Warnings: []string{},
							Hints:    mockNamesHints(block1),
						},
						mockedLabelValuesResponse: &storepb.LabelValuesResponse{
							Values:   valuesFromSeries(labels.MetricName, series1, series2)[:2],
							Warnings: []string{},
							Hints:    mockValuesHints(block1),
						},
					}: {block1},
					&storeGatewayClientMock{
						remoteAddr: "2.2.2.2",
						mockedLabelNamesResponse: &storepb.LabelNamesResponse{
							Names:    namesFromSeries(series1)[:2],
							Warnings: []string{},
							Hints:    mockNamesHints(block2),
						},
						mockedLabelValuesResponse: &storepb.LabelValuesResponse{
							Values:   valuesFromSeries(labels.MetricName, series1),
							Warnings: []string{},
							Hints:    mockValuesHints(block2),
						},
					}: {block2},
					&storeGatewayClientMock{
						remoteAddr: "3.3.3.3",
						mockedLabelNamesResponse: &storepb.LabelNamesResponse{
							Names:    namesFromSeries(series2)[:2],
							Warnings: []string{},
							Hints:    mockNamesHints(block3),
						},
						mockedLabelValuesResponse: &storepb.LabelValuesResponse{
							Values:   valuesFromSeries(labels.MetricName, series2),
							Warnings: []string{},
							Hints:    mockValuesHints(block3),
						},
					}: {block3},
				},
			},
			expectedLabelNames:  namesFromSeries(series1, series2),
			expectedLabelValues: valuesFromSeries(labels.MetricName, series1, series2),
		},
		"a single store-gateway instance has some missing blocks (consistency check failed)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
			},
			storeSetResponses: []interface{}{
				// First attempt returns a client whose response does not include all expected blocks.
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr: "1.1.1.1",
						mockedLabelNamesResponse: &storepb.LabelNamesResponse{
							Names:    namesFromSeries(series1),
							Warnings: []string{},
							Hints:    mockNamesHints(block1),
						},
						mockedLabelValuesResponse: &storepb.LabelValuesResponse{
							Values:   valuesFromSeries(labels.MetricName, series1),
							Warnings: []string{},
							Hints:    mockValuesHints(block1),
						},
					}: {block1},
				},
				// Second attempt returns an error because there are no other store-gateways left.
				errors.New("no store-gateway remaining after exclude"),
			},
			expectedErr: fmt.Sprintf("consistency check failed because some blocks were not queried: %s", block2.String()),
		},
		"multiple store-gateway instances have some missing blocks (consistency check failed)": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
				&bucketindex.Block{ID: block3},
				&bucketindex.Block{ID: block4},
			},
			storeSetResponses: []interface{}{
				// First attempt returns a client whose response does not include all expected blocks.
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr: "1.1.1.1",
						mockedLabelNamesResponse: &storepb.LabelNamesResponse{
							Names:    namesFromSeries(series1),
							Warnings: []string{},
							Hints:    mockNamesHints(block1),
						},
						mockedLabelValuesResponse: &storepb.LabelValuesResponse{
							Values:   valuesFromSeries(labels.MetricName, series1),
							Warnings: []string{},
							Hints:    mockValuesHints(block1),
						},
					}: {block1},
					&storeGatewayClientMock{
						remoteAddr: "2.2.2.2",
						mockedLabelNamesResponse: &storepb.LabelNamesResponse{
							Names:    namesFromSeries(series2),
							Warnings: []string{},
							Hints:    mockNamesHints(block2),
						},
						mockedLabelValuesResponse: &storepb.LabelValuesResponse{
							Values:   valuesFromSeries(labels.MetricName, series2),
							Warnings: []string{},
							Hints:    mockValuesHints(block2),
						},
					}: {block2},
				},
				// Second attempt returns an error because there are no other store-gateways left.
				errors.New("no store-gateway remaining after exclude"),
			},
			expectedErr: fmt.Sprintf("consistency check failed because some blocks were not queried: %s %s", block3.String(), block4.String()),
		},
		"multiple store-gateway instances have some missing blocks but queried from a replica during subsequent attempts": {
			// Block1 has series1
			// Block2 has series2
			// Block3 has series1 and series2
			// Block4 has no series (poor lonely block)
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
				&bucketindex.Block{ID: block2},
				&bucketindex.Block{ID: block3},
				&bucketindex.Block{ID: block4},
			},
			storeSetResponses: []interface{}{
				// First attempt returns a client whose response does not include all expected blocks.
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr: "1.1.1.1",
						mockedLabelNamesResponse: &storepb.LabelNamesResponse{
							Names:    namesFromSeries(series1),
							Warnings: []string{},
							Hints:    mockNamesHints(block1),
						},
						mockedLabelValuesResponse: &storepb.LabelValuesResponse{
							Values:   valuesFromSeries(labels.MetricName, series1),
							Warnings: []string{},
							Hints:    mockValuesHints(block1),
						},
					}: {block1, block3},
					&storeGatewayClientMock{
						remoteAddr: "2.2.2.2",
						mockedLabelNamesResponse: &storepb.LabelNamesResponse{
							Names:    namesFromSeries(series2),
							Warnings: []string{},
							Hints:    mockNamesHints(block2),
						},
						mockedLabelValuesResponse: &storepb.LabelValuesResponse{
							Values:   valuesFromSeries(labels.MetricName, series2),
							Warnings: []string{},
							Hints:    mockValuesHints(block2),
						},
					}: {block2, block4},
				},
				// Second attempt returns 1 missing block.
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr: "3.3.3.3",
						mockedLabelNamesResponse: &storepb.LabelNamesResponse{
							Names:    namesFromSeries(series1, series2),
							Warnings: []string{},
							Hints:    mockNamesHints(block3),
						},
						mockedLabelValuesResponse: &storepb.LabelValuesResponse{
							Values:   valuesFromSeries(labels.MetricName, series1, series2),
							Warnings: []string{},
							Hints:    mockValuesHints(block3),
						},
					}: {block3, block4},
				},
				// Third attempt returns the last missing block.
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr: "4.4.4.4",
						mockedLabelNamesResponse: &storepb.LabelNamesResponse{
							Names:    []string{},
							Warnings: []string{},
							Hints:    mockNamesHints(block4),
						},
						mockedLabelValuesResponse: &storepb.LabelValuesResponse{
							Values:   []string{},
							Warnings: []string{},
							Hints:    mockValuesHints(block4),
						},
					}: {block4},
				},
			},
			expectedLabelNames:  namesFromSeries(series1, series2),
			expectedLabelValues: valuesFromSeries(labels.MetricName, series1, series2),
			expectedMetrics: `
				# HELP cortex_querier_storegateway_instances_hit_per_query Number of store-gateway instances hit for a single query.
				# TYPE cortex_querier_storegateway_instances_hit_per_query histogram
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="0"} 0
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="1"} 0
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="2"} 0
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="3"} 0
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="4"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="5"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="6"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="7"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="8"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="9"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="10"} 1
				cortex_querier_storegateway_instances_hit_per_query_bucket{le="+Inf"} 1
				cortex_querier_storegateway_instances_hit_per_query_sum 4
				cortex_querier_storegateway_instances_hit_per_query_count 1

				# HELP cortex_querier_storegateway_refetches_per_query Number of re-fetches attempted while querying store-gateway instances due to missing blocks.
				# TYPE cortex_querier_storegateway_refetches_per_query histogram
				cortex_querier_storegateway_refetches_per_query_bucket{le="0"} 0
				cortex_querier_storegateway_refetches_per_query_bucket{le="1"} 0
				cortex_querier_storegateway_refetches_per_query_bucket{le="2"} 1
				cortex_querier_storegateway_refetches_per_query_bucket{le="4"} 1
				cortex_querier_storegateway_refetches_per_query_bucket{le="8"} 1
				cortex_querier_storegateway_refetches_per_query_bucket{le="+Inf"} 1
				cortex_querier_storegateway_refetches_per_query_sum 2
				cortex_querier_storegateway_refetches_per_query_count 1
			`,
		},
		"multiple store-gateways has the block, but one of them fails to return": {
			finderResult: bucketindex.Blocks{
				&bucketindex.Block{ID: block1},
			},
			storeSetResponses: []interface{}{
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr: "1.1.1.1",
						mockedLabelNamesResponse: &storepb.LabelNamesResponse{
							Names:    namesFromSeries(series1),
							Warnings: []string{},
							Hints:    mockNamesHints(block1),
						},
						mockedLabelValuesErr: status.Error(codes.Unavailable, "unavailable"),
					}: {block1},
				},
				map[BlocksStoreClient][]ulid.ULID{
					&storeGatewayClientMock{
						remoteAddr: "2.2.2.2",
						mockedLabelNamesResponse: &storepb.LabelNamesResponse{
							Names:    namesFromSeries(series1),
							Warnings: []string{},
							Hints:    mockNamesHints(block1),
						},
						mockedLabelValuesResponse: &storepb.LabelValuesResponse{
							Values:   valuesFromSeries(labels.MetricName, series1),
							Warnings: []string{},
							Hints:    mockValuesHints(block1),
						},
					}: {block1},
				},
			},
			expectedLabelNames:  namesFromSeries(series1),
			expectedLabelValues: valuesFromSeries(labels.MetricName, series1),
		},
	}

	for testName, testData := range tests {
		testData := testData
		var hints *storage.LabelHints
		if testData.limit > 0 {
			hints = &storage.LabelHints{
				Limit: testData.limit,
			}
		}

		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			// Splitting it because we need a new registry for names and values.
			// And also the initial expectedErr checking needs to be done for both.
			for _, testFunc := range []string{"LabelNames", "LabelValues"} {
				ctx := user.InjectOrgID(context.Background(), "user-1")
				reg := prometheus.NewPedanticRegistry()
				stores := &blocksStoreSetMock{mockedResponses: testData.storeSetResponses}
				finder := &blocksFinderMock{}
				finder.On("GetBlocks", mock.Anything, "user-1", minT, maxT).Return(testData.finderResult, map[ulid.ULID]*bucketindex.BlockDeletionMark(nil), testData.finderErr)

				q := &blocksStoreQuerier{
					minT:        minT,
					maxT:        maxT,
					finder:      finder,
					stores:      stores,
					consistency: NewBlocksConsistencyChecker(0, 0, log.NewNopLogger(), nil),
					logger:      log.NewNopLogger(),
					metrics:     newBlocksStoreQueryableMetrics(reg),
					limits:      &blocksStoreLimitsMock{},

					storeGatewayConsistencyCheckMaxAttempts: 3,
				}

				if testFunc == "LabelNames" {
					names, warnings, err := q.LabelNames(ctx, hints)
					if testData.expectedErr != "" {
						require.Equal(t, testData.expectedErr, err.Error())
						continue
					}

					require.NoError(t, err)
					require.Equal(t, 0, len(warnings))
					if testData.limit > 0 {
						testData.expectedLabelNames = testData.expectedLabelNames[:testData.limit]
					}
					require.Equal(t, testData.expectedLabelNames, names)

					// Assert on metrics (optional, only for test cases defining it).
					if testData.expectedMetrics != "" {
						assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(testData.expectedMetrics)))
					}
				}

				if testFunc == "LabelValues" {
					values, warnings, err := q.LabelValues(ctx, labels.MetricName, hints)
					if testData.expectedErr != "" {
						require.Equal(t, testData.expectedErr, err.Error())
						continue
					}

					if testData.limit > 0 {
						testData.expectedLabelValues = testData.expectedLabelValues[:testData.limit]
					}

					require.NoError(t, err)
					require.Equal(t, 0, len(warnings))
					require.Equal(t, testData.expectedLabelValues, values)

					// Assert on metrics (optional, only for test cases defining it).
					if testData.expectedMetrics != "" {
						assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(testData.expectedMetrics)))
					}
				}
			}
		})
	}
}

func TestBlocksStoreQuerier_SelectSortedShouldHonorQueryStoreAfter(t *testing.T) {

	now := time.Now()

	tests := map[string]struct {
		queryStoreAfter time.Duration
		queryMinT       int64
		queryMaxT       int64
		expectedMinT    int64
		expectedMaxT    int64
	}{
		"should not manipulate query time range if queryStoreAfter is disabled": {
			queryStoreAfter: 0,
			queryMinT:       util.TimeToMillis(now.Add(-100 * time.Minute)),
			queryMaxT:       util.TimeToMillis(now.Add(-30 * time.Minute)),
			expectedMinT:    util.TimeToMillis(now.Add(-100 * time.Minute)),
			expectedMaxT:    util.TimeToMillis(now.Add(-30 * time.Minute)),
		},
		"should not manipulate query time range if queryStoreAfter is enabled but query max time is older": {
			queryStoreAfter: time.Hour,
			queryMinT:       util.TimeToMillis(now.Add(-100 * time.Minute)),
			queryMaxT:       util.TimeToMillis(now.Add(-70 * time.Minute)),
			expectedMinT:    util.TimeToMillis(now.Add(-100 * time.Minute)),
			expectedMaxT:    util.TimeToMillis(now.Add(-70 * time.Minute)),
		},
		"should manipulate query time range if queryStoreAfter is enabled and query max time is recent": {
			queryStoreAfter: time.Hour,
			queryMinT:       util.TimeToMillis(now.Add(-100 * time.Minute)),
			queryMaxT:       util.TimeToMillis(now.Add(-30 * time.Minute)),
			expectedMinT:    util.TimeToMillis(now.Add(-100 * time.Minute)),
			expectedMaxT:    util.TimeToMillis(now.Add(-60 * time.Minute)),
		},
		"should skip the query if the query min time is more recent than queryStoreAfter": {
			queryStoreAfter: time.Hour,
			queryMinT:       util.TimeToMillis(now.Add(-50 * time.Minute)),
			queryMaxT:       util.TimeToMillis(now.Add(-20 * time.Minute)),
			expectedMinT:    0,
			expectedMaxT:    0,
		},
	}

	for testName, testData := range tests {
		testData := testData
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			ctx := user.InjectOrgID(context.Background(), "user-1")
			finder := &blocksFinderMock{}
			finder.On("GetBlocks", mock.Anything, "user-1", mock.Anything, mock.Anything).Return(bucketindex.Blocks(nil), map[ulid.ULID]*bucketindex.BlockDeletionMark(nil), error(nil))

			q := &blocksStoreQuerier{
				minT:            testData.queryMinT,
				maxT:            testData.queryMaxT,
				finder:          finder,
				stores:          &blocksStoreSetMock{},
				consistency:     NewBlocksConsistencyChecker(0, 0, log.NewNopLogger(), nil),
				logger:          log.NewNopLogger(),
				metrics:         newBlocksStoreQueryableMetrics(nil),
				limits:          &blocksStoreLimitsMock{},
				queryStoreAfter: testData.queryStoreAfter,
			}

			sp := &storage.SelectHints{
				Start: testData.queryMinT,
				End:   testData.queryMaxT,
			}

			set := q.selectSorted(ctx, sp)
			require.NoError(t, set.Err())

			if testData.expectedMinT == 0 && testData.expectedMaxT == 0 {
				assert.Len(t, finder.Calls, 0)
			} else {
				require.Len(t, finder.Calls, 1)
				assert.Equal(t, testData.expectedMinT, finder.Calls[0].Arguments.Get(2))
				assert.InDelta(t, testData.expectedMaxT, finder.Calls[0].Arguments.Get(3), float64(5*time.Second.Milliseconds()))
			}
		})
	}
}

func TestBlocksStoreQuerier_PromQLExecution(t *testing.T) {
	t.Parallel()
	logger := log.NewNopLogger()
	opts := promql.EngineOpts{
		Logger:     util_log.GoKitLogToSlog(logger),
		Timeout:    10 * time.Second,
		MaxSamples: 1e6,
	}

	block1 := ulid.MustNew(1, nil)
	block2 := ulid.MustNew(2, nil)
	series1 := []labelpb.ZLabel{{Name: "__name__", Value: "metric_1"}}
	series2 := []labelpb.ZLabel{{Name: "__name__", Value: "metric_2"}}

	from := model.Time(1589759955000)

	for _, thanosEngine := range []bool{false, true} {
		for _, enc := range encodings {
			t.Run(fmt.Sprintf("thanos engine enabled=%t, enc=%s", thanosEngine, enc.String()), func(t *testing.T) {
				var queryEngine promql.QueryEngine
				if thanosEngine {
					queryEngine = engine.New(engine.Opts{
						EngineOpts:        opts,
						LogicalOptimizers: logicalplan.AllOptimizers,
					})
				}
				queryEngine = promql.NewEngine(opts)
				// Mock the finder to simulate we need to query two blocks.
				finder := &blocksFinderMock{
					Service: services.NewIdleService(nil, nil),
				}
				finder.On("GetBlocks", mock.Anything, "user-1", mock.Anything, mock.Anything).Return(bucketindex.Blocks{
					&bucketindex.Block{ID: block1},
					&bucketindex.Block{ID: block2},
				}, map[ulid.ULID]*bucketindex.BlockDeletionMark(nil), error(nil))

				// Mock the store to simulate each block is queried from a different store-gateway.
				gateway1 := &storeGatewayClientMock{remoteAddr: "1.1.1.1", mockedSeriesResponses: []*storepb.SeriesResponse{
					{
						Result: &storepb.SeriesResponse_Series{
							Series: &storepb.Series{
								Labels: series1,
								Chunks: []storepb.AggrChunk{
									createAggrChunk(t, 15*time.Second, from, 3, enc),
								},
							},
						},
					},
					{
						Result: &storepb.SeriesResponse_Series{
							Series: &storepb.Series{
								Labels: series2,
								Chunks: []storepb.AggrChunk{
									createAggrChunk(t, 15*time.Second, from, 3, enc),
								},
							},
						},
					},
					mockHintsResponse(block1),
				}}

				gateway2 := &storeGatewayClientMock{remoteAddr: "2.2.2.2", mockedSeriesResponses: []*storepb.SeriesResponse{
					{
						Result: &storepb.SeriesResponse_Series{
							Series: &storepb.Series{
								Labels: series1,
								Chunks: []storepb.AggrChunk{
									createAggrChunk(t, 15*time.Second, from+45000, 3, enc),
								},
							},
						},
					}, {
						Result: &storepb.SeriesResponse_Series{
							Series: &storepb.Series{
								Labels: series2,
								Chunks: []storepb.AggrChunk{
									createAggrChunk(t, 15*time.Second, from+45000, 3, enc),
								},
							},
						},
					},
					mockHintsResponse(block2),
				}}

				stores := &blocksStoreSetMock{
					Service: services.NewIdleService(nil, nil),
					mockedResponses: []interface{}{
						map[BlocksStoreClient][]ulid.ULID{
							gateway1: {block1},
							gateway2: {block2},
						},
					},
				}

				// Instance the querier that will be executed to run the query.
				cfg := Config{
					QueryStoreAfter:                         0,
					StoreGatewayQueryStatsEnabled:           false,
					StoreGatewayConsistencyCheckMaxAttempts: 3,
				}
				queryable, err := NewBlocksStoreQueryable(stores, finder, NewBlocksConsistencyChecker(0, 0, logger, nil), &blocksStoreLimitsMock{}, cfg, logger, nil)
				require.NoError(t, err)
				require.NoError(t, services.StartAndAwaitRunning(context.Background(), queryable))
				defer services.StopAndAwaitTerminated(context.Background(), queryable) // nolint:errcheck

				// Run a query.
				ctx := user.InjectOrgID(context.Background(), "user-1")
				q, err := queryEngine.NewRangeQuery(ctx, queryable, nil, `{__name__=~"metric.*"}`, time.Unix(1589759955, 0), time.Unix(1589760030, 0), 15*time.Second)
				require.NoError(t, err)

				res := q.Exec(ctx)
				require.NoError(t, err)
				require.NoError(t, res.Err)

				matrix, err := res.Matrix()
				require.NoError(t, err)
				require.Len(t, matrix, 2)

				require.Equal(t, labelpb.ZLabelsToPromLabels(series1), matrix[0].Metric)
				require.Equal(t, labelpb.ZLabelsToPromLabels(series2), matrix[1].Metric)
				for _, m := range matrix {
					for i, f := range m.Floats {
						// Check sample timestamp is expected.
						require.Equal(t, f.T, int64(from)+int64(i)*15000)
						require.Equal(t, f.T, int64(f.F))
					}
					for i, h := range m.Histograms {
						h := h
						// Check sample timestamp is expected.
						require.Equal(t, h.T, int64(from)+int64(i)*15000)
						expectedH := tsdbutil.GenerateTestGaugeFloatHistogram(h.T)
						switch enc {
						case encoding.PrometheusHistogramChunk:
							require.Equal(t, expectedH, h.H)
						case encoding.PrometheusFloatHistogramChunk:
							require.Equal(t, expectedH, h.H)
						}
					}
				}
			})
		}
	}
}

type blocksStoreSetMock struct {
	services.Service

	mockedResponses []interface{}
	nextResult      int
	queriedBlocks   []ulid.ULID
}

func (m *blocksStoreSetMock) GetClientsFor(_ string, b []ulid.ULID, _ map[ulid.ULID][]string, _ map[ulid.ULID]map[string]int) (map[BlocksStoreClient][]ulid.ULID, error) {
	if m.nextResult >= len(m.mockedResponses) {
		panic("not enough mocked results")
	}
	m.queriedBlocks = append(m.queriedBlocks, b...)

	res := m.mockedResponses[m.nextResult]
	m.nextResult++

	if err, ok := res.(error); ok {
		return nil, err
	}
	if clients, ok := res.(map[BlocksStoreClient][]ulid.ULID); ok {
		return clients, nil
	}

	return nil, errors.New("unknown data type in the mocked result")
}

func (m *blocksStoreSetMock) Reset() {
	m.nextResult = 0
	m.queriedBlocks = nil
}

type blocksFinderMock struct {
	services.Service
	mock.Mock
}

func (m *blocksFinderMock) GetBlocks(ctx context.Context, userID string, minT, maxT int64) (bucketindex.Blocks, map[ulid.ULID]*bucketindex.BlockDeletionMark, error) {
	args := m.Called(ctx, userID, minT, maxT)
	return args.Get(0).(bucketindex.Blocks), args.Get(1).(map[ulid.ULID]*bucketindex.BlockDeletionMark), args.Error(2)
}

type storeGatewayClientMock struct {
	remoteAddr                string
	mockedSeriesResponses     []*storepb.SeriesResponse
	mockedSeriesErr           error
	mockedSeriesStreamErr     error
	mockedLabelNamesResponse  *storepb.LabelNamesResponse
	mockedLabelValuesResponse *storepb.LabelValuesResponse
	mockedLabelValuesErr      error
}

func (m *storeGatewayClientMock) Series(ctx context.Context, in *storepb.SeriesRequest, opts ...grpc.CallOption) (storegatewaypb.StoreGateway_SeriesClient, error) {
	seriesClient := &storeGatewaySeriesClientMock{
		limit:                 in.Limit,
		mockedResponses:       m.mockedSeriesResponses,
		mockedSeriesStreamErr: m.mockedSeriesStreamErr,
	}

	return seriesClient, m.mockedSeriesErr
}

func (m *storeGatewayClientMock) LabelNames(_ context.Context, r *storepb.LabelNamesRequest, _ ...grpc.CallOption) (*storepb.LabelNamesResponse, error) {
	if r.Limit > 0 && len(m.mockedLabelNamesResponse.Names) > int(r.Limit) {
		m.mockedLabelNamesResponse.Names = m.mockedLabelNamesResponse.Names[:r.Limit]
	}
	return m.mockedLabelNamesResponse, nil
}

func (m *storeGatewayClientMock) LabelValues(_ context.Context, r *storepb.LabelValuesRequest, _ ...grpc.CallOption) (*storepb.LabelValuesResponse, error) {
	if r.Limit > 0 && len(m.mockedLabelValuesResponse.Values) > int(r.Limit) {
		m.mockedLabelNamesResponse.Names = m.mockedLabelValuesResponse.Values[:r.Limit]
	}
	return m.mockedLabelValuesResponse, m.mockedLabelValuesErr
}

func (m *storeGatewayClientMock) RemoteAddress() string {
	return m.remoteAddr
}

type storeGatewaySeriesClientMock struct {
	grpc.ClientStream

	limit                 int64
	mockedResponses       []*storepb.SeriesResponse
	mockedSeriesStreamErr error
}

func (m *storeGatewaySeriesClientMock) Recv() (*storepb.SeriesResponse, error) {
	// Ensure some concurrency occurs.
	time.Sleep(10 * time.Millisecond)

	if len(m.mockedResponses) == 0 {
		return nil, io.EOF
	}

	res := m.mockedResponses[0]
	m.mockedResponses = m.mockedResponses[1:]
	return res, m.mockedSeriesStreamErr
}

type blocksStoreLimitsMock struct {
	maxChunksPerQuery           int
	storeGatewayTenantShardSize float64
}

func (m *blocksStoreLimitsMock) MaxChunksPerQueryFromStore(_ string) int {
	return m.maxChunksPerQuery
}

func (m *blocksStoreLimitsMock) StoreGatewayTenantShardSize(_ string) float64 {
	return m.storeGatewayTenantShardSize
}

func (m *blocksStoreLimitsMock) S3SSEType(_ string) string {
	return ""
}

func (m *blocksStoreLimitsMock) S3SSEKMSKeyID(_ string) string {
	return ""
}

func (m *blocksStoreLimitsMock) S3SSEKMSEncryptionContext(_ string) string {
	return ""
}

func mockSeriesResponse(lbls labels.Labels, samples []cortexpb.Sample, histograms []cortexpb.Histogram, floatHistograms []cortexpb.Histogram) *storepb.SeriesResponse {
	res := &storepb.SeriesResponse_Series{
		Series: &storepb.Series{
			Labels: labelpb.ZLabelsFromPromLabels(lbls),
			Chunks: []storepb.AggrChunk{},
		},
	}
	// For each sample, generate a chunk containing a single value for simplicity.
	for _, sample := range samples {
		chunk := chunkenc.NewXORChunk()
		appender, err := chunk.Appender()
		if err != nil {
			panic(err)
		}

		appender.Append(sample.TimestampMs, sample.Value)
		chunkData := chunk.Bytes()
		res.Series.Chunks = append(res.Series.Chunks, storepb.AggrChunk{
			MinTime: sample.TimestampMs,
			MaxTime: sample.TimestampMs,
			Raw:     &storepb.Chunk{Type: storepb.Chunk_XOR, Data: chunkData},
		})
	}

	for _, h := range histograms {
		chunk := chunkenc.NewHistogramChunk()
		appender, err := chunk.Appender()
		if err != nil {
			panic(err)
		}
		_, _, _, err = appender.AppendHistogram(nil, h.TimestampMs, cortexpb.HistogramProtoToHistogram(h), true)
		if err != nil {
			panic(err)
		}
		chunkData := chunk.Bytes()
		res.Series.Chunks = append(res.Series.Chunks, storepb.AggrChunk{
			MinTime: h.TimestampMs,
			MaxTime: h.TimestampMs,
			Raw:     &storepb.Chunk{Type: storepb.Chunk_HISTOGRAM, Data: chunkData},
		})
	}

	for _, fh := range floatHistograms {
		chunk := chunkenc.NewFloatHistogramChunk()
		appender, err := chunk.Appender()
		if err != nil {
			panic(err)
		}
		_, _, _, err = appender.AppendFloatHistogram(nil, fh.TimestampMs, cortexpb.FloatHistogramProtoToFloatHistogram(fh), true)
		if err != nil {
			panic(err)
		}
		chunkData := chunk.Bytes()
		res.Series.Chunks = append(res.Series.Chunks, storepb.AggrChunk{
			MinTime: fh.TimestampMs,
			MaxTime: fh.TimestampMs,
			Raw:     &storepb.Chunk{Type: storepb.Chunk_FLOAT_HISTOGRAM, Data: chunkData},
		})
	}

	return &storepb.SeriesResponse{Result: res}
}

func mockHintsResponse(ids ...ulid.ULID) *storepb.SeriesResponse {
	hints := &hintspb.SeriesResponseHints{}
	for _, id := range ids {
		hints.AddQueriedBlock(id)
	}

	any, err := types.MarshalAny(hints)
	if err != nil {
		panic(err)
	}

	return &storepb.SeriesResponse{
		Result: &storepb.SeriesResponse_Hints{
			Hints: any,
		},
	}
}

func mockNamesHints(ids ...ulid.ULID) *types.Any {
	hints := &hintspb.LabelNamesResponseHints{}
	for _, id := range ids {
		hints.AddQueriedBlock(id)
	}

	any, err := types.MarshalAny(hints)
	if err != nil {
		panic(err)
	}

	return any
}

func mockValuesHints(ids ...ulid.ULID) *types.Any {
	hints := &hintspb.LabelValuesResponseHints{}
	for _, id := range ids {
		hints.AddQueriedBlock(id)
	}

	any, err := types.MarshalAny(hints)
	if err != nil {
		panic(err)
	}

	return any
}

func namesFromSeries(series ...labels.Labels) []string {
	namesMap := map[string]struct{}{}
	for _, s := range series {
		s.Range(func(l labels.Label) {
			namesMap[l.Name] = struct{}{}
		})
	}

	names := []string{}
	for name := range namesMap {
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

func valuesFromSeries(name string, series ...labels.Labels) []string {
	valuesMap := map[string]struct{}{}
	for _, s := range series {
		s.Range(func(l labels.Label) {
			if l.Name == name {
				valuesMap[l.Value] = struct{}{}
			}
		})
	}

	values := []string{}
	for name := range valuesMap {
		values = append(values, name)
	}

	sort.Strings(values)
	return values
}

func TestCountSamplesAndChunks(t *testing.T) {
	floatChk := util.GenerateChunk(t, time.Second, model.Time(0), 100, encoding.PrometheusXorChunk)
	histogramChk := util.GenerateChunk(t, time.Second, model.Time(0), 300, encoding.PrometheusHistogramChunk)
	floatHistogramChk := util.GenerateChunk(t, time.Second, model.Time(0), 500, encoding.PrometheusFloatHistogramChunk)

	for i, tc := range []struct {
		serieses        []*storepb.Series
		expectedChunks  uint64
		expectedSamples uint64
	}{
		{
			serieses: []*storepb.Series{
				{
					Chunks: []storepb.AggrChunk{
						{
							Raw: &storepb.Chunk{
								Type: storepb.Chunk_XOR,
								Data: floatChk.Data.Bytes(),
							},
						},
					},
				},
			},
			expectedSamples: uint64(100),
			expectedChunks:  1,
		},
		{
			serieses: []*storepb.Series{
				{
					Chunks: []storepb.AggrChunk{
						{
							Raw: &storepb.Chunk{
								Type: storepb.Chunk_XOR,
								Data: floatChk.Data.Bytes(),
							},
						},
						{
							Raw: &storepb.Chunk{
								Type: storepb.Chunk_HISTOGRAM,
								Data: histogramChk.Data.Bytes(),
							},
						},
					},
				},
			},
			expectedSamples: 400,
			expectedChunks:  2,
		},
		{
			serieses: []*storepb.Series{
				{
					Chunks: []storepb.AggrChunk{
						{
							Raw: &storepb.Chunk{
								Type: storepb.Chunk_XOR,
								Data: floatChk.Data.Bytes(),
							},
						},
						{
							Raw: &storepb.Chunk{
								Type: storepb.Chunk_HISTOGRAM,
								Data: histogramChk.Data.Bytes(),
							},
						},
						{
							Raw: &storepb.Chunk{
								Type: storepb.Chunk_FLOAT_HISTOGRAM,
								Data: floatHistogramChk.Data.Bytes(),
							},
						},
					},
				},
			},
			expectedSamples: 900,
			expectedChunks:  3,
		},
	} {
		t.Run(fmt.Sprintf("test_case_%d", i), func(t *testing.T) {
			samples, chunks := countSamplesAndChunks(tc.serieses...)
			require.Equal(t, tc.expectedSamples, samples)
			require.Equal(t, tc.expectedChunks, chunks)
		})
	}
}

func createAggrChunk(t *testing.T, step time.Duration, from model.Time, points int, enc encoding.Encoding) storepb.AggrChunk {
	c := util.GenerateChunk(t, step, from, points, enc)
	chunkType := storepb.Chunk_XOR
	switch enc {
	case encoding.PrometheusHistogramChunk:
		chunkType = storepb.Chunk_HISTOGRAM
	case encoding.PrometheusFloatHistogramChunk:
		chunkType = storepb.Chunk_FLOAT_HISTOGRAM
	default:
	}
	return storepb.AggrChunk{
		MinTime: int64(c.From),
		MaxTime: int64(c.Through),
		Raw: &storepb.Chunk{
			Type: chunkType,
			Data: c.Data.Bytes(),
		},
	}
}
