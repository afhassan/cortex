package distributor

import (
	"context"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	storecache "github.com/thanos-io/thanos/pkg/store/cache"
	"github.com/weaveworks/common/httpgrpc"
	"github.com/weaveworks/common/user"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	promchunk "github.com/cortexproject/cortex/pkg/chunk/encoding"
	_ "github.com/cortexproject/cortex/pkg/cortex/configinit"
	"github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/cortexproject/cortex/pkg/ha"
	"github.com/cortexproject/cortex/pkg/ingester"
	"github.com/cortexproject/cortex/pkg/ingester/client"
	"github.com/cortexproject/cortex/pkg/ring"
	ring_client "github.com/cortexproject/cortex/pkg/ring/client"
	"github.com/cortexproject/cortex/pkg/ring/kv"
	"github.com/cortexproject/cortex/pkg/ring/kv/consul"
	"github.com/cortexproject/cortex/pkg/tenant"
	"github.com/cortexproject/cortex/pkg/util"
	"github.com/cortexproject/cortex/pkg/util/chunkcompat"
	"github.com/cortexproject/cortex/pkg/util/flagext"
	"github.com/cortexproject/cortex/pkg/util/limiter"
	"github.com/cortexproject/cortex/pkg/util/services"
	"github.com/cortexproject/cortex/pkg/util/test"
	"github.com/cortexproject/cortex/pkg/util/validation"
)

var (
	errFail       = httpgrpc.Errorf(http.StatusInternalServerError, "Fail")
	emptyResponse = &cortexpb.WriteResponse{}
)

var (
	randomStrings = []string{}
)

func init() {
	randomStrings = util.GenerateRandomStrings()
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		initConfig func(*Config)
		initLimits func(*validation.Limits)
		expected   error
	}{
		"default config should pass": {
			initConfig: func(_ *Config) {},
			initLimits: func(_ *validation.Limits) {},
			expected:   nil,
		},
		"should fail on invalid sharding strategy": {
			initConfig: func(cfg *Config) {
				cfg.ShardingStrategy = "xxx"
			},
			initLimits: func(_ *validation.Limits) {},
			expected:   errInvalidShardingStrategy,
		},
		"should pass sharding strategy when IngestionTenantShardSize = 0": {
			initConfig: func(cfg *Config) {
				cfg.ShardingStrategy = "shuffle-sharding"
			},
			initLimits: func(limits *validation.Limits) {
				limits.IngestionTenantShardSize = 0
			},
			expected: nil,
		},
		"should pass if the default shard size > 0 on when sharding strategy = shuffle-sharding": {
			initConfig: func(cfg *Config) {
				cfg.ShardingStrategy = "shuffle-sharding"
			},
			initLimits: func(limits *validation.Limits) {
				limits.IngestionTenantShardSize = 3
			},
			expected: nil,
		},
		"should fail because the ingestionTenantShardSize is a non-positive number": {
			initConfig: func(cfg *Config) {
				cfg.ShardingStrategy = "shuffle-sharding"
			},
			initLimits: func(limits *validation.Limits) {
				limits.IngestionTenantShardSize = -1
			},
			expected: errInvalidTenantShardSize,
		},
	}

	for testName, testData := range tests {
		testData := testData // Needed for t.Parallel to work correctly
		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			cfg := Config{}
			limits := validation.Limits{}
			flagext.DefaultValues(&cfg, &limits)

			testData.initConfig(&cfg)
			testData.initLimits(&limits)

			assert.Equal(t, testData.expected, cfg.Validate(limits))
		})
	}
}

func TestDistributor_Push(t *testing.T) {
	t.Parallel()
	// Metrics to assert on.
	lastSeenTimestamp := "cortex_distributor_latest_seen_sample_timestamp_seconds"
	distributorAppend := "cortex_distributor_ingester_appends_total"
	distributorAppendFailure := "cortex_distributor_ingester_append_failures_total"
	distributorReceivedSamples := "cortex_distributor_received_samples_total"
	ctx := user.InjectOrgID(context.Background(), "userDistributorPush")

	type samplesIn struct {
		num              int
		startTimestampMs int64
	}
	for name, tc := range map[string]struct {
		metricNames      []string
		numIngesters     int
		happyIngesters   int
		samples          samplesIn
		histogramSamples bool
		metadata         int
		expectedResponse *cortexpb.WriteResponse
		expectedError    error
		expectedMetrics  string
		ingesterError    error
	}{
		"A push of no samples shouldn't block or return error, even if ingesters are sad": {
			numIngesters:     3,
			happyIngesters:   0,
			expectedResponse: emptyResponse,
		},
		"A push to 3 happy ingesters should succeed": {
			numIngesters:     3,
			happyIngesters:   3,
			samples:          samplesIn{num: 5, startTimestampMs: 123456789000},
			metadata:         5,
			expectedResponse: emptyResponse,
			metricNames:      []string{lastSeenTimestamp},
			expectedMetrics: `
				# HELP cortex_distributor_latest_seen_sample_timestamp_seconds Unix timestamp of latest received sample per user.
				# TYPE cortex_distributor_latest_seen_sample_timestamp_seconds gauge
				cortex_distributor_latest_seen_sample_timestamp_seconds{user="userDistributorPush"} 123456789.004
			`,
		},
		"A push to 2 happy ingesters should succeed": {
			numIngesters:     3,
			happyIngesters:   2,
			samples:          samplesIn{num: 5, startTimestampMs: 123456789000},
			metadata:         5,
			expectedResponse: emptyResponse,
			metricNames:      []string{lastSeenTimestamp},
			expectedMetrics: `
				# HELP cortex_distributor_latest_seen_sample_timestamp_seconds Unix timestamp of latest received sample per user.
				# TYPE cortex_distributor_latest_seen_sample_timestamp_seconds gauge
				cortex_distributor_latest_seen_sample_timestamp_seconds{user="userDistributorPush"} 123456789.004
			`,
		},
		"A push to 1 happy ingesters should fail": {
			numIngesters:   3,
			happyIngesters: 1,
			samples:        samplesIn{num: 10, startTimestampMs: 123456789000},
			expectedError:  errFail,
			metricNames:    []string{lastSeenTimestamp},
			expectedMetrics: `
				# HELP cortex_distributor_latest_seen_sample_timestamp_seconds Unix timestamp of latest received sample per user.
				# TYPE cortex_distributor_latest_seen_sample_timestamp_seconds gauge
				cortex_distributor_latest_seen_sample_timestamp_seconds{user="userDistributorPush"} 123456789.009
			`,
		},
		"A push to 0 happy ingesters should fail": {
			numIngesters:   3,
			happyIngesters: 0,
			samples:        samplesIn{num: 10, startTimestampMs: 123456789000},
			expectedError:  errFail,
			metricNames:    []string{lastSeenTimestamp},
			expectedMetrics: `
				# HELP cortex_distributor_latest_seen_sample_timestamp_seconds Unix timestamp of latest received sample per user.
				# TYPE cortex_distributor_latest_seen_sample_timestamp_seconds gauge
				cortex_distributor_latest_seen_sample_timestamp_seconds{user="userDistributorPush"} 123456789.009
			`,
		},
		"A push exceeding burst size should fail": {
			numIngesters:   3,
			happyIngesters: 3,
			samples:        samplesIn{num: 25, startTimestampMs: 123456789000},
			metadata:       5,
			expectedError:  httpgrpc.Errorf(http.StatusTooManyRequests, "ingestion rate limit (20) exceeded while adding 25 samples and 5 metadata"),
			metricNames:    []string{lastSeenTimestamp},
			expectedMetrics: `
				# HELP cortex_distributor_latest_seen_sample_timestamp_seconds Unix timestamp of latest received sample per user.
				# TYPE cortex_distributor_latest_seen_sample_timestamp_seconds gauge
				cortex_distributor_latest_seen_sample_timestamp_seconds{user="userDistributorPush"} 123456789.024
			`,
		},
		"A push to ingesters should report the correct metrics with no metadata": {
			numIngesters:     3,
			happyIngesters:   2,
			samples:          samplesIn{num: 1, startTimestampMs: 123456789000},
			metadata:         0,
			metricNames:      []string{distributorAppend, distributorAppendFailure},
			expectedResponse: emptyResponse,
			expectedMetrics: `
				# HELP cortex_distributor_ingester_append_failures_total The total number of failed batch appends sent to ingesters.
				# TYPE cortex_distributor_ingester_append_failures_total counter
				cortex_distributor_ingester_append_failures_total{ingester="ingester-2",status="5xx",type="samples"} 1
				# HELP cortex_distributor_ingester_appends_total The total number of batch appends sent to ingesters.
				# TYPE cortex_distributor_ingester_appends_total counter
				cortex_distributor_ingester_appends_total{ingester="ingester-0",type="samples"} 1
				cortex_distributor_ingester_appends_total{ingester="ingester-1",type="samples"} 1
				cortex_distributor_ingester_appends_total{ingester="ingester-2",type="samples"} 1
			`,
		},
		"A push to ingesters should report the correct metrics with no samples": {
			numIngesters:     3,
			happyIngesters:   2,
			samples:          samplesIn{num: 0, startTimestampMs: 123456789000},
			metadata:         1,
			metricNames:      []string{distributorAppend, distributorAppendFailure},
			expectedResponse: emptyResponse,
			ingesterError:    httpgrpc.Errorf(http.StatusInternalServerError, "Fail"),
			expectedMetrics: `
				# HELP cortex_distributor_ingester_append_failures_total The total number of failed batch appends sent to ingesters.
				# TYPE cortex_distributor_ingester_append_failures_total counter
				cortex_distributor_ingester_append_failures_total{ingester="ingester-2",status="5xx",type="metadata"} 1
				# HELP cortex_distributor_ingester_appends_total The total number of batch appends sent to ingesters.
				# TYPE cortex_distributor_ingester_appends_total counter
				cortex_distributor_ingester_appends_total{ingester="ingester-0",type="metadata"} 1
				cortex_distributor_ingester_appends_total{ingester="ingester-1",type="metadata"} 1
				cortex_distributor_ingester_appends_total{ingester="ingester-2",type="metadata"} 1
			`,
		},
		"A push to overloaded ingesters should report the correct metrics": {
			numIngesters:     3,
			happyIngesters:   2,
			samples:          samplesIn{num: 0, startTimestampMs: 123456789000},
			metadata:         1,
			metricNames:      []string{distributorAppend, distributorAppendFailure},
			expectedResponse: emptyResponse,
			ingesterError:    httpgrpc.Errorf(http.StatusTooManyRequests, "Fail"),
			expectedMetrics: `
				# HELP cortex_distributor_ingester_appends_total The total number of batch appends sent to ingesters.
				# TYPE cortex_distributor_ingester_appends_total counter
				cortex_distributor_ingester_appends_total{ingester="ingester-0",type="metadata"} 1
				cortex_distributor_ingester_appends_total{ingester="ingester-1",type="metadata"} 1
				cortex_distributor_ingester_appends_total{ingester="ingester-2",type="metadata"} 1
				# HELP cortex_distributor_ingester_append_failures_total The total number of failed batch appends sent to ingesters.
				# TYPE cortex_distributor_ingester_append_failures_total counter
				cortex_distributor_ingester_append_failures_total{ingester="ingester-2",status="4xx",type="metadata"} 1
			`,
		},
		"A push to 3 happy ingesters should succeed, histograms": {
			numIngesters:     3,
			happyIngesters:   3,
			samples:          samplesIn{num: 5, startTimestampMs: 123456789000},
			histogramSamples: true,
			metadata:         5,
			expectedResponse: emptyResponse,
			metricNames:      []string{lastSeenTimestamp, distributorReceivedSamples},
			expectedMetrics: `
				# HELP cortex_distributor_latest_seen_sample_timestamp_seconds Unix timestamp of latest received sample per user.
				# TYPE cortex_distributor_latest_seen_sample_timestamp_seconds gauge
				cortex_distributor_latest_seen_sample_timestamp_seconds{user="userDistributorPush"} 123456789.004
				# HELP cortex_distributor_received_samples_total The total number of received samples, excluding rejected and deduped samples.
				# TYPE cortex_distributor_received_samples_total counter
				cortex_distributor_received_samples_total{type="float",user="userDistributorPush"} 0
				cortex_distributor_received_samples_total{type="histogram",user="userDistributorPush"} 5
			`,
		},
		"A push to 2 happy ingesters should succeed, histograms": {
			numIngesters:     3,
			happyIngesters:   2,
			samples:          samplesIn{num: 5, startTimestampMs: 123456789000},
			histogramSamples: true,
			metadata:         5,
			expectedResponse: emptyResponse,
			metricNames:      []string{lastSeenTimestamp, distributorReceivedSamples},
			expectedMetrics: `
				# HELP cortex_distributor_latest_seen_sample_timestamp_seconds Unix timestamp of latest received sample per user.
				# TYPE cortex_distributor_latest_seen_sample_timestamp_seconds gauge
				cortex_distributor_latest_seen_sample_timestamp_seconds{user="userDistributorPush"} 123456789.004
				# HELP cortex_distributor_received_samples_total The total number of received samples, excluding rejected and deduped samples.
				# TYPE cortex_distributor_received_samples_total counter
				cortex_distributor_received_samples_total{type="float",user="userDistributorPush"} 0
				cortex_distributor_received_samples_total{type="histogram",user="userDistributorPush"} 5
			`,
		},
		"A push to 1 happy ingesters should fail, histograms": {
			numIngesters:     3,
			happyIngesters:   1,
			samples:          samplesIn{num: 10, startTimestampMs: 123456789000},
			histogramSamples: true,
			expectedError:    errFail,
			metricNames:      []string{lastSeenTimestamp, distributorReceivedSamples},
			expectedMetrics: `
				# HELP cortex_distributor_latest_seen_sample_timestamp_seconds Unix timestamp of latest received sample per user.
				# TYPE cortex_distributor_latest_seen_sample_timestamp_seconds gauge
				cortex_distributor_latest_seen_sample_timestamp_seconds{user="userDistributorPush"} 123456789.009
				# HELP cortex_distributor_received_samples_total The total number of received samples, excluding rejected and deduped samples.
				# TYPE cortex_distributor_received_samples_total counter
				cortex_distributor_received_samples_total{type="float",user="userDistributorPush"} 0
				cortex_distributor_received_samples_total{type="histogram",user="userDistributorPush"} 10
			`,
		},
		"A push exceeding burst size should fail, histograms": {
			numIngesters:     3,
			happyIngesters:   3,
			samples:          samplesIn{num: 25, startTimestampMs: 123456789000},
			histogramSamples: true,
			metadata:         5,
			expectedError:    httpgrpc.Errorf(http.StatusTooManyRequests, "ingestion rate limit (20) exceeded while adding 25 samples and 5 metadata"),
			metricNames:      []string{lastSeenTimestamp, distributorReceivedSamples},
			expectedMetrics: `
				# HELP cortex_distributor_latest_seen_sample_timestamp_seconds Unix timestamp of latest received sample per user.
				# TYPE cortex_distributor_latest_seen_sample_timestamp_seconds gauge
				cortex_distributor_latest_seen_sample_timestamp_seconds{user="userDistributorPush"} 123456789.024
				# HELP cortex_distributor_received_samples_total The total number of received samples, excluding rejected and deduped samples.
				# TYPE cortex_distributor_received_samples_total counter
				cortex_distributor_received_samples_total{type="float",user="userDistributorPush"} 0
				cortex_distributor_received_samples_total{type="histogram",user="userDistributorPush"} 25
			`,
		},
	} {
		for _, useStreamPush := range []bool{false, true} {
			for _, shardByAllLabels := range []bool{true, false} {
				tc := tc
				name := name
				shardByAllLabels := shardByAllLabels
				useStreamPush := useStreamPush
				t.Run(fmt.Sprintf("[%s](shardByAllLabels=%v,useStreamPush=%v)", name, shardByAllLabels, useStreamPush), func(t *testing.T) {
					t.Parallel()
					limits := &validation.Limits{}
					flagext.DefaultValues(limits)
					limits.IngestionRate = 20
					limits.IngestionBurstSize = 20

					ds, _, regs, _ := prepare(t, prepConfig{
						numIngesters:     tc.numIngesters,
						happyIngesters:   tc.happyIngesters,
						numDistributors:  1,
						shardByAllLabels: shardByAllLabels,
						limits:           limits,
						errFail:          tc.ingesterError,
						useStreamPush:    useStreamPush,
					})

					var request *cortexpb.WriteRequest
					if !tc.histogramSamples {
						request = makeWriteRequest(tc.samples.startTimestampMs, tc.samples.num, tc.metadata, 0)
					} else {
						request = makeWriteRequest(tc.samples.startTimestampMs, 0, tc.metadata, tc.samples.num)
					}
					response, err := ds[0].Push(ctx, request)
					assert.Equal(t, tc.expectedResponse, response)
					assert.Equal(t, status.Code(tc.expectedError), status.Code(err))

					// Check tracked Prometheus metrics. Since the Push() response is sent as soon as the quorum
					// is reached, when we reach this point the 3rd ingester may not have received series/metadata
					// yet. To avoid flaky test we retry metrics assertion until we hit the desired state (no error)
					// within a reasonable timeout.
					if tc.expectedMetrics != "" {
						test.Poll(t, time.Second, nil, func() interface{} {
							return testutil.GatherAndCompare(regs[0], strings.NewReader(tc.expectedMetrics), tc.metricNames...)
						})
					}
				})
			}
		}
	}
}

