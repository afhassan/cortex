package storegateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/gogo/status"
	"github.com/oklog/ulid/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
	"github.com/thanos-io/thanos/pkg/block"
	thanos_metadata "github.com/thanos-io/thanos/pkg/block/metadata"
	"github.com/thanos-io/thanos/pkg/store"
	"github.com/thanos-io/thanos/pkg/store/labelpb"
	"github.com/thanos-io/thanos/pkg/store/storepb"
	"github.com/weaveworks/common/logging"
	"go.uber.org/atomic"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"github.com/cortexproject/cortex/pkg/storage/bucket"
	"github.com/cortexproject/cortex/pkg/storage/bucket/filesystem"
	cortex_tsdb "github.com/cortexproject/cortex/pkg/storage/tsdb"
	"github.com/cortexproject/cortex/pkg/storage/tsdb/bucketindex"
	cortex_testutil "github.com/cortexproject/cortex/pkg/storage/tsdb/testutil"
	"github.com/cortexproject/cortex/pkg/tenant"
	"github.com/cortexproject/cortex/pkg/util"
	"github.com/cortexproject/cortex/pkg/util/flagext"
)

func TestBucketStores_CustomerKeyError(t *testing.T) {
	userToMetric := map[string]string{
		"user-1": "series",
		"user-2": "series",
	}

	ctx := context.Background()
	cfg := prepareStorageConfig(t)
	cfg.BucketStore.BucketIndex.Enabled = true

	storageDir := t.TempDir()

	for userID, metricName := range userToMetric {
		generateStorageBlock(t, storageDir, userID, metricName, 10, 100, 15)
	}

	b, err := filesystem.NewBucketClient(filesystem.Config{Directory: storageDir})

	bucketIndexes := map[string]*bucketindex.Index{}
	// Generate Bucket Index
	for userID := range userToMetric {
		idx := &bucketindex.Index{
			Version:   bucketindex.IndexVersion1,
			UpdatedAt: time.Now().Unix(),
		}
		err := b.Iter(ctx, userID, func(s string) error {
			if id, isBlock := block.IsBlockDir(s); isBlock {
				metaFile := path.Join(userID, id.String(), block.MetaFilename)
				r, err := b.Get(ctx, metaFile)
				require.NoError(t, err)
				metaContent, err := io.ReadAll(r)
				require.NoError(t, err)
				// Unmarshal it.
				m := thanos_metadata.Meta{}
				if err := json.Unmarshal(metaContent, &m); err != nil {
					require.NoError(t, err)
				}

				idx.Blocks = append(idx.Blocks, bucketindex.BlockFromThanosMeta(m))
			}
			return nil
		})

		require.NoError(t, err)
		require.NoError(t, bucketindex.WriteIndex(ctx, b, userID, nil, idx))
		bucketIndexes[userID] = idx
	}

	cases := map[string]struct {
		mockInitialSync bool
		GetFailures     map[string]error
	}{
		"should return ResourceExhausted when fail to get bucket index": {
			mockInitialSync: true,
			GetFailures: map[string]error{
				"user-1/bucket-index.json.gz": cortex_testutil.ErrKeyAccessDeniedError,
			},
		},
		"should return ResourceExhausted when fail to block index": {
			mockInitialSync: false,
			GetFailures: map[string]error{
				"user-1/" + bucketIndexes["user-1"].Blocks[0].ID.String() + "/index": cortex_testutil.ErrKeyAccessDeniedError,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			mBucket := &cortex_testutil.MockBucketFailure{
				Bucket: b,
			}
			require.NoError(t, err)

			reg := prometheus.NewPedanticRegistry()
			stores, err := NewBucketStores(cfg, NewNoShardingStrategy(log.NewNopLogger(), nil), mBucket, defaultLimitsOverrides(t), mockLoggingLevel(), log.NewNopLogger(), reg)
			require.NoError(t, err)

			if tc.mockInitialSync {
				mBucket.GetFailures = tc.GetFailures
			}

			// Should set the error on user-1
			require.NoError(t, stores.InitialSync(ctx))
			if tc.mockInitialSync {
				s, ok := status.FromError(stores.storesErrors["user-1"])
				require.True(t, ok)
				require.Equal(t, s.Code(), codes.PermissionDenied)
				require.ErrorIs(t, stores.storesErrors["user-2"], nil)
			}
			require.NoError(t, stores.SyncBlocks(context.Background()))
			if tc.mockInitialSync {
				s, ok := status.FromError(stores.storesErrors["user-1"])
				require.True(t, ok)
				require.Equal(t, s.Code(), codes.PermissionDenied)
				require.ErrorIs(t, stores.storesErrors["user-2"], nil)
			}

			mBucket.GetFailures = tc.GetFailures

			_, _, err = querySeries(stores, "user-1", "series", 0, 100)
			s, _ := status.FromError(err)
			require.Equal(t, codes.PermissionDenied, s.Code())
			_, err = queryLabelsNames(stores, "user-1", "series", 0, 100)
			s, _ = status.FromError(err)
			require.Equal(t, codes.PermissionDenied, s.Code())
			_, err = queryLabelsValues(stores, "user-1", "__name__", "series", 0, 100)
			s, _ = status.FromError(err)
			require.Equal(t, codes.PermissionDenied, s.Code())
			_, _, err = querySeries(stores, "user-2", "series", 0, 100)
			require.NoError(t, err)
			_, err = queryLabelsNames(stores, "user-1", "series", 0, 100)
			s, _ = status.FromError(err)
			require.Equal(t, codes.PermissionDenied, s.Code())
			_, err = queryLabelsValues(stores, "user-1", "__name__", "series", 0, 100)
			s, _ = status.FromError(err)
			require.Equal(t, codes.PermissionDenied, s.Code())

			// Cleaning the error
			mBucket.GetFailures = map[string]error{}
			require.NoError(t, stores.SyncBlocks(context.Background()))
			require.ErrorIs(t, stores.storesErrors["user-1"], nil)
			require.ErrorIs(t, stores.storesErrors["user-2"], nil)
			_, _, err = querySeries(stores, "user-1", "series", 0, 100)
			require.NoError(t, err)
			_, _, err = querySeries(stores, "user-2", "series", 0, 100)
			require.NoError(t, err)
			_, err = queryLabelsNames(stores, "user-1", "series", 0, 100)
			require.NoError(t, err)
			_, err = queryLabelsValues(stores, "user-1", "__name__", "series", 0, 100)
			require.NoError(t, err)
		})
	}
}

