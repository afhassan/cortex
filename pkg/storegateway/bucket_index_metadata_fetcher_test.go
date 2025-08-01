package storegateway

import (
	"bytes"
	"context"
	"errors"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/oklog/ulid/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/thanos/pkg/block"
	"github.com/thanos-io/thanos/pkg/block/metadata"

	"github.com/cortexproject/cortex/pkg/storage/bucket"
	"github.com/cortexproject/cortex/pkg/storage/tsdb/bucketindex"
	cortex_testutil "github.com/cortexproject/cortex/pkg/storage/tsdb/testutil"
	"github.com/cortexproject/cortex/pkg/util/concurrency"
)

func TestBucketIndexMetadataFetcher_Fetch(t *testing.T) {
	t.Parallel()
	const userID = "user-1"

	bkt, _ := cortex_testutil.PrepareFilesystemBucket(t)
	reg := prometheus.NewPedanticRegistry()
	ctx := context.Background()
	now := time.Now()
	logs := &concurrency.SyncBuffer{}
	logger := log.NewLogfmtLogger(logs)

	// Create a bucket index.
	block1 := &bucketindex.Block{ID: ulid.MustNew(1, nil)}
	block2 := &bucketindex.Block{ID: ulid.MustNew(2, nil)}
	block3 := &bucketindex.Block{ID: ulid.MustNew(3, nil)}
	mark1 := &bucketindex.BlockDeletionMark{ID: block1.ID, DeletionTime: now.Add(-time.Hour).Unix()}     // Below the ignore delay threshold.
	mark2 := &bucketindex.BlockDeletionMark{ID: block2.ID, DeletionTime: now.Add(-3 * time.Hour).Unix()} // Above the ignore delay threshold.

	require.NoError(t, bucketindex.WriteIndex(ctx, bkt, userID, nil, &bucketindex.Index{
		Version:            bucketindex.IndexVersion1,
		Blocks:             bucketindex.Blocks{block1, block2, block3},
		BlockDeletionMarks: bucketindex.BlockDeletionMarks{mark1, mark2},
		UpdatedAt:          now.Unix(),
	}))

	// Create a metadata fetcher with filters.
	filters := []block.MetadataFilter{
		NewIgnoreDeletionMarkFilter(logger, bucket.NewUserBucketClient(userID, bkt, nil), 2*time.Hour, 1),
	}

	fetcher := NewBucketIndexMetadataFetcher(userID, bkt, NewNoShardingStrategy(logger, nil), nil, logger, reg, filters)
	metas, partials, err := fetcher.Fetch(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[ulid.ULID]*metadata.Meta{
		block1.ID: block1.ThanosMeta(userID),
		block3.ID: block3.ThanosMeta(userID),
	}, metas)
	assert.Empty(t, partials)
	assert.Empty(t, logs)

	assert.NoError(t, testutil.GatherAndCompare(reg, bytes.NewBufferString(`
		# HELP blocks_meta_modified Number of blocks whose metadata changed
		# TYPE blocks_meta_modified gauge
		blocks_meta_modified{modified="replica-label-removed"} 0

		# HELP blocks_meta_sync_failures_total Total blocks metadata synchronization failures
		# TYPE blocks_meta_sync_failures_total counter
		blocks_meta_sync_failures_total 0

		# HELP blocks_meta_synced Number of block metadata synced
		# TYPE blocks_meta_synced gauge
		blocks_meta_synced{state="corrupted-bucket-index"} 0
		blocks_meta_synced{state="corrupted-meta-json"} 0
		blocks_meta_synced{state="duplicate"} 0
		blocks_meta_synced{state="failed"} 0
		blocks_meta_synced{state="label-excluded"} 0
		blocks_meta_synced{state="loaded"} 2
		blocks_meta_synced{state="marked-for-deletion"} 1
		blocks_meta_synced{state="marked-for-no-compact"} 0
		blocks_meta_synced{state="no-bucket-index"} 0
		blocks_meta_synced{state="no-meta-json"} 0
		blocks_meta_synced{state="parquet-migrated"} 0
		blocks_meta_synced{state="time-excluded"} 0
		blocks_meta_synced{state="too-fresh"} 0

		# HELP blocks_meta_syncs_total Total blocks metadata synchronization attempts
		# TYPE blocks_meta_syncs_total counter
		blocks_meta_syncs_total 1
	`),
		"blocks_meta_modified",
		"blocks_meta_sync_failures_total",
		"blocks_meta_synced",
		"blocks_meta_syncs_total",
	))
}