func TestDistributor_MetricsCleanup(t *testing.T) {
	t.Parallel()
	dists, _, regs, r := prepare(t, prepConfig{
		numDistributors: 1,
		numIngesters:    2,
		happyIngesters:  2,
	})
	d := dists[0]
	reg := regs[0]

	permanentMetrics := []string{
		"cortex_distributor_received_samples_total",
		"cortex_distributor_received_exemplars_total",
		"cortex_distributor_received_metadata_total",
		"cortex_distributor_samples_in_total",
		"cortex_distributor_ingester_append_failures_total",
		"cortex_distributor_ingester_appends_total",
		"cortex_distributor_ingester_query_failures_total",
		"cortex_distributor_ingester_queries_total",
	}
	removedMetrics := []string{
		"cortex_distributor_deduped_samples_total",
		"cortex_distributor_exemplars_in_total",
		"cortex_distributor_metadata_in_total",
		"cortex_distributor_non_ha_samples_received_total",
		"cortex_distributor_latest_seen_sample_timestamp_seconds",
		"cortex_distributor_received_samples_per_labelset_total",
	}

	allMetrics := append(removedMetrics, permanentMetrics...)

	d.receivedSamples.WithLabelValues("userA", sampleMetricTypeFloat).Add(5)
	d.receivedSamples.WithLabelValues("userB", sampleMetricTypeFloat).Add(10)
	d.receivedSamples.WithLabelValues("userC", sampleMetricTypeHistogram).Add(15)
	d.receivedExemplars.WithLabelValues("userA").Add(5)
	d.receivedExemplars.WithLabelValues("userB").Add(10)
	d.receivedMetadata.WithLabelValues("userA").Add(5)
	d.receivedMetadata.WithLabelValues("userB").Add(10)
	d.incomingSamples.WithLabelValues("userA", sampleMetricTypeFloat).Add(5)
	d.incomingSamples.WithLabelValues("userB", sampleMetricTypeHistogram).Add(6)
	d.incomingExemplars.WithLabelValues("userA").Add(5)
	d.incomingMetadata.WithLabelValues("userA").Add(5)
	d.nonHASamples.WithLabelValues("userA").Add(5)
	d.dedupedSamples.WithLabelValues("userA", "cluster1").Inc() // We cannot clean this metric
	d.latestSeenSampleTimestampPerUser.WithLabelValues("userA").Set(1111)
	d.receivedSamplesPerLabelSet.WithLabelValues("userA", sampleMetricTypeFloat, "{}").Add(5)
	d.receivedSamplesPerLabelSet.WithLabelValues("userA", sampleMetricTypeHistogram, "{}").Add(10)

	h, _, _ := r.GetAllInstanceDescs(ring.WriteNoExtend)
	ingId0, _ := r.GetInstanceIdByAddr(h[0].Addr)
	ingId1, _ := r.GetInstanceIdByAddr(h[1].Addr)
	d.ingesterAppends.WithLabelValues(ingId0, typeMetadata).Inc()
	d.ingesterAppendFailures.WithLabelValues(ingId0, typeMetadata, "2xx").Inc()
	d.ingesterAppends.WithLabelValues(ingId1, typeMetadata).Inc()
	d.ingesterAppendFailures.WithLabelValues(ingId1, typeMetadata, "2xx").Inc()
	d.ingesterQueries.WithLabelValues(ingId0).Inc()
	d.ingesterQueries.WithLabelValues(ingId1).Inc()
	d.ingesterQueryFailures.WithLabelValues(ingId0).Inc()
	d.ingesterQueryFailures.WithLabelValues(ingId1).Inc()

	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
		# HELP cortex_distributor_deduped_samples_total The total number of deduplicated samples.
		# TYPE cortex_distributor_deduped_samples_total counter
		cortex_distributor_deduped_samples_total{cluster="cluster1",user="userA"} 1

		# HELP cortex_distributor_latest_seen_sample_timestamp_seconds Unix timestamp of latest received sample per user.
		# TYPE cortex_distributor_latest_seen_sample_timestamp_seconds gauge
		cortex_distributor_latest_seen_sample_timestamp_seconds{user="userA"} 1111

		# HELP cortex_distributor_metadata_in_total The total number of metadata that have come in to the distributor, including rejected.
		# TYPE cortex_distributor_metadata_in_total counter
		cortex_distributor_metadata_in_total{user="userA"} 5

		# HELP cortex_distributor_non_ha_samples_received_total The total number of received samples for a user that has HA tracking turned on, but the sample didn't contain both HA labels.
		# TYPE cortex_distributor_non_ha_samples_received_total counter
		cortex_distributor_non_ha_samples_received_total{user="userA"} 5

		# HELP cortex_distributor_received_metadata_total The total number of received metadata, excluding rejected.
		# TYPE cortex_distributor_received_metadata_total counter
		cortex_distributor_received_metadata_total{user="userA"} 5
		cortex_distributor_received_metadata_total{user="userB"} 10

		# HELP cortex_distributor_received_samples_per_labelset_total The total number of received samples per label set, excluding rejected and deduped samples.
		# TYPE cortex_distributor_received_samples_per_labelset_total counter
		cortex_distributor_received_samples_per_labelset_total{labelset="{}",type="float",user="userA"} 5
		cortex_distributor_received_samples_per_labelset_total{labelset="{}",type="histogram",user="userA"} 10

		# HELP cortex_distributor_received_samples_total The total number of received samples, excluding rejected and deduped samples.
		# TYPE cortex_distributor_received_samples_total counter
		cortex_distributor_received_samples_total{type="float",user="userA"} 5
		cortex_distributor_received_samples_total{type="float",user="userB"} 10
		cortex_distributor_received_samples_total{type="histogram",user="userC"} 15

		# HELP cortex_distributor_received_exemplars_total The total number of received exemplars, excluding rejected and deduped exemplars.
		# TYPE cortex_distributor_received_exemplars_total counter
		cortex_distributor_received_exemplars_total{user="userA"} 5
		cortex_distributor_received_exemplars_total{user="userB"} 10

		# HELP cortex_distributor_samples_in_total The total number of samples that have come in to the distributor, including rejected or deduped samples.
		# TYPE cortex_distributor_samples_in_total counter
		cortex_distributor_samples_in_total{type="float",user="userA"} 5
		cortex_distributor_samples_in_total{type="histogram",user="userB"} 6

		# HELP cortex_distributor_exemplars_in_total The total number of exemplars that have come in to the distributor, including rejected or deduped exemplars.
		# TYPE cortex_distributor_exemplars_in_total counter
		cortex_distributor_exemplars_in_total{user="userA"} 5

		# HELP cortex_distributor_ingester_append_failures_total The total number of failed batch appends sent to ingesters.
		# TYPE cortex_distributor_ingester_append_failures_total counter
		cortex_distributor_ingester_append_failures_total{ingester="ingester-0",status="2xx",type="metadata"} 1
		cortex_distributor_ingester_append_failures_total{ingester="ingester-1",status="2xx",type="metadata"} 1
		# HELP cortex_distributor_ingester_appends_total The total number of batch appends sent to ingesters.
		# TYPE cortex_distributor_ingester_appends_total counter
		cortex_distributor_ingester_appends_total{ingester="ingester-0",type="metadata"} 1
		cortex_distributor_ingester_appends_total{ingester="ingester-1",type="metadata"} 1
		# HELP cortex_distributor_ingester_queries_total The total number of queries sent to ingesters.
		# TYPE cortex_distributor_ingester_queries_total counter
		cortex_distributor_ingester_queries_total{ingester="ingester-0"} 1
		cortex_distributor_ingester_queries_total{ingester="ingester-1"} 1
		# HELP cortex_distributor_ingester_query_failures_total The total number of failed queries sent to ingesters.
		# TYPE cortex_distributor_ingester_query_failures_total counter
		cortex_distributor_ingester_query_failures_total{ingester="ingester-0"} 1
		cortex_distributor_ingester_query_failures_total{ingester="ingester-1"} 1
		`), allMetrics...))

	d.cleanupInactiveUser("userA")

	err := r.KVClient.CAS(context.Background(), ingester.RingKey, func(in interface{}) (interface{}, bool, error) {
		r := in.(*ring.Desc)
		delete(r.Ingesters, "ingester-0")
		return in, true, nil
	})

	test.Poll(t, time.Second, true, func() interface{} {
		ings, _, _ := r.GetAllInstanceDescs(ring.Write)
		return len(ings) == 1
	})

	require.NoError(t, err)
	d.cleanStaleIngesterMetrics()

	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
		# HELP cortex_distributor_received_metadata_total The total number of received metadata, excluding rejected.
		# TYPE cortex_distributor_received_metadata_total counter
		cortex_distributor_received_metadata_total{user="userB"} 10

		# HELP cortex_distributor_received_samples_total The total number of received samples, excluding rejected and deduped samples.
		# TYPE cortex_distributor_received_samples_total counter
		cortex_distributor_received_samples_total{type="float",user="userB"} 10
		cortex_distributor_received_samples_total{type="histogram",user="userC"} 15

        # HELP cortex_distributor_samples_in_total The total number of samples that have come in to the distributor, including rejected or deduped samples.
        # TYPE cortex_distributor_samples_in_total counter
        cortex_distributor_samples_in_total{type="histogram",user="userB"} 6

		# HELP cortex_distributor_received_exemplars_total The total number of received exemplars, excluding rejected and deduped exemplars.
		# TYPE cortex_distributor_received_exemplars_total counter
		cortex_distributor_received_exemplars_total{user="userB"} 10

		# HELP cortex_distributor_ingester_append_failures_total The total number of failed batch appends sent to ingesters.
		# TYPE cortex_distributor_ingester_append_failures_total counter
		cortex_distributor_ingester_append_failures_total{ingester="ingester-1",status="2xx",type="metadata"} 1
		# HELP cortex_distributor_ingester_appends_total The total number of batch appends sent to ingesters.
		# TYPE cortex_distributor_ingester_appends_total counter
		cortex_distributor_ingester_appends_total{ingester="ingester-1",type="metadata"} 1
		# HELP cortex_distributor_ingester_queries_total The total number of queries sent to ingesters.
		# TYPE cortex_distributor_ingester_queries_total counter
		cortex_distributor_ingester_queries_total{ingester="ingester-1"} 1
		# HELP cortex_distributor_ingester_query_failures_total The total number of failed queries sent to ingesters.
		# TYPE cortex_distributor_ingester_query_failures_total counter
		cortex_distributor_ingester_query_failures_total{ingester="ingester-1"} 1
		`), permanentMetrics...))

	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(""), removedMetrics...))
}

func TestDistributor_PushIngestionRateLimiter(t *testing.T) {
	t.Parallel()
	type testPush struct {
		samples       int
		metadata      int
		expectedError error
	}

	ctx := user.InjectOrgID(context.Background(), "user")
	tests := map[string]struct {
		distributors          int
		ingestionRateStrategy string
		ingestionRate         float64
		ingestionBurstSize    int
		pushes                []testPush
	}{
		"local strategy: limit should be set to each distributor": {
			distributors:          2,
			ingestionRateStrategy: validation.LocalIngestionRateStrategy,
			ingestionRate:         10,
			ingestionBurstSize:    10,
			pushes: []testPush{
				{samples: 4, expectedError: nil},
				{metadata: 1, expectedError: nil},
				{samples: 6, expectedError: httpgrpc.Errorf(http.StatusTooManyRequests, "ingestion rate limit (10) exceeded while adding 6 samples and 0 metadata")},
				{samples: 4, metadata: 1, expectedError: nil},
				{samples: 1, expectedError: httpgrpc.Errorf(http.StatusTooManyRequests, "ingestion rate limit (10) exceeded while adding 1 samples and 0 metadata")},
				{metadata: 1, expectedError: httpgrpc.Errorf(http.StatusTooManyRequests, "ingestion rate limit (10) exceeded while adding 0 samples and 1 metadata")},
			},
		},
		"global strategy: limit should be evenly shared across distributors": {
			distributors:          2,
			ingestionRateStrategy: validation.GlobalIngestionRateStrategy,
			ingestionRate:         10,
			ingestionBurstSize:    5,
			pushes: []testPush{
				{samples: 2, expectedError: nil},
				{samples: 1, expectedError: nil},
				{samples: 2, metadata: 1, expectedError: httpgrpc.Errorf(http.StatusTooManyRequests, "ingestion rate limit (5) exceeded while adding 2 samples and 1 metadata")},
				{samples: 2, expectedError: nil},
				{samples: 1, expectedError: httpgrpc.Errorf(http.StatusTooManyRequests, "ingestion rate limit (5) exceeded while adding 1 samples and 0 metadata")},
				{metadata: 1, expectedError: httpgrpc.Errorf(http.StatusTooManyRequests, "ingestion rate limit (5) exceeded while adding 0 samples and 1 metadata")},
			},
		},
		"global strategy: burst should set to each distributor": {
			distributors:          2,
			ingestionRateStrategy: validation.GlobalIngestionRateStrategy,
			ingestionRate:         10,
			ingestionBurstSize:    20,
			pushes: []testPush{
				{samples: 10, expectedError: nil},
				{samples: 5, expectedError: nil},
				{samples: 5, metadata: 1, expectedError: httpgrpc.Errorf(http.StatusTooManyRequests, "ingestion rate limit (5) exceeded while adding 5 samples and 1 metadata")},
				{samples: 5, expectedError: nil},
				{samples: 1, expectedError: httpgrpc.Errorf(http.StatusTooManyRequests, "ingestion rate limit (5) exceeded while adding 1 samples and 0 metadata")},
				{metadata: 1, expectedError: httpgrpc.Errorf(http.StatusTooManyRequests, "ingestion rate limit (5) exceeded while adding 0 samples and 1 metadata")},
			},
		},
	}

	for testName, testData := range tests {
		testData := testData

		for _, enableHistogram := range []bool{false, true} {
			enableHistogram := enableHistogram
			t.Run(fmt.Sprintf("%s, histogram=%s", testName, strconv.FormatBool(enableHistogram)), func(t *testing.T) {
				t.Parallel()
				limits := &validation.Limits{}
				flagext.DefaultValues(limits)
				limits.IngestionRateStrategy = testData.ingestionRateStrategy
				limits.IngestionRate = testData.ingestionRate
				limits.IngestionBurstSize = testData.ingestionBurstSize

				// Start all expected distributors
				distributors, _, _, _ := prepare(t, prepConfig{
					numIngesters:     3,
					happyIngesters:   3,
					numDistributors:  testData.distributors,
					shardByAllLabels: true,
					limits:           limits,
				})

				// Push samples in multiple requests to the first distributor
				for _, push := range testData.pushes {
					var request *cortexpb.WriteRequest
					if !enableHistogram {
						request = makeWriteRequest(0, push.samples, push.metadata, 0)
					} else {
						request = makeWriteRequest(0, 0, push.metadata, push.samples)
					}
					response, err := distributors[0].Push(ctx, request)

					if push.expectedError == nil {
						assert.Equal(t, emptyResponse, response)
						assert.Nil(t, err)
					} else {
						assert.Nil(t, response)
						assert.Equal(t, push.expectedError, err)
					}
				}
			})
		}
	}
}

func TestDistributor_PushIngestionRateLimiter_Histograms(t *testing.T) {
	t.Parallel()
	type testPush struct {
		samples                              int
		nhSamples                            int
		metadata                             int
		expectedError                        error
		expectedNHDiscardedSampleMetricValue int
	}

	ctx := user.InjectOrgID(context.Background(), "user")
	tests := map[string]struct {
		distributors                         int
		ingestionRateStrategy                string
		ingestionRate                        float64
		ingestionBurstSize                   int
		nativeHistogramIngestionRateStrategy string
		nativeHistogramIngestionRate         float64
		nativeHistogramIngestionBurstSize    int
		pushes                               []testPush
	}{
		"local strategy: native histograms limit should be set to each distributor": {
			distributors:                      2,
			ingestionRateStrategy:             validation.LocalIngestionRateStrategy,
			ingestionRate:                     20,
			ingestionBurstSize:                20,
			nativeHistogramIngestionRate:      10,
			nativeHistogramIngestionBurstSize: 10,
			pushes: []testPush{
				{nhSamples: 4, expectedError: nil},
				{nhSamples: 6, expectedError: nil},
				{nhSamples: 6, expectedError: nil, expectedNHDiscardedSampleMetricValue: 6},
				{nhSamples: 4, metadata: 1, expectedError: nil, expectedNHDiscardedSampleMetricValue: 10},
				{nhSamples: 1, expectedError: nil, expectedNHDiscardedSampleMetricValue: 11},
				{nhSamples: 1, expectedError: nil, expectedNHDiscardedSampleMetricValue: 12},
			},
		},
		"global strategy: native histograms limit should be evenly shared across distributors": {
			distributors:                      2,
			ingestionRateStrategy:             validation.GlobalIngestionRateStrategy,
			ingestionRate:                     20,
			ingestionBurstSize:                10,
			nativeHistogramIngestionRate:      10,
			nativeHistogramIngestionBurstSize: 5,
			pushes: []testPush{
				{nhSamples: 2, expectedError: nil},
				{nhSamples: 1, expectedError: nil},
				{nhSamples: 3, metadata: 1, expectedError: nil, expectedNHDiscardedSampleMetricValue: 3},
				{nhSamples: 2, expectedError: nil, expectedNHDiscardedSampleMetricValue: 3},
				{nhSamples: 1, expectedError: nil, expectedNHDiscardedSampleMetricValue: 4},
				{nhSamples: 1, expectedError: nil, expectedNHDiscardedSampleMetricValue: 5},
			},
		},
		"global strategy: native histograms burst should set to each distributor": {
			distributors:                      2,
			ingestionRateStrategy:             validation.GlobalIngestionRateStrategy,
			ingestionRate:                     20,
			ingestionBurstSize:                40,
			nativeHistogramIngestionRate:      10,
			nativeHistogramIngestionBurstSize: 20,
			pushes: []testPush{
				{nhSamples: 10, expectedError: nil},
				{nhSamples: 5, expectedError: nil},
				{nhSamples: 6, metadata: 1, expectedError: nil, expectedNHDiscardedSampleMetricValue: 6},
				{nhSamples: 5, expectedError: nil, expectedNHDiscardedSampleMetricValue: 6},
				{nhSamples: 1, expectedError: nil, expectedNHDiscardedSampleMetricValue: 7},
				{nhSamples: 1, expectedError: nil, expectedNHDiscardedSampleMetricValue: 8},
			},
		},
		"local strategy: If NH samples hit NH rate limit, other samples should succeed when under rate limit": {
			distributors:                      2,
			ingestionRateStrategy:             validation.LocalIngestionRateStrategy,
			ingestionRate:                     20,
			ingestionBurstSize:                20,
			nativeHistogramIngestionRate:      5,
			nativeHistogramIngestionBurstSize: 5,
			pushes: []testPush{
				{samples: 5, nhSamples: 4, expectedError: nil},
				{samples: 6, nhSamples: 2, expectedError: nil, expectedNHDiscardedSampleMetricValue: 2},
				{samples: 4, metadata: 1, nhSamples: 1, expectedError: httpgrpc.Errorf(http.StatusTooManyRequests, "ingestion rate limit (20) exceeded while adding 5 samples and 1 metadata"), expectedNHDiscardedSampleMetricValue: 2},
				{metadata: 1, expectedError: nil, expectedNHDiscardedSampleMetricValue: 2},
			},
		},
		"global strategy: If NH samples hit NH rate limit, other samples should succeed when under rate limit": {
			distributors:                      2,
			ingestionRateStrategy:             validation.GlobalIngestionRateStrategy,
			ingestionRate:                     20,
			ingestionBurstSize:                10,
			nativeHistogramIngestionRate:      10,
			nativeHistogramIngestionBurstSize: 5,
			pushes: []testPush{
				{samples: 3, nhSamples: 2, expectedError: nil},
				{samples: 3, nhSamples: 4, expectedError: nil, expectedNHDiscardedSampleMetricValue: 4},
				{samples: 1, metadata: 1, nhSamples: 1, expectedError: httpgrpc.Errorf(http.StatusTooManyRequests, "ingestion rate limit (10) exceeded while adding 2 samples and 1 metadata"), expectedNHDiscardedSampleMetricValue: 4},
				{metadata: 1, expectedError: nil, expectedNHDiscardedSampleMetricValue: 4},
			},
		},
	}

	for testName, testData := range tests {
		testData := testData

		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			limits := &validation.Limits{}
			flagext.DefaultValues(limits)
			limits.IngestionRateStrategy = testData.ingestionRateStrategy
			limits.IngestionRate = testData.ingestionRate
			limits.IngestionBurstSize = testData.ingestionBurstSize
			limits.NativeHistogramIngestionRate = testData.nativeHistogramIngestionRate
			limits.NativeHistogramIngestionBurstSize = testData.nativeHistogramIngestionBurstSize

			// Start all expected distributors
			distributors, _, _, _ := prepare(t, prepConfig{
				numIngesters:     3,
				happyIngesters:   3,
				numDistributors:  testData.distributors,
				shardByAllLabels: true,
				limits:           limits,
			})

			// Push samples in multiple requests to the first distributor
			for _, push := range testData.pushes {
				var request = makeWriteRequest(0, push.samples, push.metadata, push.nhSamples)

				response, err := distributors[0].Push(ctx, request)

				if push.expectedError == nil {
					assert.Equal(t, emptyResponse, response)
					assert.Nil(t, err)
				} else {
					assert.Nil(t, response)
					assert.Equal(t, push.expectedError, err)
				}
				assert.Equal(t, float64(push.expectedNHDiscardedSampleMetricValue), testutil.ToFloat64(distributors[0].validateMetrics.DiscardedSamples.WithLabelValues(validation.NativeHistogramRateLimited, "user")))
			}
		})
	}

}

func TestPush_QuorumError(t *testing.T) {
	t.Parallel()

	var limits validation.Limits
	flagext.DefaultValues(&limits)

	limits.IngestionRate = math.MaxFloat64

	dists, ingesters, _, r := prepare(t, prepConfig{
		numDistributors:     1,
		numIngesters:        3,
		happyIngesters:      0,
		shuffleShardSize:    3,
		shardByAllLabels:    true,
		shuffleShardEnabled: true,
		limits:              &limits,
	})

	ctx := user.InjectOrgID(context.Background(), "user")

	d := dists[0]

	// we should run several write request to make sure we dont have any race condition on the batchTracker#record code
	numberOfWrites := 10000

	// Using 429 just to make sure we are not hitting the &limits
	// Simulating 2 4xx and 1 5xx -> Should return 4xx
	ingesters[0].failResp.Store(httpgrpc.Errorf(429, "Throttling"))
	ingesters[1].failResp.Store(httpgrpc.Errorf(500, "InternalServerError"))
	ingesters[2].failResp.Store(httpgrpc.Errorf(429, "Throttling"))

	for i := 0; i < numberOfWrites; i++ {
		request := makeWriteRequest(0, 30, 20, 10)
		_, err := d.Push(ctx, request)
		status, ok := status.FromError(err)
		require.True(t, ok)
		require.Equal(t, codes.Code(429), status.Code())
	}

	// Simulating 2 5xx and 1 4xx -> Should return 5xx
	ingesters[0].failResp.Store(httpgrpc.Errorf(500, "InternalServerError"))
	ingesters[1].failResp.Store(httpgrpc.Errorf(429, "Throttling"))
	ingesters[2].failResp.Store(httpgrpc.Errorf(500, "InternalServerError"))

	for i := 0; i < numberOfWrites; i++ {
		request := makeWriteRequest(0, 300, 200, 10)
		_, err := d.Push(ctx, request)
		status, ok := status.FromError(err)
		require.True(t, ok)
		require.Equal(t, codes.Code(500), status.Code())
	}

	// Simulating 2 different errors and 1 success -> This case we may return any of the errors
	ingesters[0].failResp.Store(httpgrpc.Errorf(500, "InternalServerError"))
	ingesters[1].failResp.Store(httpgrpc.Errorf(429, "Throttling"))
	ingesters[2].happy.Store(true)

	for i := 0; i < numberOfWrites; i++ {
		request := makeWriteRequest(0, 30, 20, 10)
		_, err := d.Push(ctx, request)
		status, ok := status.FromError(err)
		require.True(t, ok)
		require.Equal(t, codes.Code(429), status.Code())
	}

	// Simulating 1 error -> Should return 2xx
	ingesters[0].failResp.Store(httpgrpc.Errorf(500, "InternalServerError"))
	ingesters[1].happy.Store(true)
	ingesters[2].happy.Store(true)

	for i := 0; i < 1; i++ {
		request := makeWriteRequest(0, 30, 20, 10)
		_, err := d.Push(ctx, request)
		require.NoError(t, err)
	}

	// Simulating an unhealthy ingester (ingester 2)
	ingesters[0].failResp.Store(httpgrpc.Errorf(500, "InternalServerError"))
	ingesters[1].happy.Store(true)
	ingesters[2].happy.Store(true)

	err := r.KVClient.CAS(context.Background(), ingester.RingKey, func(in interface{}) (interface{}, bool, error) {
		r := in.(*ring.Desc)
		ingester2 := r.Ingesters["ingester-2"]
		ingester2.State = ring.LEFT
		ingester2.Timestamp = time.Now().Unix()
		r.Ingesters["ingester-2"] = ingester2
		return in, true, nil
	})

	require.NoError(t, err)

	// Give time to the ring get updated with the KV value
	test.Poll(t, 15*time.Second, true, func() interface{} {
		replicationSet, _ := r.GetAllHealthy(ring.Read)
		return len(replicationSet.Instances) == 2
	})

	for i := 0; i < numberOfWrites; i++ {
		request := makeWriteRequest(0, 30, 20, 10)
		_, err := d.Push(ctx, request)
		require.Error(t, err)
		status, ok := status.FromError(err)
		require.True(t, ok)
		require.Equal(t, codes.Code(500), status.Code())
	}
}