func TestBucketStores_InitialSync(t *testing.T) {
	t.Parallel()
	userToMetric := map[string]string{
		"user-1": "series_1",
		"user-2": "series_2",
	}

	ctx := context.Background()
	cfg := prepareStorageConfig(t)

	storageDir := t.TempDir()

	for userID, metricName := range userToMetric {
		generateStorageBlock(t, storageDir, userID, metricName, 10, 100, 15)
	}

	bucket, err := filesystem.NewBucketClient(filesystem.Config{Directory: storageDir})
	require.NoError(t, err)

	reg := prometheus.NewPedanticRegistry()
	stores, err := NewBucketStores(cfg, NewNoShardingStrategy(log.NewNopLogger(), nil), objstore.WithNoopInstr(bucket), defaultLimitsOverrides(t), mockLoggingLevel(), log.NewNopLogger(), reg)
	require.NoError(t, err)

	// Query series before the initial sync.
	for userID, metricName := range userToMetric {
		seriesSet, warnings, err := querySeries(stores, userID, metricName, 20, 40)
		require.NoError(t, err)
		assert.Empty(t, warnings)
		assert.Empty(t, seriesSet)
	}

	require.NoError(t, stores.InitialSync(ctx))

	// Query series after the initial sync.
	for userID, metricName := range userToMetric {
		seriesSet, warnings, err := querySeries(stores, userID, metricName, 20, 40)
		require.NoError(t, err)
		assert.Empty(t, warnings)
		require.Len(t, seriesSet, 1)
		assert.Equal(t, []labelpb.ZLabel{{Name: labels.MetricName, Value: metricName}}, seriesSet[0].Labels)
	}

	// Query series of another user.
	seriesSet, warnings, err := querySeries(stores, "user-1", "series_2", 20, 40)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Empty(t, seriesSet)

	assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
			# HELP cortex_bucket_store_blocks_loaded Number of currently loaded blocks.
			# TYPE cortex_bucket_store_blocks_loaded gauge
        	cortex_bucket_store_blocks_loaded{user="user-1"} 1
        	cortex_bucket_store_blocks_loaded{user="user-2"} 1

			# HELP cortex_bucket_store_block_loads_total Total number of remote block loading attempts.
			# TYPE cortex_bucket_store_block_loads_total counter
			cortex_bucket_store_block_loads_total 2

			# HELP cortex_bucket_store_block_load_failures_total Total number of failed remote block loading attempts.
			# TYPE cortex_bucket_store_block_load_failures_total counter
			cortex_bucket_store_block_load_failures_total 0

			# HELP cortex_bucket_stores_gate_queries_concurrent_max Number of maximum concurrent queries allowed.
			# TYPE cortex_bucket_stores_gate_queries_concurrent_max gauge
			cortex_bucket_stores_gate_queries_concurrent_max 100

			# HELP cortex_bucket_stores_gate_queries_in_flight Number of queries that are currently in flight.
			# TYPE cortex_bucket_stores_gate_queries_in_flight gauge
			cortex_bucket_stores_gate_queries_in_flight 0
	`),
		"cortex_bucket_store_blocks_loaded",
		"cortex_bucket_store_block_loads_total",
		"cortex_bucket_store_block_load_failures_total",
		"cortex_bucket_stores_gate_queries_concurrent_max",
		"cortex_bucket_stores_gate_queries_in_flight",
	))

	assert.Greater(t, testutil.ToFloat64(stores.syncLastSuccess), float64(0))
}

func TestBucketStores_InitialSyncShouldRetryOnFailure(t *testing.T) {
	ctx := context.Background()
	cfg := prepareStorageConfig(t)

	storageDir := t.TempDir()

	// Generate a block for the user in the storage.
	generateStorageBlock(t, storageDir, "user-1", "series_1", 10, 100, 15)

	bucket, err := filesystem.NewBucketClient(filesystem.Config{Directory: storageDir})
	require.NoError(t, err)

	// Wrap the bucket to fail the 1st Get() request.
	bucket = &failFirstGetBucket{Bucket: bucket}

	reg := prometheus.NewPedanticRegistry()
	stores, err := NewBucketStores(cfg, NewNoShardingStrategy(log.NewNopLogger(), nil), objstore.WithNoopInstr(bucket), defaultLimitsOverrides(t), mockLoggingLevel(), log.NewNopLogger(), reg)
	require.NoError(t, err)

	// Initial sync should succeed even if a transient error occurs.
	require.NoError(t, stores.InitialSync(ctx))

	// Query series after the initial sync.
	seriesSet, warnings, err := querySeries(stores, "user-1", "series_1", 20, 40)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, seriesSet, 1)
	assert.Equal(t, []labelpb.ZLabel{{Name: labels.MetricName, Value: "series_1"}}, seriesSet[0].Labels)

	assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
			# HELP cortex_blocks_meta_syncs_total Total blocks metadata synchronization attempts
			# TYPE cortex_blocks_meta_syncs_total counter
			cortex_blocks_meta_syncs_total 2

			# HELP cortex_blocks_meta_sync_failures_total Total blocks metadata synchronization failures
			# TYPE cortex_blocks_meta_sync_failures_total counter
			cortex_blocks_meta_sync_failures_total 1

			# HELP cortex_bucket_store_blocks_loaded Number of currently loaded blocks.
			# TYPE cortex_bucket_store_blocks_loaded gauge
			cortex_bucket_store_blocks_loaded{user="user-1"} 1

			# HELP cortex_bucket_store_block_loads_total Total number of remote block loading attempts.
			# TYPE cortex_bucket_store_block_loads_total counter
			cortex_bucket_store_block_loads_total 1

			# HELP cortex_bucket_store_block_load_failures_total Total number of failed remote block loading attempts.
			# TYPE cortex_bucket_store_block_load_failures_total counter
			cortex_bucket_store_block_load_failures_total 0
	`),
		"cortex_blocks_meta_syncs_total",
		"cortex_blocks_meta_sync_failures_total",
		"cortex_bucket_store_block_loads_total",
		"cortex_bucket_store_block_load_failures_total",
		"cortex_bucket_store_blocks_loaded",
	))

	assert.Greater(t, testutil.ToFloat64(stores.syncLastSuccess), float64(0))
}