func TestBucketIndexMetadataFetcher_Fetch_KeyPermissionDenied(t *testing.T) {
	const userID = "user-1"

	bkt := &bucket.ClientMock{}
	reg := prometheus.NewPedanticRegistry()
	ctx := context.Background()

	bkt.MockGet(userID+"/bucket-index.json.gz", "c", bucket.ErrCustomerManagedKeyAccessDenied)

	fetcher := NewBucketIndexMetadataFetcher(userID, bkt, NewNoShardingStrategy(log.NewNopLogger(), nil), nil, log.NewNopLogger(), reg, nil)
	metas, _, err := fetcher.Fetch(ctx)
	require.True(t, errors.Is(err, bucket.ErrCustomerManagedKeyAccessDenied))
	assert.Empty(t, metas)

	assert.NoError(t, testutil.GatherAndCompare(reg, bytes.NewBufferString(`
		# HELP blocks_meta_modified Number of blocks whose metadata changed
		# TYPE blocks_meta_modified gauge
		blocks_meta_modified{modified="replica-label-removed"} 0
		# HELP blocks_meta_sync_failures_total Total blocks metadata synchronization failures
		# TYPE blocks_meta_sync_failures_total counter
		blocks_meta_sync_failures_total 0
		# HELP blocks_meta_synced Number of block metadata synced
		# TYPE blocks_meta_synced gauge
		blocks_meta_synced{state="corrupted-bucket-index"} 0
		blocks_meta_synced{state="corrupted-meta-json"} 0
		blocks_meta_synced{state="duplicate"} 0
		blocks_meta_synced{state="failed"} 0
		blocks_meta_synced{state="key-access-denied"} 1
		blocks_meta_synced{state="label-excluded"} 0
		blocks_meta_synced{state="loaded"} 0
		blocks_meta_synced{state="marked-for-deletion"} 0
		blocks_meta_synced{state="marked-for-no-compact"} 0
		blocks_meta_synced{state="no-bucket-index"} 0
		blocks_meta_synced{state="no-meta-json"} 0
		blocks_meta_synced{state="parquet-migrated"} 0
		blocks_meta_synced{state="time-excluded"} 0
		blocks_meta_synced{state="too-fresh"} 0
		# HELP blocks_meta_syncs_total Total blocks metadata synchronization attempts
		# TYPE blocks_meta_syncs_total counter
		blocks_meta_syncs_total 1
	`),
		"blocks_meta_modified",
		"blocks_meta_sync_failures_total",
		"blocks_meta_synced",
		"blocks_meta_syncs_total",
	))
}

func TestBucketIndexMetadataFetcher_Fetch_NoBucketIndex(t *testing.T) {
	t.Parallel()
	const userID = "user-1"

	bkt, _ := cortex_testutil.PrepareFilesystemBucket(t)
	reg := prometheus.NewPedanticRegistry()
	ctx := context.Background()
	logs := &concurrency.SyncBuffer{}
	logger := log.NewLogfmtLogger(logs)

	fetcher := NewBucketIndexMetadataFetcher(userID, bkt, NewNoShardingStrategy(logger, nil), nil, logger, reg, nil)
	metas, partials, err := fetcher.Fetch(ctx)
	require.NoError(t, err)
	assert.Empty(t, metas)
	assert.Empty(t, partials)
	assert.Empty(t, logs)

	assert.NoError(t, testutil.GatherAndCompare(reg, bytes.NewBufferString(`
		# HELP blocks_meta_modified Number of blocks whose metadata changed
		# TYPE blocks_meta_modified gauge
		blocks_meta_modified{modified="replica-label-removed"} 0

		# HELP blocks_meta_sync_failures_total Total blocks metadata synchronization failures
		# TYPE blocks_meta_sync_failures_total counter
		blocks_meta_sync_failures_total 0

		# HELP blocks_meta_synced Number of block metadata synced
		# TYPE blocks_meta_synced gauge
		blocks_meta_synced{state="corrupted-bucket-index"} 0
		blocks_meta_synced{state="corrupted-meta-json"} 0
		blocks_meta_synced{state="duplicate"} 0
		blocks_meta_synced{state="failed"} 0
		blocks_meta_synced{state="label-excluded"} 0
		blocks_meta_synced{state="loaded"} 0
		blocks_meta_synced{state="marked-for-deletion"} 0
		blocks_meta_synced{state="marked-for-no-compact"} 0
		blocks_meta_synced{state="no-bucket-index"} 1
		blocks_meta_synced{state="no-meta-json"} 0
		blocks_meta_synced{state="parquet-migrated"} 0
		blocks_meta_synced{state="time-excluded"} 0
		blocks_meta_synced{state="too-fresh"} 0

		# HELP blocks_meta_syncs_total Total blocks metadata synchronization attempts
		# TYPE blocks_meta_syncs_total counter
		blocks_meta_syncs_total 1
	`),
		"blocks_meta_modified",
		"blocks_meta_sync_failures_total",
		"blocks_meta_synced",
		"blocks_meta_syncs_total",
	))
}