func TestDistributor_PushInstanceLimits(t *testing.T) {
	t.Parallel()

	type testPush struct {
		samples       int
		metadata      int
		expectedError error
	}

	ctx := user.InjectOrgID(context.Background(), "user")
	tests := map[string]struct {
		preInflight       int
		preInflightClient int
		preRateSamples    int        // initial rate before first push
		pushes            []testPush // rate is recomputed after each push

		// limits
		inflightLimit       int
		inflightClientLimit int
		ingestionRateLimit  float64

		metricNames     []string
		expectedMetrics string
	}{
		"no limits limit": {
			preInflight:    100,
			preRateSamples: 1000,

			pushes: []testPush{
				{samples: 100, expectedError: nil},
			},

			metricNames: []string{instanceLimitsMetric},
			expectedMetrics: `
				# HELP cortex_distributor_instance_limits Instance limits used by this distributor.
				# TYPE cortex_distributor_instance_limits gauge
				cortex_distributor_instance_limits{limit="max_inflight_client_requests"} 0
				cortex_distributor_instance_limits{limit="max_inflight_push_requests"} 0
				cortex_distributor_instance_limits{limit="max_ingestion_rate"} 0
			`,
		},
		"below inflight limit": {
			preInflight:   100,
			inflightLimit: 101,
			pushes: []testPush{
				{samples: 100, expectedError: nil},
			},

			metricNames: []string{instanceLimitsMetric, "cortex_distributor_inflight_push_requests"},
			expectedMetrics: `
				# HELP cortex_distributor_inflight_push_requests Current number of inflight push requests in distributor.
				# TYPE cortex_distributor_inflight_push_requests gauge
				cortex_distributor_inflight_push_requests 100

				# HELP cortex_distributor_instance_limits Instance limits used by this distributor.
				# TYPE cortex_distributor_instance_limits gauge
				cortex_distributor_instance_limits{limit="max_inflight_client_requests"} 0
				cortex_distributor_instance_limits{limit="max_inflight_push_requests"} 101
				cortex_distributor_instance_limits{limit="max_ingestion_rate"} 0
			`,
		},
		"hits inflight limit": {
			preInflight:   101,
			inflightLimit: 101,
			pushes: []testPush{
				{samples: 100, expectedError: httpgrpc.Errorf(http.StatusServiceUnavailable, "too many inflight push requests in distributor")},
			},
		},
		"below inflight client limit": {
			preInflightClient:   90,
			inflightClientLimit: 101,
			pushes: []testPush{
				{samples: 100, expectedError: nil},
			},

			metricNames: []string{instanceLimitsMetric},
			expectedMetrics: `
				# HELP cortex_distributor_instance_limits Instance limits used by this distributor.
				# TYPE cortex_distributor_instance_limits gauge
				cortex_distributor_instance_limits{limit="max_inflight_client_requests"} 101
				cortex_distributor_instance_limits{limit="max_inflight_push_requests"} 0
				cortex_distributor_instance_limits{limit="max_ingestion_rate"} 0
			`,
		},
		"hits inflight client limit": {
			preInflightClient:   103,
			inflightClientLimit: 101,
			pushes: []testPush{
				{samples: 100, expectedError: httpgrpc.Errorf(http.StatusServiceUnavailable,
					"too many inflight ingester client requests in distributor")},
			},
		},
		"below ingestion rate limit": {
			preRateSamples:     500,
			ingestionRateLimit: 1000,

			pushes: []testPush{
				{samples: 1000, expectedError: nil},
			},

			metricNames: []string{instanceLimitsMetric, "cortex_distributor_ingestion_rate_samples_per_second"},
			expectedMetrics: `
				# HELP cortex_distributor_ingestion_rate_samples_per_second Current ingestion rate in samples/sec that distributor is using to limit access.
				# TYPE cortex_distributor_ingestion_rate_samples_per_second gauge
				cortex_distributor_ingestion_rate_samples_per_second 600

				# HELP cortex_distributor_instance_limits Instance limits used by this distributor.
				# TYPE cortex_distributor_instance_limits gauge
				cortex_distributor_instance_limits{limit="max_inflight_client_requests"} 0
				cortex_distributor_instance_limits{limit="max_inflight_push_requests"} 0
				cortex_distributor_instance_limits{limit="max_ingestion_rate"} 1000
			`,
		},
		"hits rate limit on first request, but second request can proceed": {
			preRateSamples:     1200,
			ingestionRateLimit: 1000,

			pushes: []testPush{
				{samples: 100, expectedError: httpgrpc.Errorf(http.StatusServiceUnavailable, "distributor's samples push rate limit reached")},
				{samples: 100, expectedError: nil},
			},
		},

		"below rate limit on first request, but hits the rate limit afterwards": {
			preRateSamples:     500,
			ingestionRateLimit: 1000,

			pushes: []testPush{
				{samples: 5000, expectedError: nil}, // after push, rate = 500 + 0.2*(5000-500) = 1400
				{samples: 5000, expectedError: httpgrpc.Errorf(http.StatusServiceUnavailable, "distributor's samples push rate limit reached")}, // after push, rate = 1400 + 0.2*(0 - 1400) = 1120
				{samples: 5000, expectedError: httpgrpc.Errorf(http.StatusServiceUnavailable, "distributor's samples push rate limit reached")}, // after push, rate = 1120 + 0.2*(0 - 1120) = 896
				{samples: 5000, expectedError: nil}, // 896 is below 1000, so this push succeeds, new rate = 896 + 0.2*(5000-896) = 1716.8
			},
		},
	}

	for testName, testData := range tests {
		testData := testData

		for _, enableHistogram := range []bool{true, false} {
			enableHistogram := enableHistogram
			t.Run(fmt.Sprintf("%s, histogram=%s", testName, strconv.FormatBool(enableHistogram)), func(t *testing.T) {
				t.Parallel()
				limits := &validation.Limits{}
				flagext.DefaultValues(limits)

				// Start all expected distributors
				distributors, _, regs, _ := prepare(t, prepConfig{
					numIngesters:              3,
					happyIngesters:            3,
					numDistributors:           1,
					shardByAllLabels:          true,
					limits:                    limits,
					maxInflightRequests:       testData.inflightLimit,
					maxInflightClientRequests: testData.inflightClientLimit,
					maxIngestionRate:          testData.ingestionRateLimit,
				})

				d := distributors[0]
				d.inflightPushRequests.Add(int64(testData.preInflight))
				d.inflightClientRequests.Add(int64(testData.preInflightClient))
				d.ingestionRate.Add(int64(testData.preRateSamples))

				d.ingestionRate.Tick()

				for _, push := range testData.pushes {
					var request *cortexpb.WriteRequest
					if enableHistogram {
						request = makeWriteRequest(0, 0, push.metadata, push.samples)
					} else {
						request = makeWriteRequest(0, push.samples, push.metadata, 0)
					}
					_, err := d.Push(ctx, request)

					if push.expectedError == nil {
						assert.Nil(t, err)
					} else {
						assert.Equal(t, push.expectedError, err)
					}

					d.ingestionRate.Tick()

					if testData.expectedMetrics != "" {
						assert.NoError(t, testutil.GatherAndCompare(regs[0], strings.NewReader(testData.expectedMetrics), testData.metricNames...))
					}
				}
			})
		}
	}
}

func TestDistributor_PushHAInstances(t *testing.T) {
	t.Parallel()
	ctx := user.InjectOrgID(context.Background(), "user")

	for i, tc := range []struct {
		enableTracker    bool
		acceptedReplica  string
		testReplica      string
		cluster          string
		samples          int
		expectedResponse *cortexpb.WriteResponse
		expectedCode     int32
	}{
		{
			enableTracker:    true,
			acceptedReplica:  "instance0",
			testReplica:      "instance0",
			cluster:          "cluster0",
			samples:          5,
			expectedResponse: emptyResponse,
		},
		// The 202 indicates that we didn't accept this sample.
		{
			enableTracker:   true,
			acceptedReplica: "instance2",
			testReplica:     "instance0",
			cluster:         "cluster0",
			samples:         5,
			expectedCode:    202,
		},
		// If the HA tracker is disabled we should still accept samples that have both labels.
		{
			enableTracker:    false,
			acceptedReplica:  "instance0",
			testReplica:      "instance0",
			cluster:          "cluster0",
			samples:          5,
			expectedResponse: emptyResponse,
		},
		// Using very long replica label value results in validation error.
		{
			enableTracker:    true,
			acceptedReplica:  "instance0",
			testReplica:      "instance1234567890123456789012345678901234567890",
			cluster:          "cluster0",
			samples:          5,
			expectedResponse: emptyResponse,
			expectedCode:     400,
		},
	} {
		for _, shardByAllLabels := range []bool{true, false} {
			tc := tc
			shardByAllLabels := shardByAllLabels
			for _, enableHistogram := range []bool{true, false} {
				enableHistogram := enableHistogram
				t.Run(fmt.Sprintf("[%d](shardByAllLabels=%v, histogram=%v)", i, shardByAllLabels, enableHistogram), func(t *testing.T) {
					t.Parallel()
					var limits validation.Limits
					flagext.DefaultValues(&limits)
					limits.AcceptHASamples = true
					limits.MaxLabelValueLength = 15

					ds, _, _, _ := prepare(t, prepConfig{
						numIngesters:     3,
						happyIngesters:   3,
						numDistributors:  1,
						shardByAllLabels: shardByAllLabels,
						limits:           &limits,
						enableTracker:    tc.enableTracker,
					})

					d := ds[0]

					userID, err := tenant.TenantID(ctx)
					assert.NoError(t, err)
					err = d.HATracker.CheckReplica(ctx, userID, tc.cluster, tc.acceptedReplica, time.Now())
					assert.NoError(t, err)

					request := makeWriteRequestHA(tc.samples, tc.testReplica, tc.cluster, enableHistogram)
					response, err := d.Push(ctx, request)
					assert.Equal(t, tc.expectedResponse, response)

					httpResp, ok := httpgrpc.HTTPResponseFromError(err)
					if ok {
						assert.Equal(t, tc.expectedCode, httpResp.Code)
					} else if tc.expectedCode != 0 {
						assert.Fail(t, "expected HTTP status code", tc.expectedCode)
					}
				})
			}
		}
	}
}

func TestDistributor_PushMixedHAInstances(t *testing.T) {
	t.Parallel()
	ctx := user.InjectOrgID(context.Background(), "user")

	for i, tc := range []struct {
		enableTracker        bool
		acceptMixedHASamples bool
		samples              int
		expectedResponse     *cortexpb.WriteResponse
		expectedCode         int32
	}{
		{
			enableTracker:        true,
			acceptMixedHASamples: true,
			samples:              5,
			expectedResponse:     emptyResponse,
			expectedCode:         202,
		},
	} {
		for _, shardByAllLabels := range []bool{true} {
			tc := tc
			shardByAllLabels := shardByAllLabels
			for _, enableHistogram := range []bool{false} {
				enableHistogram := enableHistogram
				t.Run(fmt.Sprintf("[%d](shardByAllLabels=%v, histogram=%v)", i, shardByAllLabels, enableHistogram), func(t *testing.T) {
					t.Parallel()
					var limits validation.Limits
					flagext.DefaultValues(&limits)
					limits.AcceptHASamples = true
					limits.AcceptMixedHASamples = tc.acceptMixedHASamples
					limits.MaxLabelValueLength = 25

					ds, ingesters, _, _ := prepare(t, prepConfig{
						numIngesters:      2,
						happyIngesters:    2,
						numDistributors:   1,
						replicationFactor: 2,
						shardByAllLabels:  shardByAllLabels,
						limits:            &limits,
						enableTracker:     tc.enableTracker,
					})

					d := ds[0]

					request := makeWriteRequestHAMixedSamples(tc.samples, enableHistogram)
					response, _ := d.Push(ctx, request)
					assert.Equal(t, tc.expectedResponse, response)

					for i := range ingesters {
						timeseries := ingesters[i].series()
						assert.Equal(t, 5, len(timeseries))
						clusters := make(map[string]int)
						replicas := make(map[string]int)
						for _, v := range timeseries {
							replicaLabel := ""
							clusterLabel := ""
							for _, label := range v.Labels {
								if label.Name == "__replica__" {
									replicaLabel = label.Value
									_, ok := replicas[label.Value]
									if !ok {
										replicas[label.Value] = 1
									} else {
										assert.Fail(t, fmt.Sprintf("Two timeseries with same replica label, %s, were found, but only one should be present", label.Value))
									}
								}
								if label.Name == "cluster" {
									clusterLabel = label.Value
									_, ok := clusters[label.Value]
									if !ok {
										clusters[label.Value] = 1
									} else {
										assert.Fail(t, fmt.Sprintf("Two timeseries with same cluster label, %s, were found, but only one should be present", label.Value))
									}
								}
							}
							if clusterLabel == "" && replicaLabel != "" {
								assert.Equal(t, "replicaNoCluster", replicaLabel)
							}
							assert.Equal(t, tc.samples, len(v.Samples))
						}
						assert.Equal(t, 3, len(clusters))
						for _, nr := range clusters {
							assert.Equal(t, true, nr == 1)
						}
						assert.Equal(t, 1, len(replicas))
						for _, nr := range clusters {
							assert.Equal(t, true, nr == 1)
						}
					}
				})
			}
		}
	}
}

func TestDistributor_PushQuery(t *testing.T) {
	t.Parallel()
	const shuffleShardSize = 5

	ctx := user.InjectOrgID(context.Background(), "user")
	nameMatcher := mustEqualMatcher(model.MetricNameLabel, "foo")
	barMatcher := mustEqualMatcher("bar", "baz")

	type testcase struct {
		name                string
		numIngesters        int
		happyIngesters      int
		samples             int
		metadata            int
		matchers            []*labels.Matcher
		expectedIngesters   int
		expectedResponse    model.Matrix
		expectedError       error
		shardByAllLabels    bool
		shuffleShardEnabled bool
	}

	// We'll programmatically build the test cases now, as we want complete
	// coverage along quite a few different axis.
	testcases := []testcase{}

	// Run every test in both sharding modes.
	for _, shardByAllLabels := range []bool{true, false} {

		// Test with between 2 and 10 ingesters.
		for numIngesters := 2; numIngesters < 10; numIngesters++ {

			// Test with between 0 and numIngesters "happy" ingesters.
			for happyIngesters := 0; happyIngesters <= numIngesters; happyIngesters++ {

				// Test either with shuffle-sharding enabled or disabled.
				for _, shuffleShardEnabled := range []bool{false, true} {
					scenario := fmt.Sprintf("shardByAllLabels=%v, numIngester=%d, happyIngester=%d, shuffleSharding=%v)", shardByAllLabels, numIngesters, happyIngesters, shuffleShardEnabled)

					// The number of ingesters we expect to query depends whether shuffle sharding and/or
					// shard by all labels are enabled.
					var expectedIngesters int
					if shuffleShardEnabled {
						expectedIngesters = min(shuffleShardSize, numIngesters)
					} else if shardByAllLabels {
						expectedIngesters = numIngesters
					} else {
						expectedIngesters = 3 // Replication factor
					}

					// When we're not sharding by metric name, queriers with more than one
					// failed ingester should fail.
					if shardByAllLabels && numIngesters-happyIngesters > 1 {
						testcases = append(testcases, testcase{
							name:                fmt.Sprintf("ExpectFail(%s)", scenario),
							numIngesters:        numIngesters,
							happyIngesters:      happyIngesters,
							matchers:            []*labels.Matcher{nameMatcher, barMatcher},
							expectedError:       errFail,
							shardByAllLabels:    shardByAllLabels,
							shuffleShardEnabled: shuffleShardEnabled,
						})
						continue
					}

					// When we have less ingesters than replication factor, any failed ingester
					// will cause a failure.
					if numIngesters < 3 && happyIngesters < 2 {
						testcases = append(testcases, testcase{
							name:                fmt.Sprintf("ExpectFail(%s)", scenario),
							numIngesters:        numIngesters,
							happyIngesters:      happyIngesters,
							matchers:            []*labels.Matcher{nameMatcher, barMatcher},
							expectedError:       errFail,
							shardByAllLabels:    shardByAllLabels,
							shuffleShardEnabled: shuffleShardEnabled,
						})
						continue
					}

					// If we're sharding by metric name and we have failed ingesters, we can't
					// tell ahead of time if the query will succeed, as we don't know which
					// ingesters will hold the results for the query.
					if !shardByAllLabels && numIngesters-happyIngesters > 1 {
						continue
					}

					// Reading all the samples back should succeed.
					testcases = append(testcases, testcase{
						name:                fmt.Sprintf("ReadAll(%s)", scenario),
						numIngesters:        numIngesters,
						happyIngesters:      happyIngesters,
						samples:             10,
						matchers:            []*labels.Matcher{nameMatcher, barMatcher},
						expectedResponse:    expectedResponse(0, 10),
						expectedIngesters:   expectedIngesters,
						shardByAllLabels:    shardByAllLabels,
						shuffleShardEnabled: shuffleShardEnabled,
					})

					// As should reading none of the samples back.
					testcases = append(testcases, testcase{
						name:                fmt.Sprintf("ReadNone(%s)", scenario),
						numIngesters:        numIngesters,
						happyIngesters:      happyIngesters,
						samples:             10,
						matchers:            []*labels.Matcher{nameMatcher, mustEqualMatcher("not", "found")},
						expectedResponse:    expectedResponse(0, 0),
						expectedIngesters:   expectedIngesters,
						shardByAllLabels:    shardByAllLabels,
						shuffleShardEnabled: shuffleShardEnabled,
					})

					// And reading each sample individually.
					for i := 0; i < 10; i++ {
						testcases = append(testcases, testcase{
							name:                fmt.Sprintf("ReadOne(%s, sample=%d)", scenario, i),
							numIngesters:        numIngesters,
							happyIngesters:      happyIngesters,
							samples:             10,
							matchers:            []*labels.Matcher{nameMatcher, mustEqualMatcher("sample", strconv.Itoa(i))},
							expectedResponse:    expectedResponse(i, i+1),
							expectedIngesters:   expectedIngesters,
							shardByAllLabels:    shardByAllLabels,
							shuffleShardEnabled: shuffleShardEnabled,
						})
					}
				}
			}
		}
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ds, ingesters, _, _ := prepare(t, prepConfig{
				numIngesters:        tc.numIngesters,
				happyIngesters:      tc.happyIngesters,
				numDistributors:     1,
				shardByAllLabels:    tc.shardByAllLabels,
				shuffleShardEnabled: tc.shuffleShardEnabled,
				shuffleShardSize:    shuffleShardSize,
			})

			request := makeWriteRequest(0, tc.samples, tc.metadata, 0)
			writeResponse, err := ds[0].Push(ctx, request)
			assert.Equal(t, &cortexpb.WriteResponse{}, writeResponse)
			assert.Nil(t, err)

			var response model.Matrix
			series, err := ds[0].QueryStream(ctx, 0, 10, false, tc.matchers...)
			assert.Equal(t, tc.expectedError, err)

			if series == nil {
				response, err = chunkcompat.SeriesChunksToMatrix(0, 10, nil)
			} else {
				response, err = chunkcompat.SeriesChunksToMatrix(0, 10, series.Chunkseries)
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedResponse.String(), response.String())

			// Check how many ingesters have been queried.
			// Due to the quorum the distributor could cancel the last request towards ingesters
			// if all other ones are successful, so we're good either has been queried X or X-1
			// ingesters.
			if tc.expectedError == nil {
				assert.Contains(t, []int{tc.expectedIngesters, tc.expectedIngesters - 1}, countMockIngestersCalls(ingesters, "QueryStream"))
			}
		})
	}
}