func TestBucketStores_SyncBlocks(t *testing.T) {
	t.Parallel()
	const (
		userID     = "user-1"
		metricName = "series_1"
	)

	ctx := context.Background()
	cfg := prepareStorageConfig(t)

	storageDir := t.TempDir()

	bucket, err := filesystem.NewBucketClient(filesystem.Config{Directory: storageDir})
	require.NoError(t, err)

	reg := prometheus.NewPedanticRegistry()
	stores, err := NewBucketStores(cfg, NewNoShardingStrategy(log.NewNopLogger(), nil), objstore.WithNoopInstr(bucket), defaultLimitsOverrides(t), mockLoggingLevel(), log.NewNopLogger(), reg)
	require.NoError(t, err)

	// Run an initial sync to discover 1 block.
	generateStorageBlock(t, storageDir, userID, metricName, 10, 100, 15)
	require.NoError(t, stores.InitialSync(ctx))

	// Query a range for which we have no samples.
	seriesSet, warnings, err := querySeries(stores, userID, metricName, 150, 180)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Empty(t, seriesSet)

	// Generate another block and sync blocks again.
	generateStorageBlock(t, storageDir, userID, metricName, 100, 200, 15)
	require.NoError(t, stores.SyncBlocks(ctx))

	seriesSet, warnings, err = querySeries(stores, userID, metricName, 150, 180)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Len(t, seriesSet, 1)
	assert.Equal(t, []labelpb.ZLabel{{Name: labels.MetricName, Value: metricName}}, seriesSet[0].Labels)

	assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
			# HELP cortex_bucket_store_blocks_loaded Number of currently loaded blocks.
			# TYPE cortex_bucket_store_blocks_loaded gauge
			cortex_bucket_store_blocks_loaded{user="user-1"} 2

			# HELP cortex_bucket_store_block_loads_total Total number of remote block loading attempts.
			# TYPE cortex_bucket_store_block_loads_total counter
			cortex_bucket_store_block_loads_total 2

			# HELP cortex_bucket_store_block_load_failures_total Total number of failed remote block loading attempts.
			# TYPE cortex_bucket_store_block_load_failures_total counter
			cortex_bucket_store_block_load_failures_total 0

			# HELP cortex_bucket_stores_gate_queries_concurrent_max Number of maximum concurrent queries allowed.
			# TYPE cortex_bucket_stores_gate_queries_concurrent_max gauge
			cortex_bucket_stores_gate_queries_concurrent_max 100

			# HELP cortex_bucket_stores_gate_queries_in_flight Number of queries that are currently in flight.
			# TYPE cortex_bucket_stores_gate_queries_in_flight gauge
			cortex_bucket_stores_gate_queries_in_flight 0
	`),
		"cortex_bucket_store_blocks_loaded",
		"cortex_bucket_store_block_loads_total",
		"cortex_bucket_store_block_load_failures_total",
		"cortex_bucket_stores_gate_queries_concurrent_max",
		"cortex_bucket_stores_gate_queries_in_flight",
	))

	assert.Greater(t, testutil.ToFloat64(stores.syncLastSuccess), float64(0))
}

func TestBucketStores_syncUsersBlocks(t *testing.T) {
	t.Parallel()
	allUsers := []string{"user-1", "user-2", "user-3"}

	tests := map[string]struct {
		shardingStrategy ShardingStrategy
		expectedStores   int32
		allowedTenants   *util.AllowedTenants
	}{
		"when sharding is disabled all users should be synced": {
			shardingStrategy: NewNoShardingStrategy(log.NewNopLogger(), nil),
			expectedStores:   3,
		},
		"sharding disabled, user-1 disabled": {
			shardingStrategy: NewNoShardingStrategy(log.NewNopLogger(), util.NewAllowedTenants(nil, []string{"user-1"})),
			expectedStores:   2,
		},
		"when sharding is enabled only stores for filtered users should be created": {
			shardingStrategy: func() ShardingStrategy {
				s := &mockShardingStrategy{}
				s.On("FilterUsers", mock.Anything, allUsers).Return([]string{"user-1", "user-2"})
				return s
			}(),
			expectedStores: 2,
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			cfg := prepareStorageConfig(t)
			cfg.BucketStore.TenantSyncConcurrency = 2

			bucketClient := &bucket.ClientMock{}
			bucketClient.MockIter("", allUsers, nil)
			bucketClient.MockIter(tenant.GlobalMarkersDir, []string{}, nil)
			bucketClient.MockIter("user-1/", []string{}, nil)
			bucketClient.MockExists(path.Join(tenant.GlobalMarkersDir, "user-1", cortex_tsdb.TenantDeletionMarkFile), false, nil)
			bucketClient.MockExists(path.Join("user-1", "markers", cortex_tsdb.TenantDeletionMarkFile), false, nil)
			bucketClient.MockIter("user-2/", []string{}, nil)
			bucketClient.MockExists(path.Join(tenant.GlobalMarkersDir, "user-2", cortex_tsdb.TenantDeletionMarkFile), false, nil)
			bucketClient.MockExists(path.Join("user-2", "markers", cortex_tsdb.TenantDeletionMarkFile), false, nil)
			bucketClient.MockIter("user-3/", []string{}, nil)
			bucketClient.MockExists(path.Join(tenant.GlobalMarkersDir, "user-3", cortex_tsdb.TenantDeletionMarkFile), false, nil)
			bucketClient.MockExists(path.Join("user-3", "markers", cortex_tsdb.TenantDeletionMarkFile), false, nil)

			stores, err := NewBucketStores(cfg, testData.shardingStrategy, bucketClient, defaultLimitsOverrides(t), mockLoggingLevel(), log.NewNopLogger(), nil)
			require.NoError(t, err)

			// Sync user stores and count the number of times the callback is called.
			var storesCount atomic.Int32
			err = stores.syncUsersBlocks(context.Background(), func(ctx context.Context, bs *store.BucketStore) error {
				storesCount.Inc()
				return nil
			})

			assert.NoError(t, err)
			bucketClient.AssertNumberOfCalls(t, "Iter", 2)
			assert.Equal(t, storesCount.Load(), testData.expectedStores)
		})
	}
}

func TestBucketStores_scanUsers(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		scanner     *mockScanner
		expectedRes []string
	}{
		"should return unique users only": {
			scanner: &mockScanner{
				res: []string{"user-1", "user-2", "user-1"},
			},
			expectedRes: []string{"user-1", "user-2"},
		},
	}

	for testName, testData := range tests {
		testData := testData
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			stores := &BucketStores{
				userScanner: testData.scanner,
			}

			users, err := stores.scanUsers(context.Background())

			assert.NoError(t, err)
			assert.ElementsMatch(t, testData.expectedRes, users)
		})
	}
}

func TestBucketStores_Series_ShouldCorrectlyQuerySeriesSpanningMultipleChunks(t *testing.T) {
	for _, lazyLoadingEnabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("lazy loading enabled = %v", lazyLoadingEnabled), func(t *testing.T) {
			testBucketStoresSeriesShouldCorrectlyQuerySeriesSpanningMultipleChunks(t, lazyLoadingEnabled)
		})
	}
}

func testBucketStoresSeriesShouldCorrectlyQuerySeriesSpanningMultipleChunks(t *testing.T, lazyLoadingEnabled bool) {
	const (
		userID     = "user-1"
		metricName = "series_1"
	)

	ctx := context.Background()
	cfg := prepareStorageConfig(t)
	cfg.BucketStore.IndexHeaderLazyLoadingEnabled = lazyLoadingEnabled
	cfg.BucketStore.IndexHeaderLazyLoadingIdleTimeout = time.Minute

	storageDir := t.TempDir()

	// Generate a single block with 1 series and a lot of samples.
	generateStorageBlock(t, storageDir, userID, metricName, 0, 10000, 1)

	bucket, err := filesystem.NewBucketClient(filesystem.Config{Directory: storageDir})
	require.NoError(t, err)

	reg := prometheus.NewPedanticRegistry()
	stores, err := NewBucketStores(cfg, NewNoShardingStrategy(log.NewNopLogger(), nil), objstore.WithNoopInstr(bucket), defaultLimitsOverrides(t), mockLoggingLevel(), log.NewNopLogger(), reg)
	require.NoError(t, err)
	require.NoError(t, stores.InitialSync(ctx))

	tests := map[string]struct {
		reqMinTime      int64
		reqMaxTime      int64
		expectedSamples int
	}{
		"query the entire block": {
			reqMinTime:      math.MinInt64,
			reqMaxTime:      math.MaxInt64,
			expectedSamples: 10000,
		},
		"query the beginning of the block": {
			reqMinTime:      0,
			reqMaxTime:      100,
			expectedSamples: store.MaxSamplesPerChunk,
		},
		"query the middle of the block": {
			reqMinTime:      4000,
			reqMaxTime:      4050,
			expectedSamples: store.MaxSamplesPerChunk,
		},
		"query the end of the block": {
			reqMinTime:      9800,
			reqMaxTime:      10000,
			expectedSamples: (store.MaxSamplesPerChunk * 2) + (10000 % store.MaxSamplesPerChunk),
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			// Query a range for which we have no samples.
			seriesSet, warnings, err := querySeries(stores, userID, metricName, testData.reqMinTime, testData.reqMaxTime)
			require.NoError(t, err)
			assert.Empty(t, warnings)
			assert.Len(t, seriesSet, 1)

			// Count returned samples.
			samples, err := readSamplesFromChunks(seriesSet[0].Chunks)
			require.NoError(t, err)
			assert.Equal(t, testData.expectedSamples, len(samples))
		})
	}
}

func TestBucketStores_Series_ShouldReturnErrorIfMaxInflightRequestIsReached(t *testing.T) {
	cfg := prepareStorageConfig(t)
	cfg.BucketStore.MaxInflightRequests = 10
	reg := prometheus.NewPedanticRegistry()
	storageDir := t.TempDir()
	generateStorageBlock(t, storageDir, "user_id", "series_1", 0, 100, 15)
	bucket, err := filesystem.NewBucketClient(filesystem.Config{Directory: storageDir})
	require.NoError(t, err)

	stores, err := NewBucketStores(cfg, NewNoShardingStrategy(log.NewNopLogger(), nil), objstore.WithNoopInstr(bucket), defaultLimitsOverrides(t), mockLoggingLevel(), log.NewNopLogger(), reg)
	require.NoError(t, err)
	require.NoError(t, stores.InitialSync(context.Background()))

	stores.inflightRequestMu.Lock()
	stores.inflightRequestCnt = 10
	stores.inflightRequestMu.Unlock()
	series, warnings, err := querySeries(stores, "user_id", "series_1", 0, 100)
	assert.ErrorIs(t, err, ErrTooManyInflightRequests)
	assert.Empty(t, series)
	assert.Empty(t, warnings)
}

func TestBucketStores_Series_ShouldNotCheckMaxInflightRequestsIfTheLimitIsDisabled(t *testing.T) {
	cfg := prepareStorageConfig(t)
	reg := prometheus.NewPedanticRegistry()
	storageDir := t.TempDir()
	generateStorageBlock(t, storageDir, "user_id", "series_1", 0, 100, 15)
	bucket, err := filesystem.NewBucketClient(filesystem.Config{Directory: storageDir})
	require.NoError(t, err)

	stores, err := NewBucketStores(cfg, NewNoShardingStrategy(log.NewNopLogger(), nil), objstore.WithNoopInstr(bucket), defaultLimitsOverrides(t), mockLoggingLevel(), log.NewNopLogger(), reg)
	require.NoError(t, err)
	require.NoError(t, stores.InitialSync(context.Background()))

	stores.inflightRequestMu.Lock()
	stores.inflightRequestCnt = 10 // max_inflight_request is set to 0 by default = disabled
	stores.inflightRequestMu.Unlock()
	series, _, err := querySeries(stores, "user_id", "series_1", 0, 100)
	require.NoError(t, err)
	assert.Equal(t, 1, len(series))
}

func TestBucketStores_SyncBlocksWithIgnoreBlocksBefore(t *testing.T) {
	t.Parallel()

	const userID = "user-1"
	const metricName = "test_metric"

	ctx := context.Background()
	cfg := prepareStorageConfig(t)

	// Configure IgnoreBlocksBefore to filter out blocks older than 2 hours
	cfg.BucketStore.IgnoreBlocksBefore = 2 * time.Hour

	storageDir := t.TempDir()

	// Create blocks with different timestamps
	now := time.Now()

	// Block 1: Very old block (should be ignored - time-excluded)
	oldBlockTime := now.Add(-5 * time.Hour)
	generateStorageBlock(t, storageDir, userID, metricName+"_old",
		oldBlockTime.UnixMilli(), oldBlockTime.Add(time.Hour).UnixMilli(), 15)

	// Block 2: Recent block (should be synced)
	recentBlockTime := now.Add(-1 * time.Hour)
	generateStorageBlock(t, storageDir, userID, metricName+"_recent",
		recentBlockTime.UnixMilli(), recentBlockTime.Add(time.Hour).UnixMilli(), 15)

	// Block 3: Current block (should be synced)
	currentBlockTime := now.Add(-30 * time.Minute)
	generateStorageBlock(t, storageDir, userID, metricName+"_current",
		currentBlockTime.UnixMilli(), currentBlockTime.Add(time.Hour).UnixMilli(), 15)

	bucket, err := filesystem.NewBucketClient(filesystem.Config{Directory: storageDir})
	require.NoError(t, err)

	reg := prometheus.NewPedanticRegistry()
	stores, err := NewBucketStores(cfg, NewNoShardingStrategy(log.NewNopLogger(), nil),
		objstore.WithNoopInstr(bucket), defaultLimitsOverrides(t), mockLoggingLevel(), log.NewNopLogger(), reg)
	require.NoError(t, err)

	// Perform initial sync
	require.NoError(t, stores.InitialSync(ctx))

	// Verify that only recent and current blocks are loaded
	// The old block should be filtered out by IgnoreBlocksBefore (time-excluded)
	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
		# HELP cortex_blocks_meta_synced Reflects current state of synced blocks (over all tenants).
		# TYPE cortex_blocks_meta_synced gauge
		cortex_blocks_meta_synced{state="corrupted-meta-json"} 0
		cortex_blocks_meta_synced{state="duplicate"} 0
		cortex_blocks_meta_synced{state="failed"} 0
		cortex_blocks_meta_synced{state="label-excluded"} 0
		cortex_blocks_meta_synced{state="loaded"} 2
		cortex_blocks_meta_synced{state="marked-for-deletion"} 0
		cortex_blocks_meta_synced{state="marked-for-no-compact"} 0
		cortex_blocks_meta_synced{state="no-meta-json"} 0
		cortex_blocks_meta_synced{state="parquet-migrated"} 0
		cortex_blocks_meta_synced{state="time-excluded"} 1
		cortex_blocks_meta_synced{state="too-fresh"} 0
		# HELP cortex_blocks_meta_syncs_total Total blocks metadata synchronization attempts
		# TYPE cortex_blocks_meta_syncs_total counter
		cortex_blocks_meta_syncs_total 3
		# HELP cortex_bucket_store_blocks_meta_sync_failures_total Total blocks metadata synchronization failures
		# TYPE cortex_bucket_store_blocks_meta_sync_failures_total counter
		cortex_bucket_store_blocks_meta_sync_failures_total 0
		# HELP cortex_bucket_store_block_loads_total Total number of remote block loading attempts.
		# TYPE cortex_bucket_store_block_loads_total counter
		cortex_bucket_store_block_loads_total 2
		# HELP cortex_bucket_store_blocks_loaded Number of currently loaded blocks.
		# TYPE cortex_bucket_store_blocks_loaded gauge
		cortex_bucket_store_blocks_loaded{user="user-1"} 2
	`), "cortex_bucket_store_block_loads_total", "cortex_bucket_store_blocks_loaded", "cortex_blocks_meta_synced"))
}