func TestBucketIndexMetadataFetcher_Fetch_CorruptedBucketIndex(t *testing.T) {
	t.Parallel()
	const userID = "user-1"

	bkt, _ := cortex_testutil.PrepareFilesystemBucket(t)
	reg := prometheus.NewPedanticRegistry()
	ctx := context.Background()
	logs := &concurrency.SyncBuffer{}
	logger := log.NewLogfmtLogger(logs)

	// Upload a corrupted bucket index.
	require.NoError(t, bkt.Upload(ctx, path.Join(userID, bucketindex.IndexCompressedFilename), strings.NewReader("invalid}!")))

	fetcher := NewBucketIndexMetadataFetcher(userID, bkt, NewNoShardingStrategy(logger, nil), nil, logger, reg, nil)
	metas, partials, err := fetcher.Fetch(ctx)
	require.NoError(t, err)
	assert.Empty(t, metas)
	assert.Empty(t, partials)
	assert.Regexp(t, "corrupted bucket index found", logs)

	assert.NoError(t, testutil.GatherAndCompare(reg, bytes.NewBufferString(`
		# HELP blocks_meta_modified Number of blocks whose metadata changed
		# TYPE blocks_meta_modified gauge
		blocks_meta_modified{modified="replica-label-removed"} 0

		# HELP blocks_meta_sync_failures_total Total blocks metadata synchronization failures
		# TYPE blocks_meta_sync_failures_total counter
		blocks_meta_sync_failures_total 0

		# HELP blocks_meta_synced Number of block metadata synced
		# TYPE blocks_meta_synced gauge
		blocks_meta_synced{state="corrupted-bucket-index"} 1
		blocks_meta_synced{state="corrupted-meta-json"} 0
		blocks_meta_synced{state="duplicate"} 0
		blocks_meta_synced{state="failed"} 0
		blocks_meta_synced{state="label-excluded"} 0
		blocks_meta_synced{state="loaded"} 0
		blocks_meta_synced{state="marked-for-deletion"} 0
		blocks_meta_synced{state="marked-for-no-compact"} 0
		blocks_meta_synced{state="no-bucket-index"} 0
		blocks_meta_synced{state="no-meta-json"} 0
		blocks_meta_synced{state="parquet-migrated"} 0
		blocks_meta_synced{state="time-excluded"} 0
		blocks_meta_synced{state="too-fresh"} 0

		# HELP blocks_meta_syncs_total Total blocks metadata synchronization attempts
		# TYPE blocks_meta_syncs_total counter
		blocks_meta_syncs_total 1
	`),
		"blocks_meta_modified",
		"blocks_meta_sync_failures_total",
		"blocks_meta_synced",
		"blocks_meta_syncs_total",
	))
}