func TestDistributor_QueryStream_ShouldReturnErrorIfMaxChunksPerQueryLimitIsReached(t *testing.T) {
	t.Parallel()
	const maxChunksLimit = 30 // Chunks are duplicated due to replication factor.

	for _, histogram := range []bool{true, false} {
		ctx := user.InjectOrgID(context.Background(), "user")
		limits := &validation.Limits{}
		flagext.DefaultValues(limits)
		limits.MaxChunksPerQuery = maxChunksLimit

		// Prepare distributors.
		ds, _, _, _ := prepare(t, prepConfig{
			numIngesters:     3,
			happyIngesters:   3,
			numDistributors:  1,
			shardByAllLabels: true,
			limits:           limits,
		})

		ctx = limiter.AddQueryLimiterToContext(ctx, limiter.NewQueryLimiter(0, 0, maxChunksLimit, 0))

		// Push a number of series below the max chunks limit. Each series has 1 sample,
		// so expect 1 chunk per series when querying back.
		initialSeries := maxChunksLimit / 3
		var writeReq *cortexpb.WriteRequest
		if histogram {
			writeReq = makeWriteRequest(0, 0, 0, initialSeries)
		} else {
			writeReq = makeWriteRequest(0, initialSeries, 0, 0)
		}
		writeRes, err := ds[0].Push(ctx, writeReq)
		assert.Equal(t, &cortexpb.WriteResponse{}, writeRes)
		assert.Nil(t, err)

		allSeriesMatchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchRegexp, model.MetricNameLabel, ".+"),
		}

		// Since the number of series (and thus chunks) is equal to the limit (but doesn't
		// exceed it), we expect a query running on all series to succeed.
		queryRes, err := ds[0].QueryStream(ctx, math.MinInt32, math.MaxInt32, false, allSeriesMatchers...)
		require.NoError(t, err)
		assert.Len(t, queryRes.Chunkseries, initialSeries)

		// Push more series to exceed the limit once we'll query back all series.
		writeReq = &cortexpb.WriteRequest{}
		for i := 0; i < maxChunksLimit; i++ {
			writeReq.Timeseries = append(writeReq.Timeseries,
				makeWriteRequestTimeseries([]cortexpb.LabelAdapter{{Name: model.MetricNameLabel, Value: fmt.Sprintf("another_series_%d", i)}}, 0, 0, histogram),
			)
		}

		writeRes, err = ds[0].Push(ctx, writeReq)
		assert.Equal(t, &cortexpb.WriteResponse{}, writeRes)
		assert.Nil(t, err)

		// Since the number of series (and thus chunks) is exceeding to the limit, we expect
		// a query running on all series to fail.
		_, err = ds[0].QueryStream(ctx, math.MinInt32, math.MaxInt32, false, allSeriesMatchers...)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "the query hit the max number of chunks limit")
	}
}

func TestDistributor_QueryStream_ShouldReturnErrorIfMaxSeriesPerQueryLimitIsReached(t *testing.T) {
	t.Parallel()
	const maxSeriesLimit = 10

	for _, histogram := range []bool{true, false} {
		ctx := user.InjectOrgID(context.Background(), "user")
		limits := &validation.Limits{}
		flagext.DefaultValues(limits)
		ctx = limiter.AddQueryLimiterToContext(ctx, limiter.NewQueryLimiter(maxSeriesLimit, 0, 0, 0))

		// Prepare distributors.
		ds, _, _, _ := prepare(t, prepConfig{
			numIngesters:     3,
			happyIngesters:   3,
			numDistributors:  1,
			shardByAllLabels: true,
			limits:           limits,
		})

		// Push a number of series below the max series limit.
		initialSeries := maxSeriesLimit
		var writeReq *cortexpb.WriteRequest
		if histogram {
			writeReq = makeWriteRequest(0, 0, 0, initialSeries)
		} else {
			writeReq = makeWriteRequest(0, initialSeries, 0, 0)
		}

		writeRes, err := ds[0].Push(ctx, writeReq)
		assert.Equal(t, &cortexpb.WriteResponse{}, writeRes)
		assert.Nil(t, err)

		allSeriesMatchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchRegexp, model.MetricNameLabel, ".+"),
		}

		// Since the number of series is equal to the limit (but doesn't
		// exceed it), we expect a query running on all series to succeed.
		queryRes, err := ds[0].QueryStream(ctx, math.MinInt32, math.MaxInt32, false, allSeriesMatchers...)
		require.NoError(t, err)
		assert.Len(t, queryRes.Chunkseries, initialSeries)

		// Push more series to exceed the limit once we'll query back all series.
		writeReq = &cortexpb.WriteRequest{}
		writeReq.Timeseries = append(writeReq.Timeseries,
			makeWriteRequestTimeseries([]cortexpb.LabelAdapter{{Name: model.MetricNameLabel, Value: "another_series"}}, 0, 0, histogram),
		)

		writeRes, err = ds[0].Push(ctx, writeReq)
		assert.Equal(t, &cortexpb.WriteResponse{}, writeRes)
		assert.Nil(t, err)

		// Since the number of series is exceeding the limit, we expect
		// a query running on all series to fail.
		_, err = ds[0].QueryStream(ctx, math.MinInt32, math.MaxInt32, false, allSeriesMatchers...)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max number of series limit")
	}
}

func TestDistributor_QueryStream_ShouldReturnErrorIfMaxChunkBytesPerQueryLimitIsReached(t *testing.T) {
	t.Parallel()
	const seriesToAdd = 10

	for _, histogram := range []bool{true, false} {
		ctx := user.InjectOrgID(context.Background(), "user")
		limits := &validation.Limits{}
		flagext.DefaultValues(limits)

		// Prepare distributors.
		// Use replication factor of 2 to always read all the chunks from both ingesters,
		// this guarantees us to always read the same chunks and have a stable test.
		ds, _, _, _ := prepare(t, prepConfig{
			numIngesters:      2,
			happyIngesters:    2,
			numDistributors:   1,
			shardByAllLabels:  true,
			limits:            limits,
			replicationFactor: 2,
		})

		allSeriesMatchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchRegexp, model.MetricNameLabel, ".+"),
		}
		// Push a single series to allow us to calculate the chunk size to calculate the limit for the test.
		writeReq := &cortexpb.WriteRequest{}
		writeReq.Timeseries = append(writeReq.Timeseries,
			makeWriteRequestTimeseries([]cortexpb.LabelAdapter{{Name: model.MetricNameLabel, Value: "another_series"}}, 0, 0, histogram),
		)
		writeRes, err := ds[0].Push(ctx, writeReq)
		assert.Equal(t, &cortexpb.WriteResponse{}, writeRes)
		assert.Nil(t, err)
		chunkSizeResponse, err := ds[0].QueryStream(ctx, math.MinInt32, math.MaxInt32, false, allSeriesMatchers...)
		require.NoError(t, err)

		// Use the resulting chunks size to calculate the limit as (series to add + our test series) * the response chunk size.
		var responseChunkSize = chunkSizeResponse.ChunksSize()
		var maxBytesLimit = (seriesToAdd) * responseChunkSize

		// Update the limiter with the calculated limits.
		ctx = limiter.AddQueryLimiterToContext(ctx, limiter.NewQueryLimiter(0, maxBytesLimit, 0, 0))

		// Push a number of series below the max chunk bytes limit. Subtract one for the series added above.
		if histogram {
			writeReq = makeWriteRequest(0, 0, 0, seriesToAdd-1)
		} else {
			writeReq = makeWriteRequest(0, seriesToAdd-1, 0, 0)
		}
		writeRes, err = ds[0].Push(ctx, writeReq)
		assert.Equal(t, &cortexpb.WriteResponse{}, writeRes)
		assert.Nil(t, err)

		// Since the number of chunk bytes is equal to the limit (but doesn't
		// exceed it), we expect a query running on all series to succeed.
		queryRes, err := ds[0].QueryStream(ctx, math.MinInt32, math.MaxInt32, false, allSeriesMatchers...)
		require.NoError(t, err)
		assert.Len(t, queryRes.Chunkseries, seriesToAdd)

		// Push another series to exceed the chunk bytes limit once we'll query back all series.
		writeReq = &cortexpb.WriteRequest{}
		writeReq.Timeseries = append(writeReq.Timeseries,
			makeWriteRequestTimeseries([]cortexpb.LabelAdapter{{Name: model.MetricNameLabel, Value: "another_series_1"}}, 0, 0, histogram),
		)

		writeRes, err = ds[0].Push(ctx, writeReq)
		assert.Equal(t, &cortexpb.WriteResponse{}, writeRes)
		assert.Nil(t, err)

		// Since the aggregated chunk size is exceeding the limit, we expect
		// a query running on all series to fail.
		_, err = ds[0].QueryStream(ctx, math.MinInt32, math.MaxInt32, false, allSeriesMatchers...)
		require.Error(t, err)
		assert.Equal(t, err, validation.LimitError(fmt.Sprintf(limiter.ErrMaxChunkBytesHit, maxBytesLimit)))
	}
}

func TestDistributor_QueryStream_ShouldReturnErrorIfMaxDataBytesPerQueryLimitIsReached(t *testing.T) {
	t.Parallel()
	const seriesToAdd = 10

	for _, histogram := range []bool{true, false} {
		ctx := user.InjectOrgID(context.Background(), "user")
		limits := &validation.Limits{}
		flagext.DefaultValues(limits)

		// Prepare distributors.
		// Use replication factor of 2 to always read all the chunks from both ingesters,
		// this guarantees us to always read the same chunks and have a stable test.
		ds, _, _, _ := prepare(t, prepConfig{
			numIngesters:      2,
			happyIngesters:    2,
			numDistributors:   1,
			shardByAllLabels:  true,
			limits:            limits,
			replicationFactor: 2,
		})

		allSeriesMatchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchRegexp, model.MetricNameLabel, ".+"),
		}
		// Push a single series to allow us to calculate the label size to calculate the limit for the test.
		writeReq := &cortexpb.WriteRequest{}
		writeReq.Timeseries = append(writeReq.Timeseries,
			makeWriteRequestTimeseries([]cortexpb.LabelAdapter{{Name: model.MetricNameLabel, Value: "another_series"}}, 0, 0, histogram),
		)

		writeRes, err := ds[0].Push(ctx, writeReq)
		assert.Equal(t, &cortexpb.WriteResponse{}, writeRes)
		assert.Nil(t, err)
		dataSizeResponse, err := ds[0].QueryStream(ctx, math.MinInt32, math.MaxInt32, false, allSeriesMatchers...)
		require.NoError(t, err)

		// Use the resulting chunks size to calculate the limit as (series to add + our test series) * the response chunk size.
		var dataSize = dataSizeResponse.Size()
		var maxBytesLimit = (seriesToAdd) * dataSize * 2 // Multiplying by RF because the limit is applied before de-duping.

		// Update the limiter with the calculated limits.
		ctx = limiter.AddQueryLimiterToContext(ctx, limiter.NewQueryLimiter(0, 0, 0, maxBytesLimit))

		// Push a number of series below the max chunk bytes limit. Subtract one for the series added above.
		if histogram {
			writeReq = makeWriteRequest(0, 0, 0, seriesToAdd-1)
		} else {
			writeReq = makeWriteRequest(0, seriesToAdd-1, 0, 0)
		}
		writeRes, err = ds[0].Push(ctx, writeReq)
		assert.Equal(t, &cortexpb.WriteResponse{}, writeRes)
		assert.Nil(t, err)

		// Since the number of chunk bytes is equal to the limit (but doesn't
		// exceed it), we expect a query running on all series to succeed.
		queryRes, err := ds[0].QueryStream(ctx, math.MinInt32, math.MaxInt32, false, allSeriesMatchers...)
		require.NoError(t, err)
		assert.Len(t, queryRes.Chunkseries, seriesToAdd)

		// Push another series to exceed the chunk bytes limit once we'll query back all series.
		writeReq = &cortexpb.WriteRequest{}
		writeReq.Timeseries = append(writeReq.Timeseries,
			makeWriteRequestTimeseries([]cortexpb.LabelAdapter{{Name: model.MetricNameLabel, Value: "another_series_1"}}, 0, 0, histogram),
		)

		writeRes, err = ds[0].Push(ctx, writeReq)
		assert.Equal(t, &cortexpb.WriteResponse{}, writeRes)
		assert.Nil(t, err)

		// Since the aggregated chunk size is exceeding the limit, we expect
		// a query running on all series to fail.
		_, err = ds[0].QueryStream(ctx, math.MinInt32, math.MaxInt32, false, allSeriesMatchers...)
		require.Error(t, err)
		assert.Equal(t, err, validation.LimitError(fmt.Sprintf(limiter.ErrMaxDataBytesHit, maxBytesLimit)))
	}
}

func TestDistributor_Push_LabelRemoval(t *testing.T) {
	t.Parallel()
	ctx := user.InjectOrgID(context.Background(), "user")

	type testcase struct {
		inputSeries    labels.Labels
		expectedSeries labels.Labels
		removeReplica  bool
		removeLabels   []string
		exemplars      []cortexpb.Exemplar
	}

	cases := []testcase{
		// Remove both cluster and replica label.
		{
			removeReplica: true,
			removeLabels:  []string{"cluster"},
			inputSeries: labels.FromStrings(
				"__name__", "some_metric",
				"cluster", "one",
				"__replica__", "two",
			),
			expectedSeries: labels.FromStrings(
				"__name__", "some_metric",
			),
		},

		// Remove multiple labels and replica.
		{
			removeReplica: true,
			removeLabels:  []string{"foo", "some"},
			inputSeries: labels.FromStrings(
				"__name__", "some_metric",
				"cluster", "one",
				"__replica__", "two",
				"foo", "bar",
				"some", "thing",
			),
			expectedSeries: labels.FromStrings(
				"__name__", "some_metric",
				"cluster", "one",
			),
		},

		// Don't remove any labels.
		{
			removeReplica: false,
			inputSeries: labels.FromStrings(
				"__name__", "some_metric",
				"__replica__", "two",
				"cluster", "one",
			),
			expectedSeries: labels.FromStrings(
				"__name__", "some_metric",
				"__replica__", "two",
				"cluster", "one",
			),
		},

		// No labels left.
		{
			removeReplica: true,
			removeLabels:  []string{"cluster"},
			inputSeries: labels.FromStrings(
				"cluster", "one",
				"__replica__", "two",
			),
			expectedSeries: labels.Labels{},
			exemplars: []cortexpb.Exemplar{
				{Labels: cortexpb.FromLabelsToLabelAdapters(labels.FromStrings("test", "a")), Value: 1, TimestampMs: 0},
				{Labels: cortexpb.FromLabelsToLabelAdapters(labels.FromStrings("test", "b")), Value: 1, TimestampMs: 0},
			},
		},
	}

	for _, tc := range cases {
		for _, histogram := range []bool{true, false} {
			var err error
			var limits validation.Limits
			flagext.DefaultValues(&limits)
			limits.DropLabels = tc.removeLabels
			limits.AcceptHASamples = tc.removeReplica

			expectedDiscardedSamples := 0
			expectedDiscardedExemplars := 0
			if tc.expectedSeries.Len() == 0 {
				expectedDiscardedSamples = 1
				expectedDiscardedExemplars = len(tc.exemplars)
				// Allow series with no labels to ingest
				limits.EnforceMetricName = false
			}

			ds, ingesters, _, _ := prepare(t, prepConfig{
				numIngesters:     2,
				happyIngesters:   2,
				numDistributors:  1,
				shardByAllLabels: true,
				limits:           &limits,
			})

			// Push the series to the distributor
			req := mockWriteRequest([]labels.Labels{tc.inputSeries}, 1, 1, histogram)
			req.Timeseries[0].Exemplars = tc.exemplars
			_, err = ds[0].Push(ctx, req)
			require.NoError(t, err)

			actualDiscardedSamples := testutil.ToFloat64(ds[0].validateMetrics.DiscardedSamples.WithLabelValues(validation.DroppedByUserConfigurationOverride, "user"))
			actualDiscardedExemplars := testutil.ToFloat64(ds[0].validateMetrics.DiscardedExemplars.WithLabelValues(validation.DroppedByUserConfigurationOverride, "user"))
			require.Equal(t, float64(expectedDiscardedSamples), actualDiscardedSamples)
			require.Equal(t, float64(expectedDiscardedExemplars), actualDiscardedExemplars)

			// Since each test pushes only 1 series, we do expect the ingester
			// to have received exactly 1 series
			for i := range ingesters {
				timeseries := ingesters[i].series()
				expectedSeries := 1
				if tc.expectedSeries.Len() == 0 {
					expectedSeries = 0
				}
				assert.Equal(t, expectedSeries, len(timeseries))
				for _, v := range timeseries {
					assert.Equal(t, tc.expectedSeries, cortexpb.FromLabelAdaptersToLabels(v.Labels))
				}
			}
		}
	}
}

func TestDistributor_Push_LabelRemoval_RemovingNameLabelWillError(t *testing.T) {
	t.Parallel()
	ctx := user.InjectOrgID(context.Background(), "user")
	type testcase struct {
		inputSeries    labels.Labels
		expectedSeries labels.Labels
		removeReplica  bool
		removeLabels   []string
	}

	tc := testcase{
		removeReplica:  true,
		removeLabels:   []string{"__name__"},
		inputSeries:    labels.FromStrings("__name__", "some_metric", "cluster", "one", "__replica__", "two"),
		expectedSeries: labels.Labels{},
	}

	var err error
	var limits validation.Limits
	flagext.DefaultValues(&limits)
	limits.DropLabels = tc.removeLabels
	limits.AcceptHASamples = tc.removeReplica

	ds, _, _, _ := prepare(t, prepConfig{
		numIngesters:     2,
		happyIngesters:   2,
		numDistributors:  1,
		shardByAllLabels: true,
		limits:           &limits,
	})

	// Push the series to the distributor
	req := mockWriteRequest([]labels.Labels{tc.inputSeries}, 1, 1, false)
	_, err = ds[0].Push(ctx, req)
	require.Error(t, err)
	assert.Equal(t, "rpc error: code = Code(400) desc = sample missing metric name", err.Error())
}

func TestDistributor_Push_ShouldGuaranteeShardingTokenConsistencyOverTheTime(t *testing.T) {
	t.Parallel()
	ctx := user.InjectOrgID(context.Background(), "user")
	tests := map[string]struct {
		inputSeries    labels.Labels
		expectedSeries labels.Labels
		expectedToken  uint32
	}{
		"metric_1 with value_1": {
			inputSeries: labels.FromStrings(
				"__name__", "metric_1",
				"cluster", "cluster_1",
				"key", "value_1",
			),
			expectedSeries: labels.FromStrings(
				"__name__", "metric_1",
				"cluster", "cluster_1",
				"key", "value_1",
			),
			expectedToken: 0xec0a2e9d,
		},

		"metric_1 with value_1 and dropped label due to config": {
			inputSeries: labels.FromStrings(
				"__name__", "metric_1",
				"cluster", "cluster_1",
				"key", "value_1",
				"dropped", "unused",
			),
			expectedSeries: labels.FromStrings(
				"__name__", "metric_1",
				"cluster", "cluster_1",
				"key", "value_1",
			),
			expectedToken: 0xec0a2e9d,
		},

		"metric_1 with value_1 and dropped HA replica label": {
			inputSeries: labels.FromStrings(
				"__name__", "metric_1",
				"cluster", "cluster_1",
				"key", "value_1",
				"__replica__", "replica_1",
			),
			expectedSeries: labels.FromStrings(
				"__name__", "metric_1",
				"cluster", "cluster_1",
				"key", "value_1",
			),
			expectedToken: 0xec0a2e9d,
		},

		"metric_2 with value_1": {
			inputSeries: labels.FromStrings(
				"__name__", "metric_2",
				"key", "value_1",
			),
			expectedSeries: labels.FromStrings(
				"__name__", "metric_2",
				"key", "value_1",
			),
			expectedToken: 0xa60906f2,
		},

		"metric_1 with value_2": {
			inputSeries: labels.FromStrings(
				"__name__", "metric_1",
				"key", "value_2",
			),
			expectedSeries: labels.FromStrings(
				"__name__", "metric_1",
				"key", "value_2",
			),
			expectedToken: 0x18abc8a2,
		},
	}

	var limits validation.Limits
	flagext.DefaultValues(&limits)
	limits.DropLabels = []string{"dropped"}
	limits.AcceptHASamples = true

	for testName, testData := range tests {
		testData := testData
		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			ds, ingesters, _, _ := prepare(t, prepConfig{
				numIngesters:     2,
				happyIngesters:   2,
				numDistributors:  1,
				shardByAllLabels: true,
				limits:           &limits,
			})

			// Push the series to the distributor
			req := mockWriteRequest([]labels.Labels{testData.inputSeries}, 1, 1, false)
			_, err := ds[0].Push(ctx, req)
			require.NoError(t, err)

			// Since each test pushes only 1 series, we do expect the ingester
			// to have received exactly 1 series
			for i := range ingesters {
				timeseries := ingesters[i].series()
				assert.Equal(t, 1, len(timeseries))

				series, ok := timeseries[testData.expectedToken]
				require.True(t, ok)
				assert.Equal(t, testData.expectedSeries, cortexpb.FromLabelAdaptersToLabels(series.Labels))
			}
		})
	}
}