func prepareStorageConfig(t *testing.T) cortex_tsdb.BlocksStorageConfig {
	cfg := cortex_tsdb.BlocksStorageConfig{}
	flagext.DefaultValues(&cfg)
	cfg.BucketStore.SyncDir = t.TempDir()

	return cfg
}

func generateStorageBlock(t *testing.T, storageDir, userID string, metricName string, minT, maxT int64, step int) {
	// Create a directory for the user (if doesn't already exist).
	userDir := filepath.Join(storageDir, userID)
	if _, err := os.Stat(userDir); err != nil {
		require.NoError(t, os.Mkdir(userDir, os.ModePerm))
	}

	// Create a temporary directory where the TSDB is opened,
	// then it will be snapshotted to the storage directory.
	tmpDir := t.TempDir()

	db, err := tsdb.Open(tmpDir, promslog.NewNopLogger(), nil, tsdb.DefaultOptions(), nil)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	series := labels.FromStrings(labels.MetricName, metricName)

	app := db.Appender(context.Background())
	for ts := minT; ts < maxT; ts += int64(step) {
		_, err = app.Append(0, series, ts, 1)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// Snapshot TSDB to the storage directory.
	require.NoError(t, db.Snapshot(userDir, true))
}

func querySeries(stores *BucketStores, userID, metricName string, minT, maxT int64) ([]*storepb.Series, annotations.Annotations, error) {
	req := &storepb.SeriesRequest{
		MinTime: minT,
		MaxTime: maxT,
		Matchers: []storepb.LabelMatcher{{
			Type:  storepb.LabelMatcher_EQ,
			Name:  labels.MetricName,
			Value: metricName,
		}},
		PartialResponseStrategy: storepb.PartialResponseStrategy_ABORT,
	}

	ctx := setUserIDToGRPCContext(context.Background(), userID)
	srv := newBucketStoreSeriesServer(ctx)
	err := stores.Series(req, srv)

	return srv.SeriesSet, srv.Warnings, err
}

func queryLabelsNames(stores *BucketStores, userID, metricName string, start, end int64) (*storepb.LabelNamesResponse, error) {
	req := &storepb.LabelNamesRequest{
		Start: start,
		End:   end,
		Matchers: []storepb.LabelMatcher{{
			Type:  storepb.LabelMatcher_EQ,
			Name:  labels.MetricName,
			Value: metricName,
		}},
		PartialResponseStrategy: storepb.PartialResponseStrategy_ABORT,
	}

	ctx := setUserIDToGRPCContext(context.Background(), userID)
	return stores.LabelNames(ctx, req)
}

func queryLabelsValues(stores *BucketStores, userID, labelName, metricName string, start, end int64) (*storepb.LabelValuesResponse, error) {
	req := &storepb.LabelValuesRequest{
		Start: start,
		End:   end,
		Label: labelName,
		Matchers: []storepb.LabelMatcher{{
			Type:  storepb.LabelMatcher_EQ,
			Name:  labels.MetricName,
			Value: metricName,
		}},
		PartialResponseStrategy: storepb.PartialResponseStrategy_ABORT,
	}

	ctx := setUserIDToGRPCContext(context.Background(), userID)
	return stores.LabelValues(ctx, req)
}

func mockLoggingLevel() logging.Level {
	level := logging.Level{}
	err := level.Set("info")
	if err != nil {
		panic(err)
	}

	return level
}

func setUserIDToGRPCContext(ctx context.Context, userID string) context.Context {
	// We have to store it in the incoming metadata because we have to emulate the
	// case it's coming from a gRPC request, while here we're running everything in-memory.
	return metadata.NewIncomingContext(ctx, metadata.Pairs(cortex_tsdb.TenantIDExternalLabel, userID))
}

func TestBucketStores_deleteLocalFilesForExcludedTenants(t *testing.T) {
	const (
		user1 = "user-1"
		user2 = "user-2"
	)

	userToMetric := map[string]string{
		user1: "series_1",
		user2: "series_2",
	}

	ctx := context.Background()
	cfg := prepareStorageConfig(t)

	storageDir := t.TempDir()

	for userID, metricName := range userToMetric {
		generateStorageBlock(t, storageDir, userID, metricName, 10, 100, 15)
	}

	bucket, err := filesystem.NewBucketClient(filesystem.Config{Directory: storageDir})
	require.NoError(t, err)

	sharding := userShardingStrategy{}

	reg := prometheus.NewPedanticRegistry()
	stores, err := NewBucketStores(cfg, &sharding, objstore.WithNoopInstr(bucket), defaultLimitsOverrides(t), mockLoggingLevel(), log.NewNopLogger(), reg)
	require.NoError(t, err)

	// Perform sync.
	sharding.users = []string{user1, user2}
	require.NoError(t, stores.InitialSync(ctx))
	require.Equal(t, []string{user1, user2}, getUsersInDir(t, cfg.BucketStore.SyncDir))

	metricNamesWithoutLoaded := []string{"cortex_bucket_store_block_drops_total", "cortex_bucket_store_block_loads_total"}
	metricNames := append(metricNamesWithoutLoaded, "cortex_bucket_store_blocks_loaded")

	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
		            	# HELP cortex_bucket_store_block_drops_total Total number of local blocks that were dropped.
		            	# TYPE cortex_bucket_store_block_drops_total counter
		            	cortex_bucket_store_block_drops_total 0
		            	# HELP cortex_bucket_store_block_loads_total Total number of remote block loading attempts.
		            	# TYPE cortex_bucket_store_block_loads_total counter
		            	cortex_bucket_store_block_loads_total 2
		            	# HELP cortex_bucket_store_blocks_loaded Number of currently loaded blocks.
		            	# TYPE cortex_bucket_store_blocks_loaded gauge
		            	cortex_bucket_store_blocks_loaded{user="user-1"} 1
		            	cortex_bucket_store_blocks_loaded{user="user-2"} 1
	`), metricNames...))

	// Single user left in shard.
	sharding.users = []string{user1}
	require.NoError(t, stores.SyncBlocks(ctx))
	require.Equal(t, []string{user1}, getUsersInDir(t, cfg.BucketStore.SyncDir))

	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
	  	            	# HELP cortex_bucket_store_block_drops_total Total number of local blocks that were dropped.
	  	            	# TYPE cortex_bucket_store_block_drops_total counter
	  	            	cortex_bucket_store_block_drops_total 1
	  	            	# HELP cortex_bucket_store_block_loads_total Total number of remote block loading attempts.
	  	            	# TYPE cortex_bucket_store_block_loads_total counter
	  	            	cortex_bucket_store_block_loads_total 2
	  	            	# HELP cortex_bucket_store_blocks_loaded Number of currently loaded blocks.
	  	            	# TYPE cortex_bucket_store_blocks_loaded gauge
	  	            	cortex_bucket_store_blocks_loaded{user="user-1"} 1
	`), metricNames...))

	// No users left in this shard.
	sharding.users = nil
	require.NoError(t, stores.SyncBlocks(ctx))
	require.Equal(t, []string(nil), getUsersInDir(t, cfg.BucketStore.SyncDir))

	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
	   	            	# HELP cortex_bucket_store_block_drops_total Total number of local blocks that were dropped.
	   	            	# TYPE cortex_bucket_store_block_drops_total counter
	   	            	cortex_bucket_store_block_drops_total 2
	   	            	# HELP cortex_bucket_store_block_loads_total Total number of remote block loading attempts.
	   	            	# TYPE cortex_bucket_store_block_loads_total counter
	   	            	cortex_bucket_store_block_loads_total 2
	`), metricNamesWithoutLoaded...))

	// We can always get user back.
	sharding.users = []string{user1}
	require.NoError(t, stores.SyncBlocks(ctx))
	require.Equal(t, []string{user1}, getUsersInDir(t, cfg.BucketStore.SyncDir))

	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
	   	            	# HELP cortex_bucket_store_block_drops_total Total number of local blocks that were dropped.
	   	            	# TYPE cortex_bucket_store_block_drops_total counter
	   	            	cortex_bucket_store_block_drops_total 2
	   	            	# HELP cortex_bucket_store_block_loads_total Total number of remote block loading attempts.
	   	            	# TYPE cortex_bucket_store_block_loads_total counter
	   	            	cortex_bucket_store_block_loads_total 3
	   	            	# HELP cortex_bucket_store_blocks_loaded Number of currently loaded blocks.
	   	            	# TYPE cortex_bucket_store_blocks_loaded gauge
	   	            	cortex_bucket_store_blocks_loaded{user="user-1"} 1
	`), metricNames...))
}