func TestBucketIndexMetadataFetcher_Fetch_ShouldResetGaugeMetrics(t *testing.T) {
	t.Parallel()
	const userID = "user-1"

	bkt, _ := cortex_testutil.PrepareFilesystemBucket(t)
	reg := prometheus.NewPedanticRegistry()
	ctx := context.Background()
	now := time.Now()
	logger := log.NewNopLogger()
	strategy := &mockShardingStrategy{}
	strategy.On("FilterUsers", mock.Anything, mock.Anything).Return([]string{userID})

	// Corrupted bucket index.
	require.NoError(t, bkt.Upload(ctx, path.Join(userID, bucketindex.IndexCompressedFilename), strings.NewReader("invalid}!")))

	fetcher := NewBucketIndexMetadataFetcher(userID, bkt, strategy, nil, logger, reg, nil)
	metas, _, err := fetcher.Fetch(ctx)
	require.NoError(t, err)
	assert.Len(t, metas, 0)

	assert.NoError(t, testutil.GatherAndCompare(reg, bytes.NewBufferString(`
		# HELP blocks_meta_synced Number of block metadata synced
		# TYPE blocks_meta_synced gauge
		blocks_meta_synced{state="corrupted-bucket-index"} 1
		blocks_meta_synced{state="corrupted-meta-json"} 0
		blocks_meta_synced{state="duplicate"} 0
		blocks_meta_synced{state="failed"} 0
		blocks_meta_synced{state="label-excluded"} 0
		blocks_meta_synced{state="loaded"} 0
		blocks_meta_synced{state="marked-for-deletion"} 0
		blocks_meta_synced{state="marked-for-no-compact"} 0
		blocks_meta_synced{state="no-bucket-index"} 0
		blocks_meta_synced{state="no-meta-json"} 0
		blocks_meta_synced{state="parquet-migrated"} 0
		blocks_meta_synced{state="time-excluded"} 0
		blocks_meta_synced{state="too-fresh"} 0
	`), "blocks_meta_synced"))

	// No bucket index.
	require.NoError(t, bucketindex.DeleteIndex(ctx, bkt, userID, nil))

	metas, _, err = fetcher.Fetch(ctx)
	require.NoError(t, err)
	assert.Len(t, metas, 0)

	assert.NoError(t, testutil.GatherAndCompare(reg, bytes.NewBufferString(`
		# HELP blocks_meta_synced Number of block metadata synced
		# TYPE blocks_meta_synced gauge
		blocks_meta_synced{state="corrupted-bucket-index"} 0
		blocks_meta_synced{state="corrupted-meta-json"} 0
		blocks_meta_synced{state="duplicate"} 0
		blocks_meta_synced{state="failed"} 0
		blocks_meta_synced{state="label-excluded"} 0
		blocks_meta_synced{state="loaded"} 0
		blocks_meta_synced{state="marked-for-deletion"} 0
		blocks_meta_synced{state="marked-for-no-compact"} 0
		blocks_meta_synced{state="no-bucket-index"} 1
		blocks_meta_synced{state="no-meta-json"} 0
		blocks_meta_synced{state="parquet-migrated"} 0
		blocks_meta_synced{state="time-excluded"} 0
		blocks_meta_synced{state="too-fresh"} 0
	`), "blocks_meta_synced"))

	// Create a bucket index.
	block1 := &bucketindex.Block{ID: ulid.MustNew(1, nil)}
	block2 := &bucketindex.Block{ID: ulid.MustNew(2, nil)}
	block3 := &bucketindex.Block{ID: ulid.MustNew(3, nil)}

	require.NoError(t, bucketindex.WriteIndex(ctx, bkt, userID, nil, &bucketindex.Index{
		Version:   bucketindex.IndexVersion1,
		Blocks:    bucketindex.Blocks{block1, block2, block3},
		UpdatedAt: now.Unix(),
	}))

	metas, _, err = fetcher.Fetch(ctx)
	require.NoError(t, err)
	assert.Len(t, metas, 3)

	assert.NoError(t, testutil.GatherAndCompare(reg, bytes.NewBufferString(`
		# HELP blocks_meta_synced Number of block metadata synced
		# TYPE blocks_meta_synced gauge
		blocks_meta_synced{state="corrupted-bucket-index"} 0
		blocks_meta_synced{state="corrupted-meta-json"} 0
		blocks_meta_synced{state="duplicate"} 0
		blocks_meta_synced{state="failed"} 0
		blocks_meta_synced{state="label-excluded"} 0
		blocks_meta_synced{state="loaded"} 3
		blocks_meta_synced{state="marked-for-deletion"} 0
		blocks_meta_synced{state="marked-for-no-compact"} 0
		blocks_meta_synced{state="no-bucket-index"} 0
		blocks_meta_synced{state="no-meta-json"} 0
		blocks_meta_synced{state="parquet-migrated"} 0
		blocks_meta_synced{state="time-excluded"} 0
		blocks_meta_synced{state="too-fresh"} 0
	`), "blocks_meta_synced"))

	// Remove the tenant from the shard.
	strategy = &mockShardingStrategy{}
	strategy.On("FilterUsers", mock.Anything, mock.Anything).Return([]string{})
	fetcher.strategy = strategy

	metas, _, err = fetcher.Fetch(ctx)
	require.NoError(t, err)
	assert.Len(t, metas, 0)

	assert.NoError(t, testutil.GatherAndCompare(reg, bytes.NewBufferString(`
		# HELP blocks_meta_synced Number of block metadata synced
		# TYPE blocks_meta_synced gauge
		blocks_meta_synced{state="corrupted-bucket-index"} 0
		blocks_meta_synced{state="corrupted-meta-json"} 0
		blocks_meta_synced{state="duplicate"} 0
		blocks_meta_synced{state="failed"} 0
		blocks_meta_synced{state="label-excluded"} 0
		blocks_meta_synced{state="loaded"} 0
		blocks_meta_synced{state="marked-for-deletion"} 0
		blocks_meta_synced{state="marked-for-no-compact"} 0
		blocks_meta_synced{state="no-bucket-index"} 0
		blocks_meta_synced{state="no-meta-json"} 0
		blocks_meta_synced{state="parquet-migrated"} 0
		blocks_meta_synced{state="time-excluded"} 0
		blocks_meta_synced{state="too-fresh"} 0
	`), "blocks_meta_synced"))
}