func TestDistributor_Push_LabelNameValidation(t *testing.T) {
	t.Parallel()
	inputLabels := labels.FromStrings(model.MetricNameLabel, "foo", "999.illegal", "baz")
	ctx := user.InjectOrgID(context.Background(), "user")

	tests := map[string]struct {
		inputLabels                labels.Labels
		skipLabelNameValidationCfg bool
		skipLabelNameValidationReq bool
		errExpected                bool
		errMessage                 string
	}{
		"label name validation is on by default": {
			inputLabels: inputLabels,
			errExpected: true,
			errMessage:  `sample invalid label: "999.illegal" metric "foo{999.illegal=\"baz\"}"`,
		},
		"label name validation can be skipped via config": {
			inputLabels:                inputLabels,
			skipLabelNameValidationCfg: true,
			errExpected:                false,
		},
		"label name validation can be skipped via WriteRequest parameter": {
			inputLabels:                inputLabels,
			skipLabelNameValidationReq: true,
			errExpected:                false,
		},
	}

	for testName, tc := range tests {
		tc := tc
		for _, histogram := range []bool{true, false} {
			histogram := histogram
			t.Run(fmt.Sprintf("%s, histogram=%s", testName, strconv.FormatBool(histogram)), func(t *testing.T) {
				t.Parallel()
				ds, _, _, _ := prepare(t, prepConfig{
					numIngesters:            2,
					happyIngesters:          2,
					numDistributors:         1,
					shuffleShardSize:        1,
					skipLabelNameValidation: tc.skipLabelNameValidationCfg,
				})
				req := mockWriteRequest([]labels.Labels{tc.inputLabels}, 42, 100000, histogram)
				req.SkipLabelNameValidation = tc.skipLabelNameValidationReq
				_, err := ds[0].Push(ctx, req)
				if tc.errExpected {
					fromError, _ := status.FromError(err)
					assert.Equal(t, tc.errMessage, fromError.Message())
				} else {
					assert.Nil(t, err)
				}
			})
		}
	}
}

func TestDistributor_Push_ExemplarValidation(t *testing.T) {
	t.Parallel()
	ctx := user.InjectOrgID(context.Background(), "user")
	manyLabels := []string{model.MetricNameLabel, "test"}
	for i := 1; i < 31; i++ {
		manyLabels = append(manyLabels, fmt.Sprintf("name_%d", i), fmt.Sprintf("value_%d", i))
	}

	tests := map[string]struct {
		req    *cortexpb.WriteRequest
		errMsg string
	}{
		"valid exemplar": {
			req: makeWriteRequestExemplar([]string{model.MetricNameLabel, "test"}, 1000, []string{"foo", "bar"}),
		},
		"rejects exemplar with no labels": {
			req:    makeWriteRequestExemplar([]string{model.MetricNameLabel, "test"}, 1000, []string{}),
			errMsg: `exemplar missing labels, timestamp: 1000 series: {__name__="test"} labels: {}`,
		},
		"rejects exemplar with no timestamp": {
			req:    makeWriteRequestExemplar([]string{model.MetricNameLabel, "test"}, 0, []string{"foo", "bar"}),
			errMsg: `exemplar missing timestamp, timestamp: 0 series: {__name__="test"} labels: {foo="bar"}`,
		},
		"rejects exemplar with too long labelset": {
			req:    makeWriteRequestExemplar([]string{model.MetricNameLabel, "test"}, 1000, []string{"foo", strings.Repeat("0", 126)}),
			errMsg: fmt.Sprintf(`exemplar combined labelset exceeds 128 characters, timestamp: 1000 series: {__name__="test"} labels: {foo="%s"}`, strings.Repeat("0", 126)),
		},
		"rejects exemplar with too many series labels": {
			req:    makeWriteRequestExemplar(manyLabels, 0, nil),
			errMsg: "series has too many labels",
		},
		"rejects exemplar with duplicate series labels": {
			req:    makeWriteRequestExemplar([]string{model.MetricNameLabel, "test", "foo", "bar", "foo", "bar"}, 0, nil),
			errMsg: "duplicate label name",
		},
		"rejects exemplar with empty series label name": {
			req:    makeWriteRequestExemplar([]string{model.MetricNameLabel, "test", "", "bar"}, 0, nil),
			errMsg: "invalid label",
		},
	}

	for testName, tc := range tests {
		tc := tc
		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			ds, _, _, _ := prepare(t, prepConfig{
				numIngesters:     2,
				happyIngesters:   2,
				numDistributors:  1,
				shuffleShardSize: 1,
			})
			_, err := ds[0].Push(ctx, tc.req)
			if tc.errMsg != "" {
				fromError, _ := status.FromError(err)
				assert.Contains(t, fromError.Message(), tc.errMsg)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func BenchmarkDistributor_GetLabelsValues(b *testing.B) {
	ctx := user.InjectOrgID(context.Background(), "user")

	testCases := []struct {
		numIngesters            int
		lblValuesPerIngester    int
		lblValuesDuplicateRatio float64
	}{
		{
			numIngesters:            16,
			lblValuesPerIngester:    1000,
			lblValuesDuplicateRatio: 0.67, // Final Result will have 33% of the total size - replication factor of 3 and no duplicates
		},
		{
			numIngesters:            16,
			lblValuesPerIngester:    1000,
			lblValuesDuplicateRatio: 0.98,
		},
		{
			numIngesters:            150,
			lblValuesPerIngester:    1000,
			lblValuesDuplicateRatio: 0.67, // Final Result will have 33% of the total size - replication factor of 3 and no duplicates
		},
		{
			numIngesters:            150,
			lblValuesPerIngester:    1000,
			lblValuesDuplicateRatio: 0.98,
		},
		{
			numIngesters:            500,
			lblValuesPerIngester:    1000,
			lblValuesDuplicateRatio: 0.67, // Final Result will have 33% of the total size - replication factor of 3 and no duplicates
		},
		{
			numIngesters:            500,
			lblValuesPerIngester:    1000,
			lblValuesDuplicateRatio: 0.98,
		},
	}

	for _, tc := range testCases {
		name := fmt.Sprintf("numIngesters%v,lblValuesPerIngester%v,lblValuesDuplicateRatio%v", tc.numIngesters, tc.lblValuesPerIngester, tc.lblValuesDuplicateRatio)
		ds, _, _, _ := prepare(b, prepConfig{
			numIngesters:            tc.numIngesters,
			happyIngesters:          tc.numIngesters,
			numDistributors:         1,
			lblValuesPerIngester:    tc.lblValuesPerIngester,
			lblValuesDuplicateRatio: tc.lblValuesDuplicateRatio,
		})
		b.Run(name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := ds[0].LabelValuesForLabelName(ctx, model.Time(time.Now().UnixMilli()), model.Time(time.Now().UnixMilli()), "__name__", nil, false)
				require.NoError(b, err)
			}
		})
	}
}

func BenchmarkDistributor_Push(b *testing.B) {
	const (
		numSeriesPerRequest = 1000
	)
	ctx := user.InjectOrgID(context.Background(), "user")

	tests := map[string]struct {
		prepareConfig func(limits *validation.Limits)
		prepareSeries func() ([]labels.Labels, []cortexpb.Sample)
		expectedErr   string
	}{
		"all samples successfully pushed": {
			prepareConfig: func(limits *validation.Limits) {},
			prepareSeries: func() ([]labels.Labels, []cortexpb.Sample) {
				metrics := make([]labels.Labels, numSeriesPerRequest)
				samples := make([]cortexpb.Sample, numSeriesPerRequest)

				lbls := labels.NewBuilder(labels.FromStrings(model.MetricNameLabel, "foo"))
				for i := 0; i < numSeriesPerRequest; i++ {
					for i := 0; i < 10; i++ {
						lbls.Set(fmt.Sprintf("name_%d", i), fmt.Sprintf("value_%d", i))
					}

					metrics[i] = lbls.Labels()
					samples[i] = cortexpb.Sample{
						Value:       float64(i),
						TimestampMs: time.Now().UnixNano() / int64(time.Millisecond),
					}
				}

				return metrics, samples
			},
			expectedErr: "",
		},
		"ingestion rate limit reached": {
			prepareConfig: func(limits *validation.Limits) {
				limits.IngestionRate = 1
				limits.IngestionBurstSize = 1
			},
			prepareSeries: func() ([]labels.Labels, []cortexpb.Sample) {
				metrics := make([]labels.Labels, numSeriesPerRequest)
				samples := make([]cortexpb.Sample, numSeriesPerRequest)

				for i := 0; i < numSeriesPerRequest; i++ {
					lbls := labels.NewBuilder(labels.FromStrings(model.MetricNameLabel, "foo"))
					for i := 0; i < 10; i++ {
						lbls.Set(fmt.Sprintf("name_%d", i), fmt.Sprintf("value_%d", i))
					}

					metrics[i] = lbls.Labels()
					samples[i] = cortexpb.Sample{
						Value:       float64(i),
						TimestampMs: time.Now().UnixNano() / int64(time.Millisecond),
					}
				}

				return metrics, samples
			},
			expectedErr: "ingestion rate limit",
		},
		"too many labels limit reached": {
			prepareConfig: func(limits *validation.Limits) {
				limits.MaxLabelNamesPerSeries = 30
			},
			prepareSeries: func() ([]labels.Labels, []cortexpb.Sample) {
				metrics := make([]labels.Labels, numSeriesPerRequest)
				samples := make([]cortexpb.Sample, numSeriesPerRequest)

				for i := 0; i < numSeriesPerRequest; i++ {
					lbls := labels.NewBuilder(labels.FromStrings(model.MetricNameLabel, "foo"))
					for i := 1; i < 31; i++ {
						lbls.Set(fmt.Sprintf("name_%d", i), fmt.Sprintf("value_%d", i))
					}

					metrics[i] = lbls.Labels()
					samples[i] = cortexpb.Sample{
						Value:       float64(i),
						TimestampMs: time.Now().UnixNano() / int64(time.Millisecond),
					}
				}

				return metrics, samples
			},
			expectedErr: "series has too many labels",
		},
		"max label name length limit reached": {
			prepareConfig: func(limits *validation.Limits) {
				limits.MaxLabelNameLength = 1024
			},
			prepareSeries: func() ([]labels.Labels, []cortexpb.Sample) {
				metrics := make([]labels.Labels, numSeriesPerRequest)
				samples := make([]cortexpb.Sample, numSeriesPerRequest)

				for i := 0; i < numSeriesPerRequest; i++ {
					lbls := labels.NewBuilder(labels.FromStrings(model.MetricNameLabel, "foo"))
					for i := 0; i < 10; i++ {
						lbls.Set(fmt.Sprintf("name_%d", i), fmt.Sprintf("value_%d", i))
					}

					// Add a label with a very long name.
					lbls.Set(fmt.Sprintf("xxx_%0.2000d", 1), "xxx")

					metrics[i] = lbls.Labels()
					samples[i] = cortexpb.Sample{
						Value:       float64(i),
						TimestampMs: time.Now().UnixNano() / int64(time.Millisecond),
					}
				}

				return metrics, samples
			},
			expectedErr: "label name too long",
		},
		"max label value length limit reached": {
			prepareConfig: func(limits *validation.Limits) {
				limits.MaxLabelValueLength = 1024
			},
			prepareSeries: func() ([]labels.Labels, []cortexpb.Sample) {
				metrics := make([]labels.Labels, numSeriesPerRequest)
				samples := make([]cortexpb.Sample, numSeriesPerRequest)

				for i := 0; i < numSeriesPerRequest; i++ {
					lbls := labels.NewBuilder(labels.FromStrings(model.MetricNameLabel, "foo"))
					for i := 0; i < 10; i++ {
						lbls.Set(fmt.Sprintf("name_%d", i), fmt.Sprintf("value_%d", i))
					}

					// Add a label with a very long value.
					lbls.Set("xxx", fmt.Sprintf("xxx_%0.2000d", 1))

					metrics[i] = lbls.Labels()
					samples[i] = cortexpb.Sample{
						Value:       float64(i),
						TimestampMs: time.Now().UnixNano() / int64(time.Millisecond),
					}
				}

				return metrics, samples
			},
			expectedErr: "label value too long",
		},
		"max label size bytes per series limit reached": {
			prepareConfig: func(limits *validation.Limits) {
				limits.MaxLabelsSizeBytes = 1024
			},
			prepareSeries: func() ([]labels.Labels, []cortexpb.Sample) {
				metrics := make([]labels.Labels, numSeriesPerRequest)
				samples := make([]cortexpb.Sample, numSeriesPerRequest)

				for i := 0; i < numSeriesPerRequest; i++ {
					lbls := labels.NewBuilder(labels.FromStrings(model.MetricNameLabel, "foo"))
					for i := 0; i < 10; i++ {
						lbls.Set(fmt.Sprintf("name_%d", i), fmt.Sprintf("value_%d", i))
					}

					// Add a label with a very long value.
					lbls.Set("xxx", fmt.Sprintf("xxx_%0.2000d", 1))

					metrics[i] = lbls.Labels()
					samples[i] = cortexpb.Sample{
						Value:       float64(i),
						TimestampMs: time.Now().UnixNano() / int64(time.Millisecond),
					}
				}

				return metrics, samples
			},
			expectedErr: "labels size bytes exceeded",
		},
		"timestamp too old": {
			prepareConfig: func(limits *validation.Limits) {
				limits.RejectOldSamples = true
				limits.RejectOldSamplesMaxAge = model.Duration(time.Hour)
			},
			prepareSeries: func() ([]labels.Labels, []cortexpb.Sample) {
				metrics := make([]labels.Labels, numSeriesPerRequest)
				samples := make([]cortexpb.Sample, numSeriesPerRequest)

				for i := 0; i < numSeriesPerRequest; i++ {
					lbls := labels.NewBuilder(labels.FromStrings(model.MetricNameLabel, "foo"))
					for i := 0; i < 10; i++ {
						lbls.Set(fmt.Sprintf("name_%d", i), fmt.Sprintf("value_%d", i))
					}

					metrics[i] = lbls.Labels()
					samples[i] = cortexpb.Sample{
						Value:       float64(i),
						TimestampMs: time.Now().Add(-2*time.Hour).UnixNano() / int64(time.Millisecond),
					}
				}

				return metrics, samples
			},
			expectedErr: "timestamp too old",
		},
		"timestamp too new": {
			prepareConfig: func(limits *validation.Limits) {
				limits.CreationGracePeriod = model.Duration(time.Minute)
			},
			prepareSeries: func() ([]labels.Labels, []cortexpb.Sample) {
				metrics := make([]labels.Labels, numSeriesPerRequest)
				samples := make([]cortexpb.Sample, numSeriesPerRequest)

				for i := 0; i < numSeriesPerRequest; i++ {
					lbls := labels.NewBuilder(labels.FromStrings(model.MetricNameLabel, "foo"))
					for i := 0; i < 10; i++ {
						lbls.Set(fmt.Sprintf("name_%d", i), fmt.Sprintf("value_%d", i))
					}

					metrics[i] = lbls.Labels()
					samples[i] = cortexpb.Sample{
						Value:       float64(i),
						TimestampMs: time.Now().Add(time.Hour).UnixNano() / int64(time.Millisecond),
					}
				}

				return metrics, samples
			},
			expectedErr: "timestamp too new",
		},
	}

	tg := ring.NewRandomTokenGenerator()

	for testName, testData := range tests {
		b.Run(testName, func(b *testing.B) {

			// Create an in-memory KV store for the ring with 1 ingester registered.
			kvStore, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
			b.Cleanup(func() { assert.NoError(b, closer.Close()) })

			err := kvStore.CAS(context.Background(), ingester.RingKey,
				func(_ interface{}) (interface{}, bool, error) {
					d := &ring.Desc{}
					d.AddIngester("ingester-1", "127.0.0.1", "", tg.GenerateTokens(d, "ingester-1", "", 128, true), ring.ACTIVE, time.Now())
					return d, true, nil
				},
			)
			require.NoError(b, err)

			ingestersRing, err := ring.New(ring.Config{
				KVStore:           kv.Config{Mock: kvStore},
				HeartbeatTimeout:  60 * time.Minute,
				ReplicationFactor: 1,
			}, ingester.RingKey, ingester.RingKey, nil, nil)
			require.NoError(b, err)
			require.NoError(b, services.StartAndAwaitRunning(context.Background(), ingestersRing))
			b.Cleanup(func() {
				require.NoError(b, services.StopAndAwaitTerminated(context.Background(), ingestersRing))
			})

			test.Poll(b, time.Second, 1, func() interface{} {
				return ingestersRing.InstancesCount()
			})

			// Prepare the distributor configuration.
			var distributorCfg Config
			var clientConfig client.Config
			limits := validation.Limits{}
			flagext.DefaultValues(&distributorCfg, &clientConfig, &limits)

			limits.IngestionRate = 10000000 // Unlimited.
			testData.prepareConfig(&limits)

			distributorCfg.ShardByAllLabels = true
			distributorCfg.IngesterClientFactory = func(addr string) (ring_client.PoolClient, error) {
				return &noopIngester{}, nil
			}
			distributorCfg.UseStreamPush = false

			overrides := validation.NewOverrides(limits, nil)

			// Start the distributor.
			distributor, err := New(distributorCfg, clientConfig, overrides, ingestersRing, true, prometheus.NewRegistry(), log.NewNopLogger())
			require.NoError(b, err)
			require.NoError(b, services.StartAndAwaitRunning(context.Background(), distributor))

			b.Cleanup(func() {
				require.NoError(b, services.StopAndAwaitTerminated(context.Background(), distributor))
			})

			// Prepare the series to remote write before starting the benchmark.
			metrics, samples := testData.prepareSeries()

			// Run the benchmark.
			b.ReportAllocs()
			b.ResetTimer()

			for n := 0; n < b.N; n++ {
				_, err := distributor.Push(ctx, cortexpb.ToWriteRequest(metrics, samples, nil, nil, cortexpb.API))
				if testData.expectedErr == "" && err != nil {
					b.Fatalf("no error expected but got %v", err)
				}
				if testData.expectedErr != "" && (err == nil || !strings.Contains(err.Error(), testData.expectedErr)) {
					b.Fatalf("expected %v error but got %v", testData.expectedErr, err)
				}
			}
		})
	}
}

func TestSlowQueries(t *testing.T) {
	t.Parallel()
	ctx := user.InjectOrgID(context.Background(), "user")
	nameMatcher := mustEqualMatcher(model.MetricNameLabel, "foo")
	nIngesters := 3
	for _, shardByAllLabels := range []bool{true, false} {
		for happy := 0; happy <= nIngesters; happy++ {
			shardByAllLabels := shardByAllLabels
			happy := happy
			t.Run(fmt.Sprintf("%t/%d", shardByAllLabels, happy), func(t *testing.T) {
				t.Parallel()
				var expectedErr error
				if nIngesters-happy > 1 {
					expectedErr = errFail
				}

				ds, _, _, _ := prepare(t, prepConfig{
					numIngesters:     nIngesters,
					happyIngesters:   happy,
					numDistributors:  1,
					queryDelay:       100 * time.Millisecond,
					shardByAllLabels: shardByAllLabels,
				})

				_, err := ds[0].QueryStream(ctx, 0, 10, false, nameMatcher)
				assert.Equal(t, expectedErr, err)
			})
		}
	}
}

func TestDistributor_MetricsForLabelMatchers_SingleSlowIngester(t *testing.T) {
	t.Parallel()
	for _, histogram := range []bool{true, false} {
		// Create distributor
		ds, ing, _, _ := prepare(t, prepConfig{
			numIngesters:        3,
			happyIngesters:      3,
			numDistributors:     1,
			shardByAllLabels:    true,
			shuffleShardEnabled: true,
			shuffleShardSize:    3,
			replicationFactor:   3,
		})

		ing[2].queryDelay = 50 * time.Millisecond

		ctx := user.InjectOrgID(context.Background(), "test")

		now := model.Now()

		for i := 0; i < 100; i++ {

			req := mockWriteRequest([]labels.Labels{labels.FromStrings(labels.MetricName, "test", "app", "m", "uniq8", strconv.Itoa(i))}, 1, now.Unix(), histogram)
			_, err := ds[0].Push(ctx, req)
			require.NoError(t, err)
		}

		for i := 0; i < 50; i++ {
			_, err := ds[0].MetricsForLabelMatchers(ctx, now, now, nil, false, mustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"))
			require.NoError(t, err)
		}
	}
}

func TestDistributor_MetricsForLabelMatchers(t *testing.T) {
	t.Parallel()
	const numIngesters = 5

	fixtures := []struct {
		lbls      labels.Labels
		value     int64
		timestamp int64
	}{
		{
			lbls:      labels.FromStrings(labels.MetricName, "test_1", "status", "200"),
			value:     1,
			timestamp: 100000,
		},
		{
			lbls:      labels.FromStrings(labels.MetricName, "test_1", "status", "500"),
			value:     1,
			timestamp: 110000,
		},
		{
			lbls:      labels.FromStrings(labels.MetricName, "test_2"),
			value:     2,
			timestamp: 200000,
		},
		// The two following series have the same FastFingerprint=e002a3a451262627
		{
			lbls:      labels.FromStrings(labels.MetricName, "fast_fingerprint_collision", "app", "l", "uniq0", "0", "uniq1", "1"),
			value:     1,
			timestamp: 300000,
		},
		{
			lbls:      labels.FromStrings(labels.MetricName, "fast_fingerprint_collision", "app", "m", "uniq0", "1", "uniq1", "1"),
			value:     1,
			timestamp: 300000,
		},
	}

	tests := map[string]struct {
		shuffleShardEnabled bool
		shuffleShardSize    int
		matchers            []*labels.Matcher
		expectedResult      []labels.Labels
		expectedIngesters   int
		queryLimiter        *limiter.QueryLimiter
		expectedErr         error
	}{
		"should return an empty response if no metric match": {
			matchers: []*labels.Matcher{
				mustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "unknown"),
			},
			expectedResult:    []labels.Labels{},
			expectedIngesters: numIngesters,
			queryLimiter:      limiter.NewQueryLimiter(0, 0, 0, 0),
			expectedErr:       nil,
		},
		"should filter metrics by single matcher": {
			matchers: []*labels.Matcher{
				mustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test_1"),
			},
			expectedResult: []labels.Labels{
				fixtures[0].lbls,
				fixtures[1].lbls,
			},
			expectedIngesters: numIngesters,
			queryLimiter:      limiter.NewQueryLimiter(0, 0, 0, 0),
			expectedErr:       nil,
		},
		"should filter metrics by multiple matchers": {
			matchers: []*labels.Matcher{
				mustNewMatcher(labels.MatchEqual, "status", "200"),
				mustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test_1"),
			},
			expectedResult: []labels.Labels{
				fixtures[0].lbls,
			},
			expectedIngesters: numIngesters,
			queryLimiter:      limiter.NewQueryLimiter(0, 0, 0, 0),
			expectedErr:       nil,
		},
		"should return all matching metrics even if their FastFingerprint collide": {
			matchers: []*labels.Matcher{
				mustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "fast_fingerprint_collision"),
			},
			expectedResult: []labels.Labels{
				fixtures[3].lbls,
				fixtures[4].lbls,
			},
			expectedIngesters: numIngesters,
			queryLimiter:      limiter.NewQueryLimiter(0, 0, 0, 0),
			expectedErr:       nil,
		},
		"should query only ingesters belonging to tenant's subring if shuffle sharding is enabled": {
			shuffleShardEnabled: true,
			shuffleShardSize:    3,
			matchers: []*labels.Matcher{
				mustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test_1"),
			},
			expectedResult: []labels.Labels{
				fixtures[0].lbls,
				fixtures[1].lbls,
			},
			expectedIngesters: 3,
			queryLimiter:      limiter.NewQueryLimiter(0, 0, 0, 0),
			expectedErr:       nil,
		},
		"should query all ingesters if shuffle sharding is enabled but shard size is 0": {
			shuffleShardEnabled: true,
			shuffleShardSize:    0,
			matchers: []*labels.Matcher{
				mustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test_1"),
			},
			expectedResult: []labels.Labels{
				fixtures[0].lbls,
				fixtures[1].lbls,
			},
			expectedIngesters: numIngesters,
			queryLimiter:      limiter.NewQueryLimiter(0, 0, 0, 0),
			expectedErr:       nil,
		},
		"should return err if series limit is exhausted": {
			shuffleShardEnabled: true,
			shuffleShardSize:    0,
			matchers: []*labels.Matcher{
				mustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test_1"),
			},
			expectedResult:    nil,
			expectedIngesters: numIngesters,
			queryLimiter:      limiter.NewQueryLimiter(1, 0, 0, 0),
			expectedErr:       validation.LimitError(fmt.Sprintf(limiter.ErrMaxSeriesHit, 1)),
		},
		"should return err if data bytes limit is exhausted": {
			shuffleShardEnabled: true,
			shuffleShardSize:    0,
			matchers: []*labels.Matcher{
				mustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test_1"),
			},
			expectedResult:    nil,
			expectedIngesters: numIngesters,
			queryLimiter:      limiter.NewQueryLimiter(0, 0, 0, 1),
			expectedErr:       validation.LimitError(fmt.Sprintf(limiter.ErrMaxDataBytesHit, 1)),
		},
		"should not exhaust series limit when only one series is fetched": {
			matchers: []*labels.Matcher{
				mustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test_2"),
			},
			expectedResult: []labels.Labels{
				fixtures[2].lbls,
			},
			expectedIngesters: numIngesters,
			queryLimiter:      limiter.NewQueryLimiter(1, 0, 0, 0),
			expectedErr:       nil,
		},
	}

	for testName, testData := range tests {
		testData := testData
		for _, histogram := range []bool{true, false} {
			histogram := histogram
			t.Run(fmt.Sprintf("%s, histogram=%s", testName, strconv.FormatBool(histogram)), func(t *testing.T) {
				t.Parallel()
				now := model.Now()

				// Create distributor
				ds, ingesters, _, _ := prepare(t, prepConfig{
					numIngesters:        numIngesters,
					happyIngesters:      numIngesters,
					numDistributors:     1,
					shardByAllLabels:    true,
					shuffleShardEnabled: testData.shuffleShardEnabled,
					shuffleShardSize:    testData.shuffleShardSize,
				})

				// Push fixtures
				ctx := user.InjectOrgID(context.Background(), "test")
				ctx = limiter.AddQueryLimiterToContext(ctx, testData.queryLimiter)

				for _, series := range fixtures {
					req := mockWriteRequest([]labels.Labels{series.lbls}, series.value, series.timestamp, histogram)
					_, err := ds[0].Push(ctx, req)
					require.NoError(t, err)
				}

				{
					metrics, err := ds[0].MetricsForLabelMatchers(ctx, now, now, nil, false, testData.matchers...)

					if testData.expectedErr != nil {
						assert.ErrorIs(t, err, testData.expectedErr)
						return
					}

					require.NoError(t, err)
					assert.ElementsMatch(t, testData.expectedResult, metrics)

					// Check how many ingesters have been queried.
					// Due to the quorum the distributor could cancel the last request towards ingesters
					// if all other ones are successful, so we're good either has been queried X or X-1
					// ingesters.
					assert.Contains(t, []int{testData.expectedIngesters, testData.expectedIngesters - 1}, countMockIngestersCalls(ingesters, "MetricsForLabelMatchers"))
				}

				{
					metrics, err := ds[0].MetricsForLabelMatchersStream(ctx, now, now, nil, false, testData.matchers...)
					if testData.expectedErr != nil {
						assert.ErrorIs(t, err, testData.expectedErr)
						return
					}

					require.NoError(t, err)
					assert.ElementsMatch(t, testData.expectedResult, metrics)

					assert.Contains(t, []int{testData.expectedIngesters, testData.expectedIngesters - 1}, countMockIngestersCalls(ingesters, "MetricsForLabelMatchersStream"))
				}
			})
		}
	}
}