func TestBucketStores_tokenBuckets(t *testing.T) {
	const (
		user1 = "user-1"
		user2 = "user-2"
	)

	ctx := context.Background()
	cfg := prepareStorageConfig(t)
	cfg.BucketStore.TokenBucketBytesLimiter.Mode = string(cortex_tsdb.TokenBucketBytesLimiterEnabled)

	storageDir := t.TempDir()
	userToMetric := map[string]string{
		user1: "series_1",
		user2: "series_2",
	}
	for userID, metricName := range userToMetric {
		generateStorageBlock(t, storageDir, userID, metricName, 10, 100, 15)
	}

	sharding := userShardingStrategy{}
	sharding.users = []string{user1, user2}

	bucket, err := filesystem.NewBucketClient(filesystem.Config{Directory: storageDir})
	assert.NoError(t, err)

	reg := prometheus.NewPedanticRegistry()
	stores, err := NewBucketStores(cfg, &sharding, objstore.WithNoopInstr(bucket), defaultLimitsOverrides(t), mockLoggingLevel(), log.NewNopLogger(), reg)
	assert.NoError(t, err)
	assert.NotNil(t, stores.instanceTokenBucket)

	assert.NoError(t, stores.InitialSync(ctx))
	assert.NotNil(t, stores.getUserTokenBucket("user-1"))
	assert.NotNil(t, stores.getUserTokenBucket("user-2"))

	sharding.users = []string{user1}
	assert.NoError(t, stores.SyncBlocks(ctx))
	assert.NotNil(t, stores.getUserTokenBucket("user-1"))
	assert.Nil(t, stores.getUserTokenBucket("user-2"))

	sharding.users = []string{}
	assert.NoError(t, stores.SyncBlocks(ctx))
	assert.Nil(t, stores.getUserTokenBucket("user-1"))
	assert.Nil(t, stores.getUserTokenBucket("user-2"))

	cfg.BucketStore.TokenBucketBytesLimiter.Mode = string(cortex_tsdb.TokenBucketBytesLimiterDryRun)
	sharding.users = []string{user1, user2}
	reg = prometheus.NewPedanticRegistry()
	stores, err = NewBucketStores(cfg, &sharding, objstore.WithNoopInstr(bucket), defaultLimitsOverrides(t), mockLoggingLevel(), log.NewNopLogger(), reg)
	assert.NoError(t, err)
	assert.NotNil(t, stores.instanceTokenBucket)

	assert.NoError(t, stores.InitialSync(ctx))
	assert.NotNil(t, stores.getUserTokenBucket("user-1"))
	assert.NotNil(t, stores.getUserTokenBucket("user-2"))

	cfg.BucketStore.TokenBucketBytesLimiter.Mode = string(cortex_tsdb.TokenBucketBytesLimiterDisabled)
	sharding.users = []string{user1, user2}
	reg = prometheus.NewPedanticRegistry()
	stores, err = NewBucketStores(cfg, &sharding, objstore.WithNoopInstr(bucket), defaultLimitsOverrides(t), mockLoggingLevel(), log.NewNopLogger(), reg)
	assert.NoError(t, err)

	assert.NoError(t, stores.InitialSync(ctx))
	assert.Nil(t, stores.instanceTokenBucket)
	assert.Nil(t, stores.getUserTokenBucket("user-1"))
	assert.Nil(t, stores.getUserTokenBucket("user-2"))
}