func BenchmarkDistributor_MetricsForLabelMatchers(b *testing.B) {
	const (
		numIngesters        = 100
		numSeriesPerRequest = 100
	)

	tests := map[string]struct {
		prepareConfig func(limits *validation.Limits)
		prepareSeries func() ([]labels.Labels, []cortexpb.Sample)
		matchers      []*labels.Matcher
		queryLimiter  *limiter.QueryLimiter
		expectedErr   error
	}{
		"get series within limits": {
			prepareConfig: func(limits *validation.Limits) {},
			prepareSeries: func() ([]labels.Labels, []cortexpb.Sample) {
				metrics := make([]labels.Labels, numSeriesPerRequest)
				samples := make([]cortexpb.Sample, numSeriesPerRequest)

				for i := 0; i < numSeriesPerRequest; i++ {
					lbls := labels.NewBuilder(labels.FromStrings(model.MetricNameLabel, fmt.Sprintf("foo_%d", i)))
					for i := 0; i < 10; i++ {
						lbls.Set(fmt.Sprintf("name_%d", i), fmt.Sprintf("value_%d", i))
					}

					metrics[i] = lbls.Labels()
					samples[i] = cortexpb.Sample{
						Value:       float64(i),
						TimestampMs: time.Now().UnixNano() / int64(time.Millisecond),
					}
				}

				return metrics, samples
			},
			matchers: []*labels.Matcher{
				mustNewMatcher(labels.MatchRegexp, model.MetricNameLabel, "foo.+"),
			},
			queryLimiter: limiter.NewQueryLimiter(100, 0, 0, 0),
			expectedErr:  nil,
		},
	}

	for testName, testData := range tests {
		b.Run(testName, func(b *testing.B) {
			// Create distributor
			ds, ingesters, _, _ := prepare(b, prepConfig{
				numIngesters:        numIngesters,
				happyIngesters:      numIngesters,
				numDistributors:     1,
				shardByAllLabels:    true,
				shuffleShardEnabled: false,
				shuffleShardSize:    0,
			})

			// Push fixtures
			ctx := user.InjectOrgID(context.Background(), "test")
			ctx = limiter.AddQueryLimiterToContext(ctx, testData.queryLimiter)

			// Prepare the series to remote write before starting the benchmark.
			metrics, samples := testData.prepareSeries()

			if _, err := ds[0].Push(ctx, cortexpb.ToWriteRequest(metrics, samples, nil, nil, cortexpb.API)); err != nil {
				b.Fatalf("error pushing to distributor %v", err)
			}

			// Run the benchmark.
			b.ReportAllocs()
			b.ResetTimer()

			for n := 0; n < b.N; n++ {
				now := model.Now()
				metrics, err := ds[0].MetricsForLabelMatchers(ctx, now, now, nil, false, testData.matchers...)

				if testData.expectedErr != nil {
					assert.EqualError(b, err, testData.expectedErr.Error())
					return
				}

				require.NoError(b, err)

				// Check how many ingesters have been queried.
				// Due to the quorum the distributor could cancel the last request towards ingesters
				// if all other ones are successful, so we're good either has been queried X or X-1
				// ingesters.
				assert.Contains(b, []int{numIngesters, numIngesters - 1}, countMockIngestersCalls(ingesters, "MetricsForLabelMatchers"))
				assert.Equal(b, numSeriesPerRequest, len(metrics))
			}
		})
	}
}

func TestDistributor_MetricsMetadata(t *testing.T) {
	t.Parallel()
	const numIngesters = 5

	tests := map[string]struct {
		shuffleShardEnabled bool
		shuffleShardSize    int
		expectedIngesters   int
	}{
		"should query all ingesters if shuffle sharding is disabled": {
			shuffleShardEnabled: false,
			expectedIngesters:   numIngesters,
		},
		"should query all ingesters if shuffle sharding is enabled but shard size is 0": {
			shuffleShardEnabled: true,
			shuffleShardSize:    0,
			expectedIngesters:   numIngesters,
		},
		"should query only ingesters belonging to tenant's subring if shuffle sharding is enabled": {
			shuffleShardEnabled: true,
			shuffleShardSize:    3,
			expectedIngesters:   3,
		},
	}

	for testName, testData := range tests {
		testData := testData
		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			// Create distributor
			ds, ingesters, _, _ := prepare(t, prepConfig{
				numIngesters:        numIngesters,
				happyIngesters:      numIngesters,
				numDistributors:     1,
				shardByAllLabels:    true,
				shuffleShardEnabled: testData.shuffleShardEnabled,
				shuffleShardSize:    testData.shuffleShardSize,
				limits:              nil,
			})

			// Push metadata
			ctx := user.InjectOrgID(context.Background(), "test")

			req := makeWriteRequest(0, 0, 10, 0)
			_, err := ds[0].Push(ctx, req)
			require.NoError(t, err)

			// Assert on metric metadata
			metadata, err := ds[0].MetricsMetadata(ctx, &client.MetricsMetadataRequest{Limit: -1, LimitPerMetric: -1, Metric: ""})
			require.NoError(t, err)
			assert.Equal(t, 10, len(metadata))

			// Check how many ingesters have been queried.
			// Due to the quorum the distributor could cancel the last request towards ingesters
			// if all other ones are successful, so we're good either has been queried X or X-1
			// ingesters.
			assert.Contains(t, []int{testData.expectedIngesters, testData.expectedIngesters - 1}, countMockIngestersCalls(ingesters, "MetricsMetadata"))
		})
	}
}

func mustNewMatcher(t labels.MatchType, n, v string) *labels.Matcher {
	m, err := labels.NewMatcher(t, n, v)
	if err != nil {
		panic(err)
	}

	return m
}

func mockWriteRequest(lbls []labels.Labels, value int64, timestampMs int64, histogram bool) *cortexpb.WriteRequest {
	var (
		samples    []cortexpb.Sample
		histograms []cortexpb.Histogram
	)
	if histogram {
		histograms = make([]cortexpb.Histogram, len(lbls))
		for i := range lbls {
			histograms[i] = cortexpb.HistogramToHistogramProto(timestampMs, tsdbutil.GenerateTestHistogram(value))
		}
	} else {
		samples = make([]cortexpb.Sample, len(lbls))
		for i := range lbls {
			samples[i] = cortexpb.Sample{
				TimestampMs: timestampMs,
				Value:       float64(value),
			}
		}
	}

	return cortexpb.ToWriteRequest(lbls, samples, nil, histograms, cortexpb.API)
}

type prepConfig struct {
	numIngesters, happyIngesters int
	queryDelay                   time.Duration
	shardByAllLabels             bool
	shuffleShardEnabled          bool
	shuffleShardSize             int
	lblValuesPerIngester         int
	lblValuesDuplicateRatio      float64
	limits                       *validation.Limits
	numDistributors              int
	skipLabelNameValidation      bool
	maxInflightRequests          int
	maxInflightClientRequests    int
	maxIngestionRate             float64
	replicationFactor            int
	enableTracker                bool
	errFail                      error
	tokens                       [][]uint32
	useStreamPush                bool
}

type prepState struct {
	unusedStrings, usedStrings []string
}

func prepare(tb testing.TB, cfg prepConfig) ([]*Distributor, []*mockIngester, []*prometheus.Registry, *ring.Ring) {
	// Strings to be used for get labels values/Names
	var unusedStrings []string
	if cfg.lblValuesPerIngester > 0 {
		unusedStrings = make([]string, min(len(randomStrings), cfg.numIngesters*cfg.lblValuesPerIngester))
		copy(unusedStrings, randomStrings)
	}
	s := &prepState{
		unusedStrings: unusedStrings,
	}
	ingesters := []*mockIngester{}
	for i := 0; i < cfg.happyIngesters; i++ {
		ingesters = append(ingesters, newMockIngester(i, s, cfg))
	}
	for i := cfg.happyIngesters; i < cfg.numIngesters; i++ {
		miError := errFail
		if cfg.errFail != nil {
			miError = cfg.errFail
		}

		ingesters = append(ingesters, &mockIngester{
			queryDelay: cfg.queryDelay,
			failResp:   *atomic.NewError(miError),
		})
	}

	// Use a real ring with a mock KV store to test ring RF logic.
	ingesterDescs := map[string]ring.InstanceDesc{}
	ingestersByAddr := map[string]*mockIngester{}
	for i := range ingesters {
		var tokens []uint32
		if len(cfg.tokens) > i {
			tokens = cfg.tokens[i]
		} else {
			tokens = []uint32{uint32((math.MaxUint32 / cfg.numIngesters) * i)}
		}
		ingester := fmt.Sprintf("ingester-%d", i)
		addr := fmt.Sprintf("ip-ingester-%d", i)
		ingesterDescs[ingester] = ring.InstanceDesc{
			Addr:                addr,
			Zone:                "",
			State:               ring.ACTIVE,
			Timestamp:           time.Now().Unix(),
			RegisteredTimestamp: time.Now().Add(-2 * time.Hour).Unix(),
			Tokens:              tokens,
		}
		ingestersByAddr[addr] = ingesters[i]
	}

	kvStore, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	tb.Cleanup(func() { assert.NoError(tb, closer.Close()) })

	err := kvStore.CAS(context.Background(), ingester.RingKey,
		func(_ interface{}) (interface{}, bool, error) {
			return &ring.Desc{
				Ingesters: ingesterDescs,
			}, true, nil
		},
	)
	require.NoError(tb, err)

	// Use a default replication factor of 3 if there isn't a provided replication factor.
	rf := cfg.replicationFactor
	if rf == 0 {
		rf = 3
	}

	ingestersRing, err := ring.New(ring.Config{
		KVStore: kv.Config{
			Mock: kvStore,
		},
		HeartbeatTimeout:  60 * time.Minute,
		ReplicationFactor: rf,
	}, ingester.RingKey, ingester.RingKey, nil, nil)
	require.NoError(tb, err)
	require.NoError(tb, services.StartAndAwaitRunning(context.Background(), ingestersRing))

	test.Poll(tb, time.Second, cfg.numIngesters, func() interface{} {
		return ingestersRing.InstancesCount()
	})

	factory := func(addr string) (ring_client.PoolClient, error) {
		return ingestersByAddr[addr], nil
	}

	distributors := make([]*Distributor, 0, cfg.numDistributors)
	registries := make([]*prometheus.Registry, 0, cfg.numDistributors)
	for i := 0; i < cfg.numDistributors; i++ {
		if cfg.limits == nil {
			cfg.limits = &validation.Limits{}
			flagext.DefaultValues(cfg.limits)
		}

		var distributorCfg Config
		var clientConfig client.Config
		flagext.DefaultValues(&distributorCfg, &clientConfig)

		distributorCfg.IngesterClientFactory = factory
		distributorCfg.ShardByAllLabels = cfg.shardByAllLabels
		distributorCfg.ExtraQueryDelay = 50 * time.Millisecond
		distributorCfg.DistributorRing.HeartbeatPeriod = 100 * time.Millisecond
		distributorCfg.DistributorRing.InstanceID = strconv.Itoa(i)
		distributorCfg.DistributorRing.KVStore.Mock = kvStore
		distributorCfg.DistributorRing.InstanceAddr = "127.0.0.1"
		distributorCfg.SkipLabelNameValidation = cfg.skipLabelNameValidation
		distributorCfg.InstanceLimits.MaxInflightPushRequests = cfg.maxInflightRequests
		distributorCfg.InstanceLimits.MaxInflightClientRequests = cfg.maxInflightClientRequests
		distributorCfg.InstanceLimits.MaxIngestionRate = cfg.maxIngestionRate
		distributorCfg.UseStreamPush = cfg.useStreamPush

		if cfg.shuffleShardEnabled {
			distributorCfg.ShardingStrategy = util.ShardingStrategyShuffle
			distributorCfg.ShuffleShardingLookbackPeriod = time.Hour

			cfg.limits.IngestionTenantShardSize = cfg.shuffleShardSize
		}

		if cfg.enableTracker {
			codec := ha.GetReplicaDescCodec()
			ringStore, closer := consul.NewInMemoryClient(codec, log.NewNopLogger(), nil)
			tb.Cleanup(func() { assert.NoError(tb, closer.Close()) })
			mock := kv.PrefixClient(ringStore, "prefix")
			distributorCfg.HATrackerConfig = HATrackerConfig{
				EnableHATracker: true,
				KVStore:         kv.Config{Mock: mock},
				UpdateTimeout:   100 * time.Millisecond,
				FailoverTimeout: time.Hour,
			}
			cfg.limits.HAMaxClusters = 100
		}

		overrides := validation.NewOverrides(*cfg.limits, nil)

		reg := prometheus.NewPedanticRegistry()
		d, err := New(distributorCfg, clientConfig, overrides, ingestersRing, true, reg, log.NewNopLogger())
		require.NoError(tb, err)
		require.NoError(tb, services.StartAndAwaitRunning(context.Background(), d))

		distributors = append(distributors, d)
		registries = append(registries, reg)
	}

	// If the distributors ring is setup, wait until the first distributor
	// updates to the expected size
	if distributors[0].distributorsRing != nil {
		test.Poll(tb, time.Second, cfg.numDistributors, func() interface{} {
			return distributors[0].distributorsLifeCycler.HealthyInstancesCount()
		})
	}

	tb.Cleanup(func() { stopAll(distributors, ingestersRing) })

	return distributors, ingesters, registries, ingestersRing
}

func stopAll(ds []*Distributor, r *ring.Ring) {
	for _, d := range ds {
		services.StopAndAwaitTerminated(context.Background(), d) //nolint:errcheck
	}

	// Mock consul doesn't stop quickly, so don't wait.
	r.StopAsync()
}

func makeWriteRequest(startTimestampMs int64, samples int, metadata int, histograms int) *cortexpb.WriteRequest {
	request := &cortexpb.WriteRequest{}
	for i := 0; i < samples; i++ {
		request.Timeseries = append(request.Timeseries, makeWriteRequestTimeseries(
			[]cortexpb.LabelAdapter{
				{Name: model.MetricNameLabel, Value: "foo"},
				{Name: "bar", Value: "baz"},
				{Name: "sample", Value: fmt.Sprintf("%d", i)},
			}, startTimestampMs+int64(i), int64(i), false))
	}

	for i := 0; i < histograms; i++ {
		request.Timeseries = append(request.Timeseries, makeWriteRequestTimeseries(
			[]cortexpb.LabelAdapter{
				{Name: model.MetricNameLabel, Value: "foo"},
				{Name: "bar", Value: "baz"},
				{Name: "histogram", Value: fmt.Sprintf("%d", i)},
			}, startTimestampMs+int64(i), int64(i), true))
	}

	for i := 0; i < metadata; i++ {
		m := &cortexpb.MetricMetadata{
			MetricFamilyName: fmt.Sprintf("metric_%d", i),
			Type:             cortexpb.COUNTER,
			Help:             fmt.Sprintf("a help for metric_%d", i),
		}
		request.Metadata = append(request.Metadata, m)
	}

	return request
}

func makeWriteRequestTimeseries(labels []cortexpb.LabelAdapter, ts, value int64, histogram bool) cortexpb.PreallocTimeseries {
	t := cortexpb.PreallocTimeseries{
		TimeSeries: &cortexpb.TimeSeries{
			Labels: labels,
		},
	}
	if histogram {
		t.Histograms = append(t.Histograms, cortexpb.HistogramToHistogramProto(ts, tsdbutil.GenerateTestHistogram(value)))
	} else {
		t.Samples = append(t.Samples, cortexpb.Sample{
			TimestampMs: ts,
			Value:       float64(value),
		})
	}
	return t
}

func makeWriteRequestHA(samples int, replica, cluster string, histogram bool) *cortexpb.WriteRequest {
	request := &cortexpb.WriteRequest{}
	for i := 0; i < samples; i++ {
		ts := cortexpb.PreallocTimeseries{
			TimeSeries: &cortexpb.TimeSeries{
				Labels: []cortexpb.LabelAdapter{
					{Name: "__name__", Value: "foo"},
					{Name: "__replica__", Value: replica},
					{Name: "bar", Value: "baz"},
					{Name: "cluster", Value: cluster},
					{Name: "sample", Value: fmt.Sprintf("%d", i)},
				},
			},
		}
		if histogram {
			ts.Histograms = []cortexpb.Histogram{
				cortexpb.HistogramToHistogramProto(int64(i), tsdbutil.GenerateTestHistogram(int64(i))),
			}
		} else {
			ts.Samples = []cortexpb.Sample{
				{
					Value:       float64(i),
					TimestampMs: int64(i),
				},
			}
		}
		request.Timeseries = append(request.Timeseries, ts)
	}
	return request
}

func makeWriteRequestHAMixedSamples(samples int, histogram bool) *cortexpb.WriteRequest {
	request := &cortexpb.WriteRequest{}

	for _, haPair := range []struct {
		cluster string
		replica string
	}{
		{
			cluster: "cluster0",
			replica: "replica0",
		},
		{
			cluster: "cluster0",
			replica: "replica1",
		},
		{
			cluster: "cluster1",
			replica: "replica0",
		},
		{
			cluster: "cluster1",
			replica: "replica1",
		},
		{
			cluster: "",
			replica: "replicaNoCluster",
		},
		{
			cluster: "clusterNoReplica",
			replica: "",
		},
		{
			cluster: "",
			replica: "",
		},
	} {
		cluster := haPair.cluster
		replica := haPair.replica
		var ts cortexpb.PreallocTimeseries
		if cluster == "" && replica == "" {
			ts = cortexpb.PreallocTimeseries{
				TimeSeries: &cortexpb.TimeSeries{
					Labels: []cortexpb.LabelAdapter{
						{Name: "__name__", Value: "foo"},
						{Name: "bar", Value: "baz"},
					},
				},
			}
		} else if cluster == "" && replica != "" {
			ts = cortexpb.PreallocTimeseries{
				TimeSeries: &cortexpb.TimeSeries{
					Labels: []cortexpb.LabelAdapter{
						{Name: "__name__", Value: "foo"},
						{Name: "__replica__", Value: replica},
						{Name: "bar", Value: "baz"},
					},
				},
			}
		} else if cluster != "" && replica == "" {
			ts = cortexpb.PreallocTimeseries{
				TimeSeries: &cortexpb.TimeSeries{
					Labels: []cortexpb.LabelAdapter{
						{Name: "__name__", Value: "foo"},
						{Name: "bar", Value: "baz"},
						{Name: "cluster", Value: cluster},
					},
				},
			}
		} else {
			ts = cortexpb.PreallocTimeseries{
				TimeSeries: &cortexpb.TimeSeries{
					Labels: []cortexpb.LabelAdapter{
						{Name: "__name__", Value: "foo"},
						{Name: "__replica__", Value: replica},
						{Name: "bar", Value: "baz"},
						{Name: "cluster", Value: cluster},
					},
				},
			}
		}
		if histogram {
			ts.Histograms = []cortexpb.Histogram{
				cortexpb.HistogramToHistogramProto(int64(samples), tsdbutil.GenerateTestHistogram(int64(samples))),
			}
		} else {
			var s = make([]cortexpb.Sample, 0)
			for i := 0; i < samples; i++ {
				sample := cortexpb.Sample{
					Value:       float64(i),
					TimestampMs: int64(i),
				}
				s = append(s, sample)
			}
			ts.Samples = s
		}
		request.Timeseries = append(request.Timeseries, ts)
	}
	return request
}

func makeWriteRequestExemplar(seriesLabels []string, timestamp int64, exemplarLabels []string) *cortexpb.WriteRequest {
	return &cortexpb.WriteRequest{
		Timeseries: []cortexpb.PreallocTimeseries{
			{
				TimeSeries: &cortexpb.TimeSeries{
					// Labels: []cortexpb.LabelAdapter{{Name: model.MetricNameLabel, Value: "test"}},
					Labels: cortexpb.FromLabelsToLabelAdapters(labels.FromStrings(seriesLabels...)),
					Exemplars: []cortexpb.Exemplar{
						{
							Labels:      cortexpb.FromLabelsToLabelAdapters(labels.FromStrings(exemplarLabels...)),
							TimestampMs: timestamp,
						},
					},
				},
			},
		},
	}
}

func expectedResponse(start, end int) model.Matrix {
	result := model.Matrix{}
	for i := start; i < end; i++ {
		result = append(result, &model.SampleStream{
			Metric: model.Metric{
				model.MetricNameLabel: "foo",
				"bar":                 "baz",
				"sample":              model.LabelValue(fmt.Sprintf("%d", i)),
			},
			Values: []model.SamplePair{
				{
					Value:     model.SampleValue(i),
					Timestamp: model.Time(i),
				},
			},
		})
	}
	return result
}

func mustEqualMatcher(k, v string) *labels.Matcher {
	m, err := labels.NewMatcher(labels.MatchEqual, k, v)
	if err != nil {
		panic(err)
	}
	return m
}

type mockIngester struct {
	sync.Mutex
	client.IngesterClient
	grpc_health_v1.HealthClient
	happy      atomic.Bool
	failResp   atomic.Error
	stats      client.UsersStatsResponse
	timeseries map[uint32]*cortexpb.PreallocTimeseries
	metadata   map[uint32]map[cortexpb.MetricMetadata]struct{}
	queryDelay time.Duration
	calls      map[string]int
	lblsValues []string
}

func newMockIngester(id int, ps *prepState, cfg prepConfig) *mockIngester {
	lblsValues := make([]string, 0, cfg.lblValuesPerIngester)
	usedStrings := make([]string, len(ps.usedStrings))
	copy(usedStrings, ps.usedStrings)

	for i := 0; i < cfg.lblValuesPerIngester; i++ {
		var s string
		if i < int(float64(cfg.lblValuesPerIngester)*cfg.lblValuesDuplicateRatio) && id > 0 {
			index := rand.Int() % len(usedStrings)
			s = usedStrings[index]
			usedStrings = append(usedStrings[:index], usedStrings[index+1:]...)
		} else {
			s = ps.unusedStrings[0]
			ps.usedStrings = append(ps.usedStrings, s)
			ps.unusedStrings = ps.unusedStrings[1:]
		}
		lblsValues = append(lblsValues, s)
	}
	sort.Strings(lblsValues)
	return &mockIngester{
		happy:      *atomic.NewBool(true),
		queryDelay: cfg.queryDelay,
		lblsValues: lblsValues,
	}
}

func (i *mockIngester) series() map[uint32]*cortexpb.PreallocTimeseries {
	i.Lock()
	defer i.Unlock()

	result := map[uint32]*cortexpb.PreallocTimeseries{}
	for k, v := range i.timeseries {
		result[k] = v
	}
	return result
}

func (i *mockIngester) Check(ctx context.Context, in *grpc_health_v1.HealthCheckRequest, opts ...grpc.CallOption) (*grpc_health_v1.HealthCheckResponse, error) {
	i.Lock()
	defer i.Unlock()

	i.trackCall("Check")

	return &grpc_health_v1.HealthCheckResponse{}, nil
}

func (i *mockIngester) Close() error {
	return nil
}

func (i *mockIngester) LabelValues(_ context.Context, _ *client.LabelValuesRequest, _ ...grpc.CallOption) (*client.LabelValuesResponse, error) {
	return &client.LabelValuesResponse{
		LabelValues: i.lblsValues,
	}, nil
}

func (i *mockIngester) PushPreAlloc(ctx context.Context, in *cortexpb.PreallocWriteRequest, opts ...grpc.CallOption) (*cortexpb.WriteResponse, error) {
	return i.Push(ctx, &in.WriteRequest, opts...)
}

func (i *mockIngester) PushStreamConnection(ctx context.Context, in *cortexpb.WriteRequest, opts ...grpc.CallOption) (*cortexpb.WriteResponse, error) {
	return i.Push(ctx, in, opts...)
}

func (i *mockIngester) Push(ctx context.Context, req *cortexpb.WriteRequest, opts ...grpc.CallOption) (*cortexpb.WriteResponse, error) {
	i.Lock()
	defer i.Unlock()

	i.trackCall("Push")

	if !i.happy.Load() {
		return nil, i.failResp.Load()
	}

	if i.timeseries == nil {
		i.timeseries = map[uint32]*cortexpb.PreallocTimeseries{}
	}

	if i.metadata == nil {
		i.metadata = map[uint32]map[cortexpb.MetricMetadata]struct{}{}
	}

	orgid, err := tenant.TenantID(ctx)
	if err != nil {
		return nil, err
	}

	for j := range req.Timeseries {
		series := req.Timeseries[j]
		hash := shardByAllLabels(orgid, series.Labels)
		existing, ok := i.timeseries[hash]
		if !ok {
			// Make a copy because the request Timeseries are reused
			item := cortexpb.TimeSeries{
				Labels:  make([]cortexpb.LabelAdapter, len(series.Labels)),
				Samples: make([]cortexpb.Sample, len(series.Samples)),
			}

			copy(item.Labels, series.Labels)
			copy(item.Samples, series.Samples)

			i.timeseries[hash] = &cortexpb.PreallocTimeseries{TimeSeries: &item}
		} else {
			existing.Samples = append(existing.Samples, series.Samples...)
		}
	}

	for _, m := range req.Metadata {
		hash := shardByMetricName(orgid, m.MetricFamilyName)
		set, ok := i.metadata[hash]
		if !ok {
			set = map[cortexpb.MetricMetadata]struct{}{}
			i.metadata[hash] = set
		}
		set[*m] = struct{}{}
	}

	return &cortexpb.WriteResponse{}, nil
}

func (i *mockIngester) Query(ctx context.Context, req *client.QueryRequest, opts ...grpc.CallOption) (*client.QueryResponse, error) {
	time.Sleep(i.queryDelay)

	i.Lock()
	defer i.Unlock()

	i.trackCall("Query")

	if !i.happy.Load() {
		return nil, errFail
	}

	_, _, matchers, err := client.FromQueryRequest(storecache.NoopMatchersCache, req)
	if err != nil {
		return nil, err
	}

	response := client.QueryResponse{}
	for _, ts := range i.timeseries {
		if match(ts.Labels, matchers) {
			response.Timeseries = append(response.Timeseries, *ts.TimeSeries)
		}
	}
	return &response, nil
}

func (i *mockIngester) QueryStream(ctx context.Context, req *client.QueryRequest, opts ...grpc.CallOption) (client.Ingester_QueryStreamClient, error) {
	time.Sleep(i.queryDelay)

	i.Lock()
	defer i.Unlock()

	i.trackCall("QueryStream")

	if !i.happy.Load() {
		return nil, errFail
	}

	_, _, matchers, err := client.FromQueryRequest(storecache.NoopMatchersCache, req)
	if err != nil {
		return nil, err
	}

	results := []*client.QueryStreamResponse{}
	for _, ts := range i.timeseries {
		if !match(ts.Labels, matchers) {
			continue
		}

		c := chunkenc.NewXORChunk()
		appender, err := c.Appender()
		if err != nil {
			return nil, err
		}
		chunks := []chunkenc.Chunk{c}
		for _, sample := range ts.Samples {
			appender.Append(sample.TimestampMs, sample.Value)
		}

		wireChunks := []client.Chunk{}
		for _, c := range chunks {
			e, err := promchunk.FromPromChunkEncoding(c.Encoding())
			if err != nil {
				return nil, err
			}
			chunk := client.Chunk{
				Encoding: int32(e),
				Data:     c.Bytes(),
			}
			wireChunks = append(wireChunks, chunk)
		}

		results = append(results, &client.QueryStreamResponse{
			Chunkseries: []client.TimeSeriesChunk{
				{
					Labels: ts.Labels,
					Chunks: wireChunks,
				},
			},
		})
	}
	return &queryStream{
		results: results,
	}, nil
}

func (i *mockIngester) MetricsForLabelMatchersStream(ctx context.Context, req *client.MetricsForLabelMatchersRequest, opts ...grpc.CallOption) (client.Ingester_MetricsForLabelMatchersStreamClient, error) {
	time.Sleep(i.queryDelay)
	i.Lock()
	defer i.Unlock()

	i.trackCall("MetricsForLabelMatchersStream")

	if !i.happy.Load() {
		return nil, errFail
	}

	_, _, _, multiMatchers, err := client.FromMetricsForLabelMatchersRequest(storecache.NoopMatchersCache, req)
	if err != nil {
		return nil, err
	}

	results := []*client.MetricsForLabelMatchersStreamResponse{}
	for _, matchers := range multiMatchers {
		for _, ts := range i.timeseries {
			if match(ts.Labels, matchers) {
				results = append(results, &client.MetricsForLabelMatchersStreamResponse{
					Metric: []*cortexpb.Metric{{Labels: ts.Labels}},
				})
			}
		}
	}

	return &metricsForLabelMatchersStream{
		results: results,
	}, nil
}

func (i *mockIngester) MetricsForLabelMatchers(ctx context.Context, req *client.MetricsForLabelMatchersRequest, opts ...grpc.CallOption) (*client.MetricsForLabelMatchersResponse, error) {
	time.Sleep(i.queryDelay)
	i.Lock()
	defer i.Unlock()

	i.trackCall("MetricsForLabelMatchers")

	if !i.happy.Load() {
		return nil, errFail
	}

	_, _, _, multiMatchers, err := client.FromMetricsForLabelMatchersRequest(storecache.NoopMatchersCache, req)
	if err != nil {
		return nil, err
	}

	response := client.MetricsForLabelMatchersResponse{}
	for _, matchers := range multiMatchers {
		for _, ts := range i.timeseries {
			if match(ts.Labels, matchers) {
				response.Metric = append(response.Metric, &cortexpb.Metric{Labels: ts.Labels})
			}
		}
	}
	return &response, nil
}

func (i *mockIngester) MetricsMetadata(ctx context.Context, req *client.MetricsMetadataRequest, opts ...grpc.CallOption) (*client.MetricsMetadataResponse, error) {
	i.Lock()
	defer i.Unlock()

	i.trackCall("MetricsMetadata")

	if !i.happy.Load() {
		return nil, errFail
	}

	resp := &client.MetricsMetadataResponse{}
	for _, sets := range i.metadata {
		for m := range sets {
			resp.Metadata = append(resp.Metadata, &m)
		}
	}

	return resp, nil
}

func (i *mockIngester) trackCall(name string) {
	if i.calls == nil {
		i.calls = map[string]int{}
	}

	i.calls[name]++
}

func (i *mockIngester) countCalls(name string) int {
	i.Lock()
	defer i.Unlock()

	return i.calls[name]
}

// noopIngester is a mocked ingester which does nothing.
type noopIngester struct {
	client.IngesterClient
	grpc_health_v1.HealthClient
}

func (i *noopIngester) Close() error {
	return nil
}

func (i *noopIngester) PushPreAlloc(ctx context.Context, in *cortexpb.PreallocWriteRequest, opts ...grpc.CallOption) (*cortexpb.WriteResponse, error) {
	return nil, nil
}

func (i *noopIngester) Push(ctx context.Context, req *cortexpb.WriteRequest, opts ...grpc.CallOption) (*cortexpb.WriteResponse, error) {
	return nil, nil
}

func (i *noopIngester) PushStreamConnection(ctx context.Context, in *cortexpb.WriteRequest, opts ...grpc.CallOption) (*cortexpb.WriteResponse, error) {
	return nil, nil
}

type queryStream struct {
	grpc.ClientStream
	i       int
	results []*client.QueryStreamResponse
}

func (*queryStream) CloseSend() error {
	return nil
}

func (s *queryStream) Recv() (*client.QueryStreamResponse, error) {
	if s.i >= len(s.results) {
		return nil, io.EOF
	}
	result := s.results[s.i]
	s.i++
	return result, nil
}

type metricsForLabelMatchersStream struct {
	grpc.ClientStream
	i       int
	results []*client.MetricsForLabelMatchersStreamResponse
}

func (*metricsForLabelMatchersStream) CloseSend() error {
	return nil
}

func (s *metricsForLabelMatchersStream) Recv() (*client.MetricsForLabelMatchersStreamResponse, error) {
	if s.i >= len(s.results) {
		return nil, io.EOF
	}
	result := s.results[s.i]
	s.i++
	return result, nil
}

func (i *mockIngester) AllUserStats(ctx context.Context, in *client.UserStatsRequest, opts ...grpc.CallOption) (*client.UsersStatsResponse, error) {
	return &i.stats, nil
}

func match(labels []cortexpb.LabelAdapter, matchers []*labels.Matcher) bool {
outer:
	for _, matcher := range matchers {
		for _, labels := range labels {
			if matcher.Name == labels.Name && matcher.Matches(labels.Value) {
				continue outer
			}
		}
		return false
	}
	return true
}