func TestBucketStores_getTokensToRetrieve(t *testing.T) {
	cfg := prepareStorageConfig(t)
	cfg.BucketStore.TokenBucketBytesLimiter.FetchedPostingsTokenFactor = 1
	cfg.BucketStore.TokenBucketBytesLimiter.TouchedPostingsTokenFactor = 2
	cfg.BucketStore.TokenBucketBytesLimiter.FetchedSeriesTokenFactor = 3
	cfg.BucketStore.TokenBucketBytesLimiter.TouchedSeriesTokenFactor = 4
	cfg.BucketStore.TokenBucketBytesLimiter.FetchedChunksTokenFactor = 0
	cfg.BucketStore.TokenBucketBytesLimiter.TouchedChunksTokenFactor = 0.5

	storageDir := t.TempDir()
	bucket, err := filesystem.NewBucketClient(filesystem.Config{Directory: storageDir})
	assert.NoError(t, err)

	reg := prometheus.NewPedanticRegistry()
	stores, err := NewBucketStores(cfg, NewNoShardingStrategy(log.NewNopLogger(), nil), objstore.WithNoopInstr(bucket), defaultLimitsOverrides(t), mockLoggingLevel(), log.NewNopLogger(), reg)
	assert.NoError(t, err)

	assert.Equal(t, int64(2), stores.getTokensToRetrieve(2, store.PostingsFetched))
	assert.Equal(t, int64(4), stores.getTokensToRetrieve(2, store.PostingsTouched))
	assert.Equal(t, int64(6), stores.getTokensToRetrieve(2, store.SeriesFetched))
	assert.Equal(t, int64(8), stores.getTokensToRetrieve(2, store.SeriesTouched))
	assert.Equal(t, int64(0), stores.getTokensToRetrieve(2, store.ChunksFetched))
	assert.Equal(t, int64(1), stores.getTokensToRetrieve(2, store.ChunksTouched))
}