func TestDistributorValidation(t *testing.T) {
	t.Parallel()
	ctx := user.InjectOrgID(context.Background(), "1")
	now := model.Now()
	future, past := now.Add(5*time.Hour), now.Add(-25*time.Hour)
	testHistogram := tsdbutil.GenerateTestHistogram(1)
	testFloatHistogram := tsdbutil.GenerateTestFloatHistogram(1)

	for i, tc := range []struct {
		metadata   []*cortexpb.MetricMetadata
		labels     []labels.Labels
		samples    []cortexpb.Sample
		histograms []cortexpb.Histogram
		err        error
	}{
		// Test validation passes.
		{
			metadata: []*cortexpb.MetricMetadata{{MetricFamilyName: "testmetric", Help: "a test metric.", Unit: "", Type: cortexpb.COUNTER}},
			labels: []labels.Labels{
				labels.FromStrings(labels.MetricName, "testmetric", "foo", "bar"),
			},
			samples: []cortexpb.Sample{{
				TimestampMs: int64(now),
				Value:       1,
			}},
			histograms: []cortexpb.Histogram{
				cortexpb.HistogramToHistogramProto(int64(now), testHistogram),
			},
		},
		// Test validation fails for very old samples.
		{
			labels: []labels.Labels{
				labels.FromStrings(labels.MetricName, "testmetric", "foo", "bar"),
			},
			samples: []cortexpb.Sample{{
				TimestampMs: int64(past),
				Value:       2,
			}},
			err: httpgrpc.Errorf(http.StatusBadRequest, `timestamp too old: %d metric: "testmetric"`, past),
		},
		// Test validation fails for samples from the future.
		{
			labels: []labels.Labels{
				labels.FromStrings(labels.MetricName, "testmetric", "foo", "bar"),
			},
			samples: []cortexpb.Sample{{
				TimestampMs: int64(future),
				Value:       4,
			}},
			err: httpgrpc.Errorf(http.StatusBadRequest, `timestamp too new: %d metric: "testmetric"`, future),
		},

		// Test maximum labels names per series.
		{
			labels: []labels.Labels{
				labels.FromStrings(labels.MetricName, "testmetric", "foo", "bar", "foo2", "bar2"),
			},
			samples: []cortexpb.Sample{{
				TimestampMs: int64(now),
				Value:       2,
			}},
			err: httpgrpc.Errorf(http.StatusBadRequest, `series has too many labels (actual: 3, limit: 2) series: 'testmetric{foo2="bar2", foo="bar"}'`),
		},
		// Test multiple validation fails return the first one.
		{
			labels: []labels.Labels{
				labels.FromStrings(labels.MetricName, "testmetric", "foo", "bar", "foo2", "bar2"),
				labels.FromStrings(labels.MetricName, "testmetric", "foo", "bar"),
			},
			samples: []cortexpb.Sample{
				{TimestampMs: int64(now), Value: 2},
				{TimestampMs: int64(past), Value: 2},
			},
			err: httpgrpc.Errorf(http.StatusBadRequest, `series has too many labels (actual: 3, limit: 2) series: 'testmetric{foo2="bar2", foo="bar"}'`),
		},
		// Test metadata validation fails
		{
			metadata: []*cortexpb.MetricMetadata{{MetricFamilyName: "", Help: "a test metric.", Unit: "", Type: cortexpb.COUNTER}},
			labels: []labels.Labels{
				labels.FromStrings(labels.MetricName, "testmetric", "foo", "bar"),
			},
			samples: []cortexpb.Sample{{
				TimestampMs: int64(now),
				Value:       1,
			}},
			err: httpgrpc.Errorf(http.StatusBadRequest, `metadata missing metric name`),
		},
		// Test maximum labels names per series for histogram samples.
		{
			labels: []labels.Labels{
				labels.FromStrings(labels.MetricName, "testmetric", "foo", "bar", "foo2", "bar2"),
			},
			histograms: []cortexpb.Histogram{
				cortexpb.HistogramToHistogramProto(int64(now), testHistogram),
			},
			err: httpgrpc.Errorf(http.StatusBadRequest, `series has too many labels (actual: 3, limit: 2) series: 'testmetric{foo2="bar2", foo="bar"}'`),
		},
		// Test validation fails for very old histogram samples.
		{
			labels: []labels.Labels{
				labels.FromStrings(labels.MetricName, "testmetric", "foo", "bar"),
			},
			histograms: []cortexpb.Histogram{
				cortexpb.HistogramToHistogramProto(int64(past), testHistogram),
			},
			err: httpgrpc.Errorf(http.StatusBadRequest, `timestamp too old: %d metric: "testmetric"`, past),
		},
		// Test validation fails for histogram samples from the future.
		{
			labels: []labels.Labels{
				labels.FromStrings(labels.MetricName, "testmetric", "foo", "bar"),
			},
			histograms: []cortexpb.Histogram{
				cortexpb.FloatHistogramToHistogramProto(int64(future), testFloatHistogram),
			},
			err: httpgrpc.Errorf(http.StatusBadRequest, `timestamp too new: %d metric: "testmetric"`, future),
		},
	} {
		tc := tc
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			var limits validation.Limits
			flagext.DefaultValues(&limits)

			limits.CreationGracePeriod = model.Duration(2 * time.Hour)
			limits.RejectOldSamples = true
			limits.RejectOldSamplesMaxAge = model.Duration(24 * time.Hour)
			limits.MaxLabelNamesPerSeries = 2

			ds, _, _, _ := prepare(t, prepConfig{
				numIngesters:     3,
				happyIngesters:   3,
				numDistributors:  1,
				shardByAllLabels: true,
				limits:           &limits,
			})

			_, err := ds[0].Push(ctx, cortexpb.ToWriteRequest(tc.labels, tc.samples, tc.metadata, tc.histograms, cortexpb.API))
			require.Equal(t, tc.err, err)
		})
	}
}

func TestRemoveReplicaLabel(t *testing.T) {
	t.Parallel()
	replicaLabel := "replica"
	clusterLabel := "cluster"
	cases := []struct {
		labelsIn  []cortexpb.LabelAdapter
		labelsOut []cortexpb.LabelAdapter
	}{
		// Replica label is present
		{
			labelsIn: []cortexpb.LabelAdapter{
				{Name: "__name__", Value: "foo"},
				{Name: "bar", Value: "baz"},
				{Name: "sample", Value: "1"},
				{Name: "replica", Value: replicaLabel},
			},
			labelsOut: []cortexpb.LabelAdapter{
				{Name: "__name__", Value: "foo"},
				{Name: "bar", Value: "baz"},
				{Name: "sample", Value: "1"},
			},
		},
		// Replica label is not present
		{
			labelsIn: []cortexpb.LabelAdapter{
				{Name: "__name__", Value: "foo"},
				{Name: "bar", Value: "baz"},
				{Name: "sample", Value: "1"},
				{Name: "cluster", Value: clusterLabel},
			},
			labelsOut: []cortexpb.LabelAdapter{
				{Name: "__name__", Value: "foo"},
				{Name: "bar", Value: "baz"},
				{Name: "sample", Value: "1"},
				{Name: "cluster", Value: clusterLabel},
			},
		},
	}

	for _, c := range cases {
		removeLabel(replicaLabel, &c.labelsIn)
		assert.Equal(t, c.labelsOut, c.labelsIn)
	}
}

// This is not great, but we deal with unsorted labels when validating labels.
func TestShardByAllLabelsReturnsWrongResultsForUnsortedLabels(t *testing.T) {
	t.Parallel()
	val1 := shardByAllLabels("test", []cortexpb.LabelAdapter{
		{Name: "__name__", Value: "foo"},
		{Name: "bar", Value: "baz"},
		{Name: "sample", Value: "1"},
	})

	val2 := shardByAllLabels("test", []cortexpb.LabelAdapter{
		{Name: "__name__", Value: "foo"},
		{Name: "sample", Value: "1"},
		{Name: "bar", Value: "baz"},
	})

	assert.NotEqual(t, val1, val2)
}

func TestSortLabels(t *testing.T) {
	t.Parallel()
	sorted := []cortexpb.LabelAdapter{
		{Name: "__name__", Value: "foo"},
		{Name: "bar", Value: "baz"},
		{Name: "cluster", Value: "cluster"},
		{Name: "sample", Value: "1"},
	}

	// no allocations if input is already sorted
	require.Equal(t, 0.0, testing.AllocsPerRun(100, func() {
		sortLabelsIfNeeded(sorted)
	}))

	unsorted := []cortexpb.LabelAdapter{
		{Name: "__name__", Value: "foo"},
		{Name: "sample", Value: "1"},
		{Name: "cluster", Value: "cluster"},
		{Name: "bar", Value: "baz"},
	}

	sortLabelsIfNeeded(unsorted)

	sort.SliceIsSorted(unsorted, func(i, j int) bool {
		return strings.Compare(unsorted[i].Name, unsorted[j].Name) < 0
	})
}

func TestDistributor_Push_Relabel(t *testing.T) {
	t.Parallel()
	ctx := user.InjectOrgID(context.Background(), "userDistributorPushRelabel")

	type testcase struct {
		name                 string
		inputSeries          []labels.Labels
		expectedSeries       labels.Labels
		metricRelabelConfigs []*relabel.Config
	}

	cases := []testcase{
		{
			name: "with no relabel config",
			inputSeries: []labels.Labels{
				labels.FromStrings("__name__", "foo", "cluster", "one"),
			},
			expectedSeries: labels.FromStrings("__name__", "foo", "cluster", "one"),
		},
		{
			name: "with hardcoded replace",
			inputSeries: []labels.Labels{
				labels.FromStrings("__name__", "foo", "cluster", "one"),
			},
			expectedSeries: labels.FromStrings("__name__", "foo", "cluster", "two"),
			metricRelabelConfigs: []*relabel.Config{
				{
					SourceLabels: []model.LabelName{"cluster"},
					Action:       relabel.DefaultRelabelConfig.Action,
					Regex:        relabel.DefaultRelabelConfig.Regex,
					TargetLabel:  "cluster",
					Replacement:  "two",
				},
			},
		},
		{
			name: "with drop action",
			inputSeries: []labels.Labels{
				labels.FromStrings("__name__", "foo", "cluster", "one"),
				labels.FromStrings("__name__", "bar", "cluster", "two"),
			},
			expectedSeries: labels.FromStrings("__name__", "bar", "cluster", "two"),
			metricRelabelConfigs: []*relabel.Config{
				{
					SourceLabels: []model.LabelName{"__name__"},
					Action:       relabel.Drop,
					Regex:        relabel.MustNewRegexp("(foo)"),
				},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		for _, enableHistogram := range []bool{false, true} {
			enableHistogram := enableHistogram
			t.Run(fmt.Sprintf("%s, histogram=%s", tc.name, strconv.FormatBool(enableHistogram)), func(t *testing.T) {
				t.Parallel()
				var err error
				var limits validation.Limits
				flagext.DefaultValues(&limits)
				limits.MetricRelabelConfigs = tc.metricRelabelConfigs

				ds, ingesters, _, _ := prepare(t, prepConfig{
					numIngesters:     2,
					happyIngesters:   2,
					numDistributors:  1,
					shardByAllLabels: true,
					limits:           &limits,
				})

				// Push the series to the distributor
				req := mockWriteRequest(tc.inputSeries, 1, 1, enableHistogram)
				_, err = ds[0].Push(ctx, req)
				require.NoError(t, err)

				// Since each test pushes only 1 series, we do expect the ingester
				// to have received exactly 1 series
				for i := range ingesters {
					timeseries := ingesters[i].series()
					assert.Equal(t, 1, len(timeseries))
					for _, v := range timeseries {
						assert.Equal(t, tc.expectedSeries, cortexpb.FromLabelAdaptersToLabels(v.Labels))
					}
				}
			})
		}
	}
}

func TestDistributor_Push_EmptyLabel(t *testing.T) {
	t.Parallel()
	ctx := user.InjectOrgID(context.Background(), "pushEmptyLabel")
	type testcase struct {
		name           string
		inputSeries    []labels.Labels
		expectedSeries labels.Labels
	}

	cases := []testcase{
		{
			name: "with empty label",
			inputSeries: []labels.Labels{
				labels.FromStrings("__name__", "foo", "empty", ""),
				labels.FromStrings("__name__", "foo", "changHash", ""),
			},
			expectedSeries: labels.FromStrings("__name__", "foo"),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var err error
			var limits validation.Limits
			flagext.DefaultValues(&limits)

			token := [][]uint32{
				{1},
				{2},
				{3},
				{1106054333},
				{5},
				{6},
				{7},
				{8},
				{9},
				{3827924125},
			}

			ds, ingesters, _, _ := prepare(t, prepConfig{
				numIngesters:      10,
				happyIngesters:    10,
				numDistributors:   1,
				shardByAllLabels:  true,
				limits:            &limits,
				replicationFactor: 1,
				shuffleShardSize:  10,
				tokens:            token,
			})

			// Push the series to the distributor
			req := mockWriteRequest(tc.inputSeries, 1, 1, false)
			_, err = ds[0].Push(ctx, req)
			require.NoError(t, err)

			// Since each test pushes only 1 series, we do expect the ingester
			// to have received exactly 1 series
			ingesterWithSeries := 0
			for i := range ingesters {
				timeseries := ingesters[i].series()
				if len(timeseries) > 0 {
					ingesterWithSeries++
				}
			}
			assert.Equal(t, 1, ingesterWithSeries)
		})
	}
}

func TestDistributor_Push_RelabelDropWillExportMetricOfDroppedSamples(t *testing.T) {
	t.Parallel()
	metricRelabelConfigs := []*relabel.Config{
		{
			SourceLabels: []model.LabelName{"__name__"},
			Action:       relabel.Drop,
			Regex:        relabel.MustNewRegexp("(foo)"),
		},
	}

	inputSeries := []labels.Labels{
		labels.FromStrings("__name__", "foo", "cluster", "one"),
		labels.FromStrings("__name__", "bar", "cluster", "two"),
	}

	var err error
	var limits validation.Limits
	flagext.DefaultValues(&limits)
	limits.MetricRelabelConfigs = metricRelabelConfigs

	for _, histogramEnabled := range []bool{false, true} {
		ds, ingesters, _, _ := prepare(t, prepConfig{
			numIngesters:     2,
			happyIngesters:   2,
			numDistributors:  1,
			shardByAllLabels: true,
			limits:           &limits,
		})

		// Push the series to the distributor
		id := "user"
		req := mockWriteRequest(inputSeries, 1, 1, histogramEnabled)
		req.Timeseries[0].Exemplars = []cortexpb.Exemplar{
			{Labels: cortexpb.FromLabelsToLabelAdapters(labels.FromStrings("test", "a")), Value: 1, TimestampMs: 0},
			{Labels: cortexpb.FromLabelsToLabelAdapters(labels.FromStrings("test", "b")), Value: 1, TimestampMs: 0},
		}
		ctx := user.InjectOrgID(context.Background(), id)
		_, err = ds[0].Push(ctx, req)
		require.NoError(t, err)

		for i := range ingesters {
			timeseries := ingesters[i].series()
			assert.Equal(t, 1, len(timeseries))
		}

		require.Equal(t, testutil.ToFloat64(ds[0].validateMetrics.DiscardedSamples.WithLabelValues(validation.DroppedByRelabelConfiguration, id)), float64(1))
		require.Equal(t, testutil.ToFloat64(ds[0].validateMetrics.DiscardedExemplars.WithLabelValues(validation.DroppedByRelabelConfiguration, id)), float64(2))
		receivedFloatSamples := testutil.ToFloat64(ds[0].receivedSamples.WithLabelValues(id, "float"))
		receivedHistogramSamples := testutil.ToFloat64(ds[0].receivedSamples.WithLabelValues(id, "histogram"))
		if histogramEnabled {
			require.Equal(t, receivedFloatSamples, float64(0))
			require.Equal(t, receivedHistogramSamples, float64(1))
		} else {
			require.Equal(t, receivedFloatSamples, float64(1))
			require.Equal(t, receivedHistogramSamples, float64(0))
		}
	}
}

func TestDistributor_PushLabelSetMetrics(t *testing.T) {
	t.Parallel()
	inputSeries := []labels.Labels{
		labels.FromStrings("__name__", "foo", "cluster", "one"),
		labels.FromStrings("__name__", "bar", "cluster", "one"),
		labels.FromStrings("__name__", "bar", "cluster", "two"),
		labels.FromStrings("__name__", "foo", "cluster", "three"),
	}

	var err error
	var limits validation.Limits
	flagext.DefaultValues(&limits)
	limits.LimitsPerLabelSet = []validation.LimitsPerLabelSet{
		{Hash: 0, LabelSet: labels.FromStrings("cluster", "one")},
		{Hash: 1, LabelSet: labels.FromStrings("cluster", "two")},
		{Hash: 2, LabelSet: labels.EmptyLabels()},
	}

	ds, _, regs, _ := prepare(t, prepConfig{
		numIngesters:     2,
		happyIngesters:   2,
		numDistributors:  1,
		shardByAllLabels: true,
		limits:           &limits,
	})
	reg := regs[0]

	// Push the series to the distributor
	id := "user"
	req := mockWriteRequest(inputSeries, 1, 1, false)
	ctx := user.InjectOrgID(context.Background(), id)
	_, err = ds[0].Push(ctx, req)
	require.NoError(t, err)

	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
		# HELP cortex_distributor_received_samples_per_labelset_total The total number of received samples per label set, excluding rejected and deduped samples.
		# TYPE cortex_distributor_received_samples_per_labelset_total counter
		cortex_distributor_received_samples_per_labelset_total{labelset="{cluster=\"one\"}",type="float",user="user"} 2
		cortex_distributor_received_samples_per_labelset_total{labelset="{cluster=\"two\"}",type="float",user="user"} 1
		cortex_distributor_received_samples_per_labelset_total{labelset="{}",type="float",user="user"} 1
		`), "cortex_distributor_received_samples_per_labelset_total"))

	// Push more series.
	inputSeries = []labels.Labels{
		labels.FromStrings("__name__", "baz", "cluster", "two"),
		labels.FromStrings("__name__", "foo", "cluster", "four"),
	}
	// Write the same request twice for different users.
	req = mockWriteRequest(inputSeries, 1, 1, false)
	ctx2 := user.InjectOrgID(context.Background(), "user2")
	_, err = ds[0].Push(ctx, req)
	require.NoError(t, err)
	req = mockWriteRequest(inputSeries, 1, 1, false)
	_, err = ds[0].Push(ctx2, req)
	require.NoError(t, err)
	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
		# HELP cortex_distributor_received_samples_per_labelset_total The total number of received samples per label set, excluding rejected and deduped samples.
		# TYPE cortex_distributor_received_samples_per_labelset_total counter
		cortex_distributor_received_samples_per_labelset_total{labelset="{cluster=\"one\"}",type="float",user="user"} 2
		cortex_distributor_received_samples_per_labelset_total{labelset="{cluster=\"two\"}",type="float",user="user"} 2
		cortex_distributor_received_samples_per_labelset_total{labelset="{cluster=\"two\"}",type="float",user="user2"} 1
		cortex_distributor_received_samples_per_labelset_total{labelset="{}",type="float",user="user"} 2
		cortex_distributor_received_samples_per_labelset_total{labelset="{}",type="float",user="user2"} 1
		`), "cortex_distributor_received_samples_per_labelset_total"))

	// Remove existing limits and add new limits
	limits.LimitsPerLabelSet = []validation.LimitsPerLabelSet{
		{Hash: 3, LabelSet: labels.FromStrings("cluster", "three")},
		{Hash: 4, LabelSet: labels.FromStrings("cluster", "four")},
		{Hash: 2, LabelSet: labels.EmptyLabels()},
	}
	ds[0].limits = validation.NewOverrides(limits, nil)
	ds[0].updateLabelSetMetrics()
	// Old label set metrics are removed. New label set metrics will be added when
	// new requests come in.
	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
		# HELP cortex_distributor_received_samples_per_labelset_total The total number of received samples per label set, excluding rejected and deduped samples.
		# TYPE cortex_distributor_received_samples_per_labelset_total counter
		cortex_distributor_received_samples_per_labelset_total{labelset="{}",type="float",user="user"} 2
		cortex_distributor_received_samples_per_labelset_total{labelset="{}",type="float",user="user2"} 1
		`), "cortex_distributor_received_samples_per_labelset_total"))

	// Metrics from `user` got removed but `user2` metric should remain.
	ds[0].cleanupInactiveUser(id)
	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
		# HELP cortex_distributor_received_samples_per_labelset_total The total number of received samples per label set, excluding rejected and deduped samples.
		# TYPE cortex_distributor_received_samples_per_labelset_total counter
		cortex_distributor_received_samples_per_labelset_total{labelset="{}",type="float",user="user2"} 1
		`), "cortex_distributor_received_samples_per_labelset_total"))
}

func countMockIngestersCalls(ingesters []*mockIngester, name string) int {
	count := 0
	for i := 0; i < len(ingesters); i++ {
		if ingesters[i].countCalls(name) > 0 {
			count++
		}
	}
	return count
}

func TestFindHALabels(t *testing.T) {
	t.Parallel()
	replicaLabel, clusterLabel := "replica", "cluster"
	type expectedOutput struct {
		cluster string
		replica string
	}
	cases := []struct {
		labelsIn []cortexpb.LabelAdapter
		expected expectedOutput
	}{
		{
			[]cortexpb.LabelAdapter{
				{Name: "__name__", Value: "foo"},
				{Name: "bar", Value: "baz"},
				{Name: "sample", Value: "1"},
				{Name: replicaLabel, Value: "1"},
			},
			expectedOutput{cluster: "", replica: "1"},
		},
		{
			[]cortexpb.LabelAdapter{
				{Name: "__name__", Value: "foo"},
				{Name: "bar", Value: "baz"},
				{Name: "sample", Value: "1"},
				{Name: clusterLabel, Value: "cluster-2"},
			},
			expectedOutput{cluster: "cluster-2", replica: ""},
		},
		{
			[]cortexpb.LabelAdapter{
				{Name: "__name__", Value: "foo"},
				{Name: "bar", Value: "baz"},
				{Name: "sample", Value: "1"},
				{Name: replicaLabel, Value: "3"},
				{Name: clusterLabel, Value: "cluster-3"},
			},
			expectedOutput{cluster: "cluster-3", replica: "3"},
		},
	}

	for _, c := range cases {
		cluster, replica := findHALabels(replicaLabel, clusterLabel, c.labelsIn)
		assert.Equal(t, c.expected.cluster, cluster)
		assert.Equal(t, c.expected.replica, replica)
	}
}