func getUsersInDir(t *testing.T, dir string) []string {
	fs, err := os.ReadDir(dir)
	require.NoError(t, err)

	var result []string
	for _, fi := range fs {
		if fi.IsDir() {
			result = append(result, fi.Name())
		}
	}
	sort.Strings(result)
	return result
}

type userShardingStrategy struct {
	users []string
}

func (u *userShardingStrategy) FilterUsers(ctx context.Context, userIDs []string) []string {
	return u.users
}

func (u *userShardingStrategy) FilterBlocks(ctx context.Context, userID string, metas map[ulid.ULID]*thanos_metadata.Meta, loaded map[ulid.ULID]struct{}, synced block.GaugeVec) error {
	if util.StringsContain(u.users, userID) {
		return nil
	}

	for k := range metas {
		delete(metas, k)
	}
	return nil
}

func (u *userShardingStrategy) OwnBlock(userID string, _ thanos_metadata.Meta) (bool, error) {
	if util.StringsContain(u.users, userID) {
		return true, nil
	}

	return false, nil
}

// failFirstGetBucket is an objstore.Bucket wrapper which fails the first Get() request with a mocked error.
type failFirstGetBucket struct {
	objstore.Bucket

	firstGet atomic.Bool
}

func (f *failFirstGetBucket) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	if f.firstGet.CompareAndSwap(false, true) {
		return nil, errors.New("Get() request mocked error")
	}

	return f.Bucket.Get(ctx, name)
}

type mockScanner struct {
	res []string
}

func (m *mockScanner) ScanUsers(_ context.Context) (active, deleting, deleted []string, err error) {
	return m.res, nil, nil, nil
}
