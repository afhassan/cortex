package compactor

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/oklog/ulid/v2"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
	"github.com/thanos-io/thanos/pkg/block"
	"github.com/thanos-io/thanos/pkg/block/metadata"
	"github.com/thanos-io/thanos/pkg/compact"
	"gopkg.in/yaml.v2"

	"github.com/cortexproject/cortex/pkg/ring"
	"github.com/cortexproject/cortex/pkg/ring/kv/consul"
	"github.com/cortexproject/cortex/pkg/storage/bucket"
	"github.com/cortexproject/cortex/pkg/storage/parquet"
	cortex_tsdb "github.com/cortexproject/cortex/pkg/storage/tsdb"
	"github.com/cortexproject/cortex/pkg/storage/tsdb/bucketindex"
	cortex_storage_testutil "github.com/cortexproject/cortex/pkg/storage/tsdb/testutil"
	"github.com/cortexproject/cortex/pkg/storage/tsdb/users"
	"github.com/cortexproject/cortex/pkg/util"
	"github.com/cortexproject/cortex/pkg/util/concurrency"
	"github.com/cortexproject/cortex/pkg/util/flagext"
	"github.com/cortexproject/cortex/pkg/util/services"
	cortex_testutil "github.com/cortexproject/cortex/pkg/util/test"
	"github.com/cortexproject/cortex/pkg/util/validation"
)

func TestConfig_ShouldSupportYamlConfig(t *testing.T) {
	yamlCfg := `
block_ranges: [2h, 48h]
consistency_delay: 1h
block_sync_concurrency: 123
data_dir: /tmp
compaction_interval: 15m
compaction_retries: 123
`

	cfg := Config{}
	flagext.DefaultValues(&cfg)
	assert.NoError(t, yaml.Unmarshal([]byte(yamlCfg), &cfg))
	assert.Equal(t, cortex_tsdb.DurationList{2 * time.Hour, 48 * time.Hour}, cfg.BlockRanges)
	assert.Equal(t, time.Hour, cfg.ConsistencyDelay)
	assert.Equal(t, 123, cfg.BlockSyncConcurrency)
	assert.Equal(t, "/tmp", cfg.DataDir)
	assert.Equal(t, 15*time.Minute, cfg.CompactionInterval)
	assert.Equal(t, 123, cfg.CompactionRetries)
}

func TestConfig_ShouldSupportCliFlags(t *testing.T) {
	fs := flag.NewFlagSet("", flag.PanicOnError)
	cfg := Config{}
	cfg.RegisterFlags(fs)
	require.NoError(t, fs.Parse([]string{
		"-compactor.block-ranges=2h,48h",
		"-compactor.consistency-delay=1h",
		"-compactor.block-sync-concurrency=123",
		"-compactor.data-dir=/tmp",
		"-compactor.compaction-interval=15m",
		"-compactor.compaction-retries=123",
	}))

	assert.Equal(t, cortex_tsdb.DurationList{2 * time.Hour, 48 * time.Hour}, cfg.BlockRanges)
	assert.Equal(t, time.Hour, cfg.ConsistencyDelay)
	assert.Equal(t, 123, cfg.BlockSyncConcurrency)
	assert.Equal(t, "/tmp", cfg.DataDir)
	assert.Equal(t, 15*time.Minute, cfg.CompactionInterval)
	assert.Equal(t, 123, cfg.CompactionRetries)
}

func TestConfig_Validate(t *testing.T) {
	tests := map[string]struct {
		setup      func(cfg *Config)
		initLimits func(*validation.Limits)
		expected   string
	}{
		"should pass with the default config": {
			setup:      func(cfg *Config) {},
			initLimits: func(_ *validation.Limits) {},
			expected:   "",
		},
		"should pass with only 1 block range period": {
			setup: func(cfg *Config) {
				cfg.BlockRanges = cortex_tsdb.DurationList{time.Hour}
			},
			initLimits: func(_ *validation.Limits) {},
			expected:   "",
		},
		"should fail with non divisible block range periods": {
			setup: func(cfg *Config) {
				cfg.BlockRanges = cortex_tsdb.DurationList{2 * time.Hour, 12 * time.Hour, 24 * time.Hour, 30 * time.Hour}
			},

			initLimits: func(_ *validation.Limits) {},
			expected:   errors.Errorf(errInvalidBlockRanges, 30*time.Hour, 24*time.Hour).Error(),
		},
		"should fail with duration values of zero": {
			setup: func(cfg *Config) {
				cfg.BlockRanges = cortex_tsdb.DurationList{2 * time.Hour, 0, 24 * time.Hour, 30 * time.Hour}
			},
			initLimits: func(_ *validation.Limits) {},
			expected:   errors.Errorf("compactor block range period cannot be zero").Error(),
		},
		"should pass with valid shuffle sharding config": {
			setup: func(cfg *Config) {
				cfg.ShardingStrategy = util.ShardingStrategyShuffle
				cfg.ShardingEnabled = true
			},
			initLimits: func(limits *validation.Limits) {
				limits.CompactorTenantShardSize = 1
			},
			expected: "",
		},
		"should fail with bad compactor tenant shard size": {
			setup: func(cfg *Config) {
				cfg.ShardingStrategy = util.ShardingStrategyShuffle
				cfg.ShardingEnabled = true
			},
			initLimits: func(_ *validation.Limits) {},
			expected:   errInvalidTenantShardSize.Error(),
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			cfg := &Config{}
			limits := validation.Limits{}
			flagext.DefaultValues(cfg, &limits)
			testData.setup(cfg)
			testData.initLimits(&limits)

			if actualErr := cfg.Validate(limits); testData.expected != "" {
				assert.EqualError(t, actualErr, testData.expected)
			} else {
				assert.NoError(t, actualErr)
			}
		})
	}
}

func TestCompactor_SkipCompactionWhenCmkError(t *testing.T) {
	t.Parallel()
	userID := "user-1"

	ss := bucketindex.Status{Status: bucketindex.CustomerManagedKeyError, Version: bucketindex.SyncStatusFileVersion}
	content, err := json.Marshal(ss)
	require.NoError(t, err)

	// No user blocks stored in the bucket.
	bucketClient := &bucket.ClientMock{}
	bucketClient.MockGet(users.UserIndexCompressedFilename, "", nil)
	bucketClient.MockIter("", []string{userID}, nil)
	bucketClient.MockIter("__markers__", []string{}, nil)
	bucketClient.MockIter(userID+"/", []string{userID + "/01DTVP434PA9VFXSW2JKB3392D"}, nil)
	bucketClient.MockGet(userID+"/01DTVP434PA9VFXSW2JKB3392D/meta.json", mockBlockMetaJSON("01DTVP434PA9VFXSW2JKB3392D"), nil)
	bucketClient.MockIter(userID+"/markers/", nil, nil)
	bucketClient.MockGet(userID+"/markers/cleaner-visit-marker.json", "", nil)
	bucketClient.MockUpload(userID+"/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockDelete(userID+"/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockGet(userID+"/bucket-index-sync-status.json", string(content), nil)
	bucketClient.MockGet(userID+"/bucket-index.json.gz", "", nil)
	bucketClient.MockUpload(userID+"/bucket-index-sync-status.json", nil)
	bucketClient.MockUpload(userID+"/bucket-index.json.gz", nil)
	bucketClient.MockExists(cortex_tsdb.GetGlobalDeletionMarkPath(userID), false, nil)
	bucketClient.MockExists(cortex_tsdb.GetLocalDeletionMarkPath(userID), false, nil)

	cfg := prepareConfig()
	c, _, _, logs, _ := prepare(t, cfg, bucketClient, nil)
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))

	// Wait until a run has completed.
	cortex_testutil.Poll(t, time.Second, 1.0, func() interface{} {
		return prom_testutil.ToFloat64(c.CompactionRunsCompleted)
	})

	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), c))
	assert.Contains(t, strings.Split(strings.TrimSpace(logs.String()), "\n"), `level=info component=compactor msg="skipping compactUser due CustomerManagedKeyError" user=user-1`)
}

func TestCompactor_ShouldDoNothingOnNoUserBlocks(t *testing.T) {
	t.Parallel()

	// No user blocks stored in the bucket.
	bucketClient := &bucket.ClientMock{}
	bucketClient.MockGet(users.UserIndexCompressedFilename, "", nil)
	bucketClient.MockIter("", []string{}, nil)
	bucketClient.MockIter("__markers__", []string{}, nil)
	cfg := prepareConfig()
	c, _, _, logs, registry := prepare(t, cfg, bucketClient, nil)
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))

	// Wait until a run has completed.
	cortex_testutil.Poll(t, time.Second, 1.0, func() interface{} {
		return prom_testutil.ToFloat64(c.CompactionRunsCompleted)
	})

	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), c))

	assert.Equal(t, prom_testutil.ToFloat64(c.CompactionRunInterval), cfg.CompactionInterval.Seconds())

	assert.ElementsMatch(t, []string{
		`level=info component=compactor msg="compactor started"`,
		`level=info component=compactor msg="discovering users from bucket"`,
		`level=info component=compactor msg="discovered users from bucket" users=0`,
	}, removeIgnoredLogs(strings.Split(strings.TrimSpace(logs.String()), "\n")))

	assert.NoError(t, prom_testutil.GatherAndCompare(registry, strings.NewReader(`
		# TYPE cortex_compactor_runs_started_total counter
		# HELP cortex_compactor_runs_started_total Total number of compaction runs started.
		cortex_compactor_runs_started_total 1

		# TYPE cortex_compactor_runs_completed_total counter
		# HELP cortex_compactor_runs_completed_total Total number of compaction runs successfully completed.
		cortex_compactor_runs_completed_total 1

		# TYPE cortex_compactor_runs_failed_total counter
		# HELP cortex_compactor_runs_failed_total Total number of compaction runs failed.
		cortex_compactor_runs_failed_total 0

		# TYPE cortex_compactor_block_cleanup_failures_total counter
		# HELP cortex_compactor_block_cleanup_failures_total Total number of blocks failed to be deleted.
		cortex_compactor_block_cleanup_failures_total 0

		# HELP cortex_compactor_blocks_cleaned_total Total number of blocks deleted.
		# TYPE cortex_compactor_blocks_cleaned_total counter
		cortex_compactor_blocks_cleaned_total 0
		# HELP cortex_compactor_blocks_marked_for_no_compaction_total Total number of blocks marked for no compact during a compaction run.
		# TYPE cortex_compactor_blocks_marked_for_no_compaction_total counter
		cortex_compactor_blocks_marked_for_no_compaction_total 0

		# HELP cortex_compactor_meta_sync_consistency_delay_seconds Configured consistency delay in seconds.
		# TYPE cortex_compactor_meta_sync_consistency_delay_seconds gauge
		cortex_compactor_meta_sync_consistency_delay_seconds 0

		# TYPE cortex_compactor_block_cleanup_started_total counter
		# HELP cortex_compactor_block_cleanup_started_total Total number of blocks cleanup runs started.
		cortex_compactor_block_cleanup_started_total{user_status="active"} 1
		cortex_compactor_block_cleanup_started_total{user_status="deleted"} 1

		# TYPE cortex_compactor_block_cleanup_completed_total counter
		# HELP cortex_compactor_block_cleanup_completed_total Total number of blocks cleanup runs successfully completed.
		cortex_compactor_block_cleanup_completed_total{user_status="active"} 1
		cortex_compactor_block_cleanup_completed_total{user_status="deleted"} 1
	`),
		"cortex_compactor_runs_started_total",
		"cortex_compactor_runs_completed_total",
		"cortex_compactor_runs_failed_total",
		"cortex_compactor_block_cleanup_failures_total",
		"cortex_compactor_blocks_cleaned_total",
		"cortex_compactor_blocks_marked_for_no_compaction_total",
		"cortex_compactor_meta_sync_consistency_delay_seconds",
		"cortex_compactor_block_cleanup_started_total",
		"cortex_compactor_block_cleanup_completed_total",
	))
}

func TestCompactor_ShouldRetryCompactionOnFailureWhileDiscoveringUsersFromBucket(t *testing.T) {
	t.Parallel()

	// Fail to iterate over the bucket while discovering users.
	bucketClient := &bucket.ClientMock{}
	bucketClient.MockGet(users.UserIndexCompressedFilename, "", nil)
	bucketClient.MockIter("__markers__", nil, errors.New("failed to iterate the bucket"))
	bucketClient.MockIter("", nil, errors.New("failed to iterate the bucket"))

	c, _, _, logs, registry := prepare(t, prepareConfig(), bucketClient, nil)
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))

	// Wait until all retry attempts have completed.
	cortex_testutil.Poll(t, time.Second, 1.0, func() interface{} {
		return prom_testutil.ToFloat64(c.CompactionRunsFailed)
	})

	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), c))

	// Ensure the bucket iteration has been retried the configured number of times.
	bucketClient.AssertNumberOfCalls(t, "Iter", 1+3)

	assert.ElementsMatch(t, []string{
		`level=error component=cleaner msg="failed to scan users on startup" err="failed to discover users from bucket: failed to iterate the bucket"`,
		`level=info component=compactor msg="compactor started"`,
		`level=info component=compactor msg="discovering users from bucket"`,
		`level=error component=compactor msg="failed to discover users from bucket" err="failed to iterate the bucket"`,
	}, removeIgnoredLogs(strings.Split(strings.TrimSpace(logs.String()), "\n")))

	assert.NoError(t, prom_testutil.GatherAndCompare(registry, strings.NewReader(`
		# TYPE cortex_compactor_runs_started_total counter
		# HELP cortex_compactor_runs_started_total Total number of compaction runs started.
		cortex_compactor_runs_started_total 1

		# TYPE cortex_compactor_runs_completed_total counter
		# HELP cortex_compactor_runs_completed_total Total number of compaction runs successfully completed.
		cortex_compactor_runs_completed_total 0

		# TYPE cortex_compactor_runs_failed_total counter
		# HELP cortex_compactor_runs_failed_total Total number of compaction runs failed.
		cortex_compactor_runs_failed_total 1

		# TYPE cortex_compactor_block_cleanup_failures_total counter
		# HELP cortex_compactor_block_cleanup_failures_total Total number of blocks failed to be deleted.
		cortex_compactor_block_cleanup_failures_total 0

		# HELP cortex_compactor_blocks_cleaned_total Total number of blocks deleted.
		# TYPE cortex_compactor_blocks_cleaned_total counter
		cortex_compactor_blocks_cleaned_total 0
		# HELP cortex_compactor_blocks_marked_for_no_compaction_total Total number of blocks marked for no compact during a compaction run.
		# TYPE cortex_compactor_blocks_marked_for_no_compaction_total counter
		cortex_compactor_blocks_marked_for_no_compaction_total 0

		# HELP cortex_compactor_meta_sync_consistency_delay_seconds Configured consistency delay in seconds.
		# TYPE cortex_compactor_meta_sync_consistency_delay_seconds gauge
		cortex_compactor_meta_sync_consistency_delay_seconds 0

		# TYPE cortex_compactor_block_cleanup_failed_total counter
		# HELP cortex_compactor_block_cleanup_failed_total Total number of blocks cleanup runs failed.
		cortex_compactor_block_cleanup_failed_total{user_status="active"} 1
		cortex_compactor_block_cleanup_failed_total{user_status="deleted"} 1
	`),
		"cortex_compactor_runs_started_total",
		"cortex_compactor_runs_completed_total",
		"cortex_compactor_runs_failed_total",
		"cortex_compactor_block_cleanup_failures_total",
		"cortex_compactor_blocks_cleaned_total",
		"cortex_compactor_blocks_marked_for_no_compaction_total",
		"cortex_compactor_meta_sync_consistency_delay_seconds",
		"cortex_compactor_block_cleanup_failed_total",
	))
}

func TestCompactor_ShouldIncrementCompactionErrorIfFailedToCompactASingleTenant(t *testing.T) {
	t.Parallel()

	userID := "test-user"
	bucketClient := &bucket.ClientMock{}
	bucketClient.MockGet(users.UserIndexCompressedFilename, "", nil)
	bucketClient.MockIter("", []string{userID}, nil)
	bucketClient.MockIter("__markers__", []string{}, nil)
	bucketClient.MockIter(userID+"/", []string{userID + "/01DTVP434PA9VFXSW2JKB3392D", userID + "/01FN6CDF3PNEWWRY5MPGJPE3EX", userID + "/01DTVP434PA9VFXSW2JKB3392D/meta.json", userID + "/01FN6CDF3PNEWWRY5MPGJPE3EX/meta.json"}, nil)
	bucketClient.MockIter(userID+"/markers/", nil, nil)
	bucketClient.MockGet(userID+"/markers/cleaner-visit-marker.json", "", nil)
	bucketClient.MockUpload(userID+"/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockDelete(userID+"/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockExists(cortex_tsdb.GetGlobalDeletionMarkPath(userID), false, nil)
	bucketClient.MockExists(cortex_tsdb.GetLocalDeletionMarkPath(userID), false, nil)
	bucketClient.MockGet(userID+"/01DTVP434PA9VFXSW2JKB3392D/meta.json", mockBlockMetaJSON("01DTVP434PA9VFXSW2JKB3392D"), nil)
	bucketClient.MockGet(userID+"/01DTVP434PA9VFXSW2JKB3392D/no-compact-mark.json", "", nil)
	bucketClient.MockGet(userID+"/01DTVP434PA9VFXSW2JKB3392D/deletion-mark.json", "", nil)
	bucketClient.MockGet(userID+"/01DTVP434PA9VFXSW2JKB3392D/visit-mark.json", "", nil)
	bucketClient.MockGet(userID+"/bucket-index-sync-status.json", "", nil)
	bucketClient.MockUpload(userID+"/01DTVP434PA9VFXSW2JKB3392D/visit-mark.json", nil)
	bucketClient.MockGet(userID+"/01FN6CDF3PNEWWRY5MPGJPE3EX/meta.json", mockBlockMetaJSON("01FN6CDF3PNEWWRY5MPGJPE3EX"), nil)
	bucketClient.MockGet(userID+"/01FN6CDF3PNEWWRY5MPGJPE3EX/no-compact-mark.json", "", nil)
	bucketClient.MockGet(userID+"/01FN6CDF3PNEWWRY5MPGJPE3EX/deletion-mark.json", "", nil)
	bucketClient.MockGet(userID+"/01FN6CDF3PNEWWRY5MPGJPE3EX/visit-mark.json", "", nil)
	bucketClient.MockUpload(userID+"/01FN6CDF3PNEWWRY5MPGJPE3EX/visit-mark.json", nil)
	bucketClient.MockGet(userID+"/bucket-index.json.gz", "", nil)
	bucketClient.MockUpload(userID+"/bucket-index.json.gz", nil)
	bucketClient.MockUpload(userID+"/bucket-index-sync-status.json", nil)

	c, _, tsdbPlannerMock, _, registry := prepare(t, prepareConfig(), bucketClient, nil)
	tsdbPlannerMock.On("Plan", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*metadata.Meta{}, errors.New("Failed to plan"))
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))

	// Wait until all retry attempts have completed.
	cortex_testutil.Poll(t, time.Second, 1.0, func() interface{} {
		return prom_testutil.ToFloat64(c.CompactionRunsFailed)
	})

	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), c))

	assert.NoError(t, prom_testutil.GatherAndCompare(registry, strings.NewReader(`
		# TYPE cortex_compactor_runs_started_total counter
		# HELP cortex_compactor_runs_started_total Total number of compaction runs started.
		cortex_compactor_runs_started_total 1

		# TYPE cortex_compactor_runs_completed_total counter
		# HELP cortex_compactor_runs_completed_total Total number of compaction runs successfully completed.
		cortex_compactor_runs_completed_total 0

		# TYPE cortex_compactor_runs_failed_total counter
		# HELP cortex_compactor_runs_failed_total Total number of compaction runs failed.
		cortex_compactor_runs_failed_total 1
	`),
		"cortex_compactor_runs_started_total",
		"cortex_compactor_runs_completed_total",
		"cortex_compactor_runs_failed_total",
	))
}

func TestCompactor_ShouldCompactAndRemoveUserFolder(t *testing.T) {
	bucketClient := &bucket.ClientMock{}
	bucketClient.MockGet(users.UserIndexCompressedFilename, "", nil)
	bucketClient.MockIter("", []string{"user-1"}, nil)
	bucketClient.MockIter("__markers__", []string{}, nil)
	bucketClient.MockExists(cortex_tsdb.GetGlobalDeletionMarkPath("user-1"), false, nil)
	bucketClient.MockExists(cortex_tsdb.GetLocalDeletionMarkPath("user-1"), false, nil)
	bucketClient.MockIter("user-1/", []string{"user-1/01DTVP434PA9VFXSW2JKB3392D", "user-1/01DTVP434PA9VFXSW2JKB3392D/meta.json", "user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/meta.json"}, nil)
	bucketClient.MockIter("user-1/markers/", nil, nil)
	bucketClient.MockGet("user-1/markers/cleaner-visit-marker.json", "", nil)
	bucketClient.MockUpload("user-1/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockDelete("user-1/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/meta.json", mockBlockMetaJSON("01DTVP434PA9VFXSW2JKB3392D"), nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/deletion-mark.json", "", nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/no-compact-mark.json", "", nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/visit-mark.json", "", nil)
	bucketClient.MockGet("user-1/bucket-index-sync-status.json", "", nil)
	bucketClient.MockGet("user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/meta.json", mockBlockMetaJSON("01FN6CDF3PNEWWRY5MPGJPE3EX"), nil)
	bucketClient.MockGet("user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/deletion-mark.json", "", nil)
	bucketClient.MockGet("user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/no-compact-mark.json", "", nil)
	bucketClient.MockGet("user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/visit-mark.json", "", nil)
	bucketClient.MockGet("user-1/bucket-index.json.gz", "", nil)
	bucketClient.MockGet("user-1/bucket-index-sync-status.json", "", nil)
	bucketClient.MockIter("user-1/markers/", nil, nil)
	bucketClient.MockUpload("user-1/bucket-index.json.gz", nil)
	bucketClient.MockUpload("user-1/bucket-index-sync-status.json", nil)

	c, _, tsdbPlanner, _, _ := prepare(t, prepareConfig(), bucketClient, nil)

	// Make sure the user folder is created and is being used
	// This will be called during compaction
	tsdbPlanner.On("Plan", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		_, err := os.Stat(c.compactDirForUser("user-1"))
		require.NoError(t, err)
	}).Return([]*metadata.Meta{}, nil)

	require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))

	// Wait until a run has completed.
	cortex_testutil.Poll(t, time.Second, 1.0, func() interface{} {
		return prom_testutil.ToFloat64(c.CompactionRunsCompleted)
	})

	_, err := os.Stat(c.compactDirForUser("user-1"))
	require.True(t, os.IsNotExist(err))
}

func TestCompactor_ShouldIterateOverUsersAndRunCompaction(t *testing.T) {
	t.Parallel()

	// Mock the bucket to contain two users, each one with one block.
	bucketClient := &bucket.ClientMock{}
	bucketClient.MockGet(users.UserIndexCompressedFilename, "", nil)
	bucketClient.MockIter("", []string{"user-1", "user-2"}, nil)
	bucketClient.MockIter("__markers__", []string{}, nil)
	bucketClient.MockExists(cortex_tsdb.GetGlobalDeletionMarkPath("user-1"), false, nil)
	bucketClient.MockExists(cortex_tsdb.GetLocalDeletionMarkPath("user-1"), false, nil)
	bucketClient.MockExists(cortex_tsdb.GetGlobalDeletionMarkPath("user-2"), false, nil)
	bucketClient.MockExists(cortex_tsdb.GetLocalDeletionMarkPath("user-2"), false, nil)
	bucketClient.MockIter("user-1/", []string{"user-1/01DTVP434PA9VFXSW2JKB3392D", "user-1/01DTVP434PA9VFXSW2JKB3392D/meta.json", "user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/meta.json"}, nil)
	bucketClient.MockIter("user-2/", []string{"user-2/01DTW0ZCPDDNV4BV83Q2SV4QAZ", "user-2/01DTW0ZCPDDNV4BV83Q2SV4QAZ/meta.json", "user-2/01FN3V83ABR9992RF8WRJZ76ZQ/meta.json"}, nil)
	bucketClient.MockIter("user-1/markers/", nil, nil)
	bucketClient.MockGet("user-1/markers/cleaner-visit-marker.json", "", nil)
	bucketClient.MockUpload("user-1/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockDelete("user-1/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockIter("user-2/markers/", nil, nil)
	bucketClient.MockGet("user-2/markers/cleaner-visit-marker.json", "", nil)
	bucketClient.MockUpload("user-2/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockDelete("user-2/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/meta.json", mockBlockMetaJSON("01DTVP434PA9VFXSW2JKB3392D"), nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/deletion-mark.json", "", nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/no-compact-mark.json", "", nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/visit-mark.json", "", nil)
	bucketClient.MockGet("user-1/bucket-index-sync-status.json", "", nil)
	bucketClient.MockGet("user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/meta.json", mockBlockMetaJSON("01FN6CDF3PNEWWRY5MPGJPE3EX"), nil)
	bucketClient.MockGet("user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/deletion-mark.json", "", nil)
	bucketClient.MockGet("user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/no-compact-mark.json", "", nil)
	bucketClient.MockGet("user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/visit-mark.json", "", nil)
	bucketClient.MockGet("user-2/01DTW0ZCPDDNV4BV83Q2SV4QAZ/meta.json", mockBlockMetaJSON("01DTW0ZCPDDNV4BV83Q2SV4QAZ"), nil)
	bucketClient.MockGet("user-2/01DTW0ZCPDDNV4BV83Q2SV4QAZ/deletion-mark.json", "", nil)
	bucketClient.MockGet("user-2/01DTW0ZCPDDNV4BV83Q2SV4QAZ/no-compact-mark.json", "", nil)
	bucketClient.MockGet("user-2/01DTW0ZCPDDNV4BV83Q2SV4QAZ/visit-mark.json", "", nil)
	bucketClient.MockGet("user-2/01FN3V83ABR9992RF8WRJZ76ZQ/meta.json", mockBlockMetaJSON("01FN3V83ABR9992RF8WRJZ76ZQ"), nil)
	bucketClient.MockGet("user-2/01FN3V83ABR9992RF8WRJZ76ZQ/deletion-mark.json", "", nil)
	bucketClient.MockGet("user-2/01FN3V83ABR9992RF8WRJZ76ZQ/no-compact-mark.json", "", nil)
	bucketClient.MockGet("user-2/01FN3V83ABR9992RF8WRJZ76ZQ/visit-mark.json", "", nil)
	bucketClient.MockGet("user-1/bucket-index.json.gz", "", nil)
	bucketClient.MockGet("user-2/bucket-index.json.gz", "", nil)
	bucketClient.MockGet("user-1/bucket-index-sync-status.json", "", nil)
	bucketClient.MockGet("user-2/bucket-index-sync-status.json", "", nil)
	bucketClient.MockIter("user-1/markers/", nil, nil)
	bucketClient.MockIter("user-2/markers/", nil, nil)
	bucketClient.MockUpload("user-1/bucket-index.json.gz", nil)
	bucketClient.MockUpload("user-2/bucket-index.json.gz", nil)
	bucketClient.MockUpload("user-1/bucket-index-sync-status.json", nil)
	bucketClient.MockUpload("user-2/bucket-index-sync-status.json", nil)

	c, _, tsdbPlanner, logs, registry := prepare(t, prepareConfig(), bucketClient, nil)

	// Mock the planner as if there's no compaction to do,
	// in order to simplify tests (all in all, we just want to
	// test our logic and not TSDB compactor which we expect to
	// be already tested).
	tsdbPlanner.On("Plan", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*metadata.Meta{}, nil)

	require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))

	// Wait until a run has completed.
	cortex_testutil.Poll(t, time.Second, 1.0, func() interface{} {
		return prom_testutil.ToFloat64(c.CompactionRunsCompleted)
	})

	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), c))

	// Ensure a plan has been executed for the blocks of each user.
	tsdbPlanner.AssertNumberOfCalls(t, "Plan", 2)

	assert.Len(t, tsdbPlanner.getNoCompactBlocks(), 0)

	assert.ElementsMatch(t, []string{
		`level=info component=compactor msg="compactor started"`,
		`level=info component=compactor msg="discovering users from bucket"`,
		`level=info component=compactor msg="discovered users from bucket" users=2`,
		`level=info component=compactor msg="starting compaction of user blocks" user=user-1`,
		`level=info component=compactor org_id=user-1 msg="start sync of metas"`,
		`level=info component=compactor org_id=user-1 msg="start of GC"`,
		`level=info component=compactor org_id=user-1 msg="start of compactions"`,
		`level=info component=compactor org_id=user-1 msg="compaction iterations done"`,
		`level=info component=compactor msg="successfully compacted user blocks" user=user-1`,
		`level=info component=compactor msg="starting compaction of user blocks" user=user-2`,
		`level=info component=compactor org_id=user-2 msg="start sync of metas"`,
		`level=info component=compactor org_id=user-2 msg="start of GC"`,
		`level=info component=compactor org_id=user-2 msg="start of compactions"`,
		`level=info component=compactor org_id=user-2 msg="compaction iterations done"`,
		`level=info component=compactor msg="successfully compacted user blocks" user=user-2`,
	}, removeIgnoredLogs(strings.Split(strings.TrimSpace(logs.String()), "\n")))

	// Instead of testing for shipper metrics, we only check our metrics here.
	// Real shipper metrics are too variable to embed into a test.
	testedMetrics := []string{
		"cortex_compactor_runs_started_total", "cortex_compactor_runs_completed_total", "cortex_compactor_runs_failed_total",
		"cortex_compactor_blocks_cleaned_total", "cortex_compactor_block_cleanup_failures_total", "cortex_compactor_blocks_marked_for_deletion_total",
		"cortex_compactor_block_cleanup_started_total", "cortex_compactor_block_cleanup_completed_total",
		"cortex_compactor_blocks_marked_for_no_compaction_total",
	}
	assert.NoError(t, prom_testutil.GatherAndCompare(registry, strings.NewReader(`
		# TYPE cortex_compactor_runs_started_total counter
		# HELP cortex_compactor_runs_started_total Total number of compaction runs started.
		cortex_compactor_runs_started_total 1

		# TYPE cortex_compactor_runs_completed_total counter
		# HELP cortex_compactor_runs_completed_total Total number of compaction runs successfully completed.
		cortex_compactor_runs_completed_total 1

		# TYPE cortex_compactor_runs_failed_total counter
		# HELP cortex_compactor_runs_failed_total Total number of compaction runs failed.
		cortex_compactor_runs_failed_total 0

		# TYPE cortex_compactor_block_cleanup_failures_total counter
		# HELP cortex_compactor_block_cleanup_failures_total Total number of blocks failed to be deleted.
		cortex_compactor_block_cleanup_failures_total 0

		# HELP cortex_compactor_blocks_cleaned_total Total number of blocks deleted.
		# TYPE cortex_compactor_blocks_cleaned_total counter
		cortex_compactor_blocks_cleaned_total 0

		# HELP cortex_compactor_blocks_marked_for_deletion_total Total number of blocks marked for deletion in compactor.
		# TYPE cortex_compactor_blocks_marked_for_deletion_total counter
		cortex_compactor_blocks_marked_for_deletion_total{reason="compaction",user="user-1"} 0
		cortex_compactor_blocks_marked_for_deletion_total{reason="compaction",user="user-2"} 0
		cortex_compactor_blocks_marked_for_deletion_total{reason="retention",user="user-1"} 0
		cortex_compactor_blocks_marked_for_deletion_total{reason="retention",user="user-2"} 0

		# TYPE cortex_compactor_block_cleanup_started_total counter
		# HELP cortex_compactor_block_cleanup_started_total Total number of blocks cleanup runs started.
		cortex_compactor_block_cleanup_started_total{user_status="active"} 1
		cortex_compactor_block_cleanup_started_total{user_status="deleted"} 1

		# TYPE cortex_compactor_block_cleanup_completed_total counter
		# HELP cortex_compactor_block_cleanup_completed_total Total number of blocks cleanup runs successfully completed.
		cortex_compactor_block_cleanup_completed_total{user_status="active"} 1
		cortex_compactor_block_cleanup_completed_total{user_status="deleted"} 1

		# HELP cortex_compactor_blocks_marked_for_no_compaction_total Total number of blocks marked for no compact during a compaction run.
		# TYPE cortex_compactor_blocks_marked_for_no_compaction_total counter
		cortex_compactor_blocks_marked_for_no_compaction_total 0
	`), testedMetrics...))
}

func TestCompactor_ShouldNotCompactBlocksMarkedForDeletion(t *testing.T) {
	t.Parallel()

	cfg := prepareConfig()
	cfg.DeletionDelay = 10 * time.Minute // Delete block after 10 minutes

	// Mock the bucket to contain two users, each one with one block.
	bucketClient := &bucket.ClientMock{}
	bucketClient.MockGet(users.UserIndexCompressedFilename, "", nil)
	bucketClient.MockIter("", []string{"user-1"}, nil)
	bucketClient.MockIter("__markers__", []string{}, nil)
	bucketClient.MockIter("user-1/", []string{"user-1/01DTVP434PA9VFXSW2JKB3392D", "user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ"}, nil)
	bucketClient.MockExists(cortex_tsdb.GetGlobalDeletionMarkPath("user-1"), false, nil)
	bucketClient.MockExists(cortex_tsdb.GetLocalDeletionMarkPath("user-1"), false, nil)

	// Block that has just been marked for deletion. It will not be deleted just yet, and it also will not be compacted.
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/meta.json", mockBlockMetaJSON("01DTVP434PA9VFXSW2JKB3392D"), nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/deletion-mark.json", mockDeletionMarkJSON("01DTVP434PA9VFXSW2JKB3392D", time.Now()), nil)
	bucketClient.MockGet("user-1/markers/01DTVP434PA9VFXSW2JKB3392D-deletion-mark.json", mockDeletionMarkJSON("01DTVP434PA9VFXSW2JKB3392D", time.Now()), nil)
	bucketClient.MockGet("user-1/bucket-index-sync-status.json", "", nil)

	// This block will be deleted by cleaner.
	bucketClient.MockGet("user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/meta.json", mockBlockMetaJSON("01DTW0ZCPDDNV4BV83Q2SV4QAZ"), nil)
	bucketClient.MockGet("user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/deletion-mark.json", mockDeletionMarkJSON("01DTW0ZCPDDNV4BV83Q2SV4QAZ", time.Now().Add(-cfg.DeletionDelay)), nil)
	bucketClient.MockGet("user-1/markers/01DTW0ZCPDDNV4BV83Q2SV4QAZ-deletion-mark.json", mockDeletionMarkJSON("01DTW0ZCPDDNV4BV83Q2SV4QAZ", time.Now().Add(-cfg.DeletionDelay)), nil)

	bucketClient.MockIter("user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ", []string{
		"user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/meta.json",
		"user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/deletion-mark.json",
	}, nil)

	bucketClient.MockIter("user-1/markers/", []string{
		"user-1/markers/01DTVP434PA9VFXSW2JKB3392D-deletion-mark.json",
		"user-1/markers/01DTW0ZCPDDNV4BV83Q2SV4QAZ-deletion-mark.json",
	}, nil)

	bucketClient.MockGet("user-1/markers/cleaner-visit-marker.json", "", nil)
	bucketClient.MockUpload("user-1/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockDelete("user-1/markers/cleaner-visit-marker.json", nil)

	bucketClient.MockDelete("user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/meta.json", nil)
	bucketClient.MockDelete("user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/deletion-mark.json", nil)
	bucketClient.MockDelete("user-1/markers/01DTW0ZCPDDNV4BV83Q2SV4QAZ-deletion-mark.json", nil)
	bucketClient.MockDelete("user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ", nil)
	bucketClient.MockGet("user-1/bucket-index.json.gz", "", nil)
	bucketClient.MockGet("user-1/bucket-index-sync-status.json", "", nil)
	bucketClient.MockUpload("user-1/bucket-index.json.gz", nil)
	bucketClient.MockUpload("user-1/bucket-index-sync-status.json", nil)

	c, _, tsdbPlanner, logs, registry := prepare(t, cfg, bucketClient, nil)

	require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))

	// Wait until a run has completed.
	cortex_testutil.Poll(t, time.Second, 1.0, func() interface{} {
		return prom_testutil.ToFloat64(c.CompactionRunsCompleted)
	})

	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), c))

	// Since both blocks are marked for deletion, none of them are going to be compacted.
	tsdbPlanner.AssertNumberOfCalls(t, "Plan", 0)

	assert.ElementsMatch(t, []string{
		`level=info component=compactor msg="compactor started"`,
		`level=info component=compactor msg="discovering users from bucket"`,
		`level=info component=compactor msg="discovered users from bucket" users=1`,
		`level=info component=compactor msg="starting compaction of user blocks" user=user-1`,
		`level=info component=compactor org_id=user-1 msg="start sync of metas"`,
		`level=info component=compactor org_id=user-1 msg="start of GC"`,
		`level=info component=compactor org_id=user-1 msg="start of compactions"`,
		`level=info component=compactor org_id=user-1 msg="compaction iterations done"`,
		`level=info component=compactor msg="successfully compacted user blocks" user=user-1`,
	}, removeIgnoredLogs(strings.Split(strings.TrimSpace(logs.String()), "\n")))

	// Instead of testing for shipper metrics, we only check our metrics here.
	// Real shipper metrics are too variable to embed into a test.
	testedMetrics := []string{
		"cortex_compactor_runs_started_total", "cortex_compactor_runs_completed_total", "cortex_compactor_runs_failed_total",
		"cortex_compactor_blocks_cleaned_total", "cortex_compactor_block_cleanup_failures_total", "cortex_compactor_blocks_marked_for_deletion_total",
		"cortex_compactor_block_cleanup_started_total", "cortex_compactor_block_cleanup_completed_total",
		"cortex_compactor_blocks_marked_for_no_compaction_total",
	}
	assert.NoError(t, prom_testutil.GatherAndCompare(registry, strings.NewReader(`
		# TYPE cortex_compactor_runs_started_total counter
		# HELP cortex_compactor_runs_started_total Total number of compaction runs started.
		cortex_compactor_runs_started_total 1

		# TYPE cortex_compactor_runs_completed_total counter
		# HELP cortex_compactor_runs_completed_total Total number of compaction runs successfully completed.
		cortex_compactor_runs_completed_total 1

		# TYPE cortex_compactor_runs_failed_total counter
		# HELP cortex_compactor_runs_failed_total Total number of compaction runs failed.
		cortex_compactor_runs_failed_total 0

		# TYPE cortex_compactor_block_cleanup_failures_total counter
		# HELP cortex_compactor_block_cleanup_failures_total Total number of blocks failed to be deleted.
		cortex_compactor_block_cleanup_failures_total 0

		# HELP cortex_compactor_blocks_cleaned_total Total number of blocks deleted.
		# TYPE cortex_compactor_blocks_cleaned_total counter
		cortex_compactor_blocks_cleaned_total 1

		# HELP cortex_compactor_blocks_marked_for_deletion_total Total number of blocks marked for deletion in compactor.
		# TYPE cortex_compactor_blocks_marked_for_deletion_total counter
		cortex_compactor_blocks_marked_for_deletion_total{reason="compaction",user="user-1"} 0
		cortex_compactor_blocks_marked_for_deletion_total{reason="retention",user="user-1"} 0

		# TYPE cortex_compactor_block_cleanup_started_total counter
		# HELP cortex_compactor_block_cleanup_started_total Total number of blocks cleanup runs started.
		cortex_compactor_block_cleanup_started_total{user_status="active"} 1
		cortex_compactor_block_cleanup_started_total{user_status="deleted"} 1

		# TYPE cortex_compactor_block_cleanup_completed_total counter
		# HELP cortex_compactor_block_cleanup_completed_total Total number of blocks cleanup runs successfully completed.
		cortex_compactor_block_cleanup_completed_total{user_status="active"} 1
		cortex_compactor_block_cleanup_completed_total{user_status="deleted"} 1

		# HELP cortex_compactor_blocks_marked_for_no_compaction_total Total number of blocks marked for no compact during a compaction run.
		# TYPE cortex_compactor_blocks_marked_for_no_compaction_total counter
		cortex_compactor_blocks_marked_for_no_compaction_total 0
	`), testedMetrics...))
}

func TestCompactor_ShouldNotCompactBlocksMarkedForSkipCompact(t *testing.T) {
	t.Parallel()

	// Mock the bucket to contain two users, each one with one block.
	bucketClient := &bucket.ClientMock{}
	bucketClient.MockGet(users.UserIndexCompressedFilename, "", nil)
	bucketClient.MockIter("", []string{"user-1", "user-2"}, nil)
	bucketClient.MockIter("__markers__", []string{}, nil)
	bucketClient.MockExists(cortex_tsdb.GetGlobalDeletionMarkPath("user-1"), false, nil)
	bucketClient.MockExists(cortex_tsdb.GetLocalDeletionMarkPath("user-1"), false, nil)
	bucketClient.MockExists(cortex_tsdb.GetGlobalDeletionMarkPath("user-2"), false, nil)
	bucketClient.MockExists(cortex_tsdb.GetLocalDeletionMarkPath("user-2"), false, nil)
	bucketClient.MockIter("user-1/", []string{"user-1/01DTVP434PA9VFXSW2JKB3392D", "user-1/01DTVP434PA9VFXSW2JKB3392D/meta.json", "user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/meta.json"}, nil)
	bucketClient.MockIter("user-2/", []string{"user-2/01DTW0ZCPDDNV4BV83Q2SV4QAZ", "user-2/01DTW0ZCPDDNV4BV83Q2SV4QAZ/meta.json", "user-2/01FN3V83ABR9992RF8WRJZ76ZQ/meta.json"}, nil)
	bucketClient.MockIter("user-1/markers/", nil, nil)
	bucketClient.MockGet("user-1/markers/cleaner-visit-marker.json", "", nil)
	bucketClient.MockUpload("user-1/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockDelete("user-1/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockIter("user-2/markers/", nil, nil)
	bucketClient.MockGet("user-2/markers/cleaner-visit-marker.json", "", nil)
	bucketClient.MockUpload("user-2/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockDelete("user-2/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/meta.json", mockBlockMetaJSON("01DTVP434PA9VFXSW2JKB3392D"), nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/deletion-mark.json", "", nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/no-compact-mark.json", mockNoCompactBlockJSON("01DTVP434PA9VFXSW2JKB3392D"), nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/visit-mark.json", "", nil)
	bucketClient.MockGet("user-1/bucket-index-sync-status.json", "", nil)
	bucketClient.MockUpload("user-1/01DTVP434PA9VFXSW2JKB3392D/visit-mark.json", nil)
	bucketClient.MockGet("user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/meta.json", mockBlockMetaJSON("01FN6CDF3PNEWWRY5MPGJPE3EX"), nil)
	bucketClient.MockGet("user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/deletion-mark.json", "", nil)
	bucketClient.MockGet("user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/no-compact-mark.json", mockNoCompactBlockJSON("01FN6CDF3PNEWWRY5MPGJPE3EX"), nil)
	bucketClient.MockGet("user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/visit-mark.json", "", nil)
	bucketClient.MockUpload("user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/visit-mark.json", nil)

	bucketClient.MockGet("user-2/01DTW0ZCPDDNV4BV83Q2SV4QAZ/meta.json", mockBlockMetaJSON("01DTW0ZCPDDNV4BV83Q2SV4QAZ"), nil)
	bucketClient.MockGet("user-2/01DTW0ZCPDDNV4BV83Q2SV4QAZ/deletion-mark.json", "", nil)
	bucketClient.MockGet("user-2/01DTW0ZCPDDNV4BV83Q2SV4QAZ/no-compact-mark.json", "", nil)
	bucketClient.MockGet("user-2/01DTW0ZCPDDNV4BV83Q2SV4QAZ/visit-mark.json", "", nil)
	bucketClient.MockUpload("user-2/01DTW0ZCPDDNV4BV83Q2SV4QAZ/visit-mark.json", nil)
	bucketClient.MockGet("user-2/01FN3V83ABR9992RF8WRJZ76ZQ/meta.json", mockBlockMetaJSON("01FN3V83ABR9992RF8WRJZ76ZQ"), nil)
	bucketClient.MockGet("user-2/01FN3V83ABR9992RF8WRJZ76ZQ/deletion-mark.json", "", nil)
	bucketClient.MockGet("user-2/01FN3V83ABR9992RF8WRJZ76ZQ/no-compact-mark.json", "", nil)
	bucketClient.MockGet("user-2/01FN3V83ABR9992RF8WRJZ76ZQ/visit-mark.json", "", nil)
	bucketClient.MockUpload("user-2/01FN3V83ABR9992RF8WRJZ76ZQ/visit-mark.json", nil)

	bucketClient.MockGet("user-1/bucket-index.json.gz", "", nil)
	bucketClient.MockGet("user-2/bucket-index.json.gz", "", nil)
	bucketClient.MockGet("user-1/bucket-index-sync-status.json", "", nil)
	bucketClient.MockGet("user-2/bucket-index-sync-status.json", "", nil)
	bucketClient.MockIter("user-1/markers/", nil, nil)
	bucketClient.MockIter("user-2/markers/", nil, nil)
	bucketClient.MockUpload("user-1/bucket-index.json.gz", nil)
	bucketClient.MockUpload("user-2/bucket-index.json.gz", nil)
	bucketClient.MockUpload("user-1/bucket-index-sync-status.json", nil)
	bucketClient.MockUpload("user-2/bucket-index-sync-status.json", nil)

	c, _, tsdbPlanner, _, registry := prepare(t, prepareConfig(), bucketClient, nil)

	tsdbPlanner.On("Plan", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*metadata.Meta{}, nil)

	require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))

	cortex_testutil.Poll(t, time.Second, 1.0, func() interface{} {
		return prom_testutil.ToFloat64(c.CompactionRunsCompleted)
	})

	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), c))

	// Planner still called for user with all blocks makred for skip compaction.
	tsdbPlanner.AssertNumberOfCalls(t, "Plan", 2)

	assert.ElementsMatch(t, []string{"01DTVP434PA9VFXSW2JKB3392D", "01FN6CDF3PNEWWRY5MPGJPE3EX"}, tsdbPlanner.getNoCompactBlocks())

	testedMetrics := []string{"cortex_compactor_blocks_marked_for_no_compaction_total"}

	assert.NoError(t, prom_testutil.GatherAndCompare(registry, strings.NewReader(`
		# HELP cortex_compactor_blocks_marked_for_no_compaction_total Total number of blocks marked for no compact during a compaction run.
		# TYPE cortex_compactor_blocks_marked_for_no_compaction_total counter
		cortex_compactor_blocks_marked_for_no_compaction_total 0
	`), testedMetrics...))
}

func TestCompactor_ShouldNotCompactBlocksForUsersMarkedForDeletion(t *testing.T) {
	t.Parallel()

	cfg := prepareConfig()
	cfg.DeletionDelay = 10 * time.Minute      // Delete block after 10 minutes
	cfg.TenantCleanupDelay = 10 * time.Minute // To make sure it's not 0.

	// Mock the bucket to contain two users, each one with one block.
	bucketClient := &bucket.ClientMock{}
	bucketClient.MockGet(users.UserIndexCompressedFilename, "", nil)
	bucketClient.MockIter("", []string{"user-1"}, nil)
	bucketClient.MockIter("__markers__", []string{"__markers__/user-1/"}, nil)
	bucketClient.MockIter("user-1/", []string{"user-1/01DTVP434PA9VFXSW2JKB3392D"}, nil)
	bucketClient.MockGet(cortex_tsdb.GetGlobalDeletionMarkPath("user-1"), `{"deletion_time": 1}`, nil)
	bucketClient.MockUpload(cortex_tsdb.GetGlobalDeletionMarkPath("user-1"), nil)

	bucketClient.MockGet("user-1/markers/cleaner-visit-marker.json", "", nil)
	bucketClient.MockUpload("user-1/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockDelete("user-1/markers/cleaner-visit-marker.json", nil)

	bucketClient.MockIter("user-1/01DTVP434PA9VFXSW2JKB3392D", []string{"user-1/01DTVP434PA9VFXSW2JKB3392D/meta.json", "user-1/01DTVP434PA9VFXSW2JKB3392D/index"}, nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/meta.json", mockBlockMetaJSON("01DTVP434PA9VFXSW2JKB3392D"), nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/index", "some index content", nil)
	bucketClient.MockGet("user-1/bucket-index-sync-status.json", "", nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/visit-mark.json", "", nil)
	bucketClient.MockUpload("user-1/01DTVP434PA9VFXSW2JKB3392D/visit-mark.json", nil)
	bucketClient.MockExists("user-1/01DTVP434PA9VFXSW2JKB3392D/deletion-mark.json", false, nil)

	bucketClient.MockDelete("user-1/01DTVP434PA9VFXSW2JKB3392D/meta.json", nil)
	bucketClient.MockDelete("user-1/01DTVP434PA9VFXSW2JKB3392D/index", nil)
	bucketClient.MockDelete("user-1/bucket-index.json.gz", nil)
	bucketClient.MockDelete("user-1/bucket-index-sync-status.json", nil)

	c, _, tsdbPlanner, logs, registry := prepare(t, cfg, bucketClient, nil)

	// Mock the planner as if there's no compaction to do,
	// in order to simplify tests (all in all, we just want to
	// test our logic and not TSDB compactor which we expect to
	// be already tested).
	tsdbPlanner.On("Plan", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*metadata.Meta{}, nil)

	require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))

	// Wait until a run has completed.
	cortex_testutil.Poll(t, time.Second, 1.0, func() interface{} {
		return prom_testutil.ToFloat64(c.CompactionRunsCompleted)
	})

	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), c))

	// No user is compacted, single user we have is marked for deletion.
	tsdbPlanner.AssertNumberOfCalls(t, "Plan", 0)

	assert.ElementsMatch(t, []string{
		`level=info component=compactor msg="compactor started"`,
		`level=info component=compactor msg="discovering users from bucket"`,
		`level=info component=compactor msg="discovered users from bucket" users=1`,
		`level=debug component=compactor msg="skipping user because it is marked for deletion" user=user-1`,
	}, removeIgnoredLogs(strings.Split(strings.TrimSpace(logs.String()), "\n")))

	// Instead of testing for shipper metrics, we only check our metrics here.
	// Real shipper metrics are too variable to embed into a test.
	testedMetrics := []string{
		"cortex_compactor_runs_started_total", "cortex_compactor_runs_completed_total", "cortex_compactor_runs_failed_total",
		"cortex_compactor_blocks_cleaned_total", "cortex_compactor_block_cleanup_failures_total",
		"cortex_compactor_block_cleanup_started_total", "cortex_compactor_block_cleanup_completed_total",
		"cortex_compactor_blocks_marked_for_no_compaction_total",
	}
	assert.NoError(t, prom_testutil.GatherAndCompare(registry, strings.NewReader(`
		# TYPE cortex_compactor_runs_started_total counter
		# HELP cortex_compactor_runs_started_total Total number of compaction runs started.
		cortex_compactor_runs_started_total 1

		# TYPE cortex_compactor_runs_completed_total counter
		# HELP cortex_compactor_runs_completed_total Total number of compaction runs successfully completed.
		cortex_compactor_runs_completed_total 1

		# TYPE cortex_compactor_runs_failed_total counter
		# HELP cortex_compactor_runs_failed_total Total number of compaction runs failed.
		cortex_compactor_runs_failed_total 0

		# TYPE cortex_compactor_block_cleanup_failures_total counter
		# HELP cortex_compactor_block_cleanup_failures_total Total number of blocks failed to be deleted.
		cortex_compactor_block_cleanup_failures_total 0

		# HELP cortex_compactor_blocks_cleaned_total Total number of blocks deleted.
		# TYPE cortex_compactor_blocks_cleaned_total counter
		cortex_compactor_blocks_cleaned_total 1

		# TYPE cortex_compactor_block_cleanup_started_total counter
		# HELP cortex_compactor_block_cleanup_started_total Total number of blocks cleanup runs started.
		cortex_compactor_block_cleanup_started_total{user_status="active"} 1
		cortex_compactor_block_cleanup_started_total{user_status="deleted"} 1

		# TYPE cortex_compactor_block_cleanup_completed_total counter
		# HELP cortex_compactor_block_cleanup_completed_total Total number of blocks cleanup runs successfully completed.
		cortex_compactor_block_cleanup_completed_total{user_status="active"} 1
		cortex_compactor_block_cleanup_completed_total{user_status="deleted"} 1

		# HELP cortex_compactor_blocks_marked_for_no_compaction_total Total number of blocks marked for no compact during a compaction run.
		# TYPE cortex_compactor_blocks_marked_for_no_compaction_total counter
		cortex_compactor_blocks_marked_for_no_compaction_total 0
	`), testedMetrics...))
}

func TestCompactor_ShouldSkipOutOrOrderBlocks(t *testing.T) {
	bucketClient, tmpDir := cortex_storage_testutil.PrepareFilesystemBucket(t)
	bucketClient = bucketindex.BucketWithGlobalMarkers(bucketClient)

	b1 := createTSDBBlock(t, bucketClient, "user-1", 10, 20, map[string]string{"__name__": "Teste"})
	b2 := createTSDBBlock(t, bucketClient, "user-1", 20, 30, map[string]string{"__name__": "Teste"})

	// Read bad index file.
	indexFile, err := os.Open("testdata/out_of_order_chunks/index")
	require.NoError(t, err)
	indexFileStat, err := indexFile.Stat()
	require.NoError(t, err)

	dir := path.Join(tmpDir, "user-1", b1.String())
	outputFile, err := os.OpenFile(path.Join(dir, "index"), os.O_RDWR|os.O_TRUNC, 0755)
	require.NoError(t, err)

	n, err := io.Copy(outputFile, indexFile)
	require.NoError(t, err)
	require.Equal(t, indexFileStat.Size(), n)

	cfg := prepareConfig()
	cfg.SkipBlocksWithOutOfOrderChunksEnabled = true
	c, tsdbCompac, tsdbPlanner, _, registry := prepare(t, cfg, bucketClient, nil)

	tsdbCompac.On("CompactWithBlockPopulator", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]ulid.ULID{b1}, nil)

	tsdbPlanner.On("Plan", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*metadata.Meta{
		{
			BlockMeta: tsdb.BlockMeta{
				ULID:    b1,
				MinTime: 10,
				MaxTime: 20,
			},
		},
		{
			BlockMeta: tsdb.BlockMeta{
				ULID:    b2,
				MinTime: 20,
				MaxTime: 30,
			},
		},
	}, nil)

	require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))

	defer services.StopAndAwaitTerminated(context.Background(), c) //nolint:errcheck

	// Wait until a run has completed.
	cortex_testutil.Poll(t, 5*time.Second, true, func() interface{} {
		if _, err := os.Stat(path.Join(dir, "no-compact-mark.json")); err == nil {
			return true
		}
		return false
	})

	assert.NoError(t, prom_testutil.GatherAndCompare(registry, strings.NewReader(`
			# HELP cortex_compactor_blocks_marked_for_no_compaction_total Total number of blocks marked for no compact during a compaction run.
			# TYPE cortex_compactor_blocks_marked_for_no_compaction_total counter
			cortex_compactor_blocks_marked_for_no_compaction_total 1
		`), "cortex_compactor_blocks_marked_for_no_compaction_total"))
}

func TestCompactor_ShouldCompactAllUsersOnShardingEnabledButOnlyOneInstanceRunning(t *testing.T) {
	t.Parallel()

	// Mock the bucket to contain two users, each one with one block.
	bucketClient := &bucket.ClientMock{}
	bucketClient.MockGet(users.UserIndexCompressedFilename, "", nil)
	bucketClient.MockIter("", []string{"user-1", "user-2"}, nil)
	bucketClient.MockIter("__markers__", []string{}, nil)
	bucketClient.MockExists(cortex_tsdb.GetGlobalDeletionMarkPath("user-1"), false, nil)
	bucketClient.MockExists(cortex_tsdb.GetLocalDeletionMarkPath("user-1"), false, nil)
	bucketClient.MockExists(cortex_tsdb.GetGlobalDeletionMarkPath("user-2"), false, nil)
	bucketClient.MockExists(cortex_tsdb.GetLocalDeletionMarkPath("user-2"), false, nil)
	bucketClient.MockIter("user-1/", []string{"user-1/01DTVP434PA9VFXSW2JKB3392D", "user-1/01DTVP434PA9VFXSW2JKB3392D/meta.json", "user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/meta.json"}, nil)
	bucketClient.MockIter("user-2/", []string{"user-2/01DTW0ZCPDDNV4BV83Q2SV4QAZ", "user-2/01DTW0ZCPDDNV4BV83Q2SV4QAZ/meta.json", "user-2/01FN3V83ABR9992RF8WRJZ76ZQ/meta.json"}, nil)
	bucketClient.MockIter("user-1/markers/", nil, nil)
	bucketClient.MockGet("user-1/markers/cleaner-visit-marker.json", "", nil)
	bucketClient.MockUpload("user-1/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockDelete("user-1/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockIter("user-2/markers/", nil, nil)
	bucketClient.MockGet("user-2/markers/cleaner-visit-marker.json", "", nil)
	bucketClient.MockUpload("user-2/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockDelete("user-2/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/meta.json", mockBlockMetaJSON("01DTVP434PA9VFXSW2JKB3392D"), nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/deletion-mark.json", "", nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/no-compact-mark.json", "", nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/visit-mark.json", "", nil)
	bucketClient.MockGet("user-1/bucket-index-sync-status.json", "", nil)
	bucketClient.MockUpload("user-1/01DTVP434PA9VFXSW2JKB3392D/visit-mark.json", nil)
	bucketClient.MockGet("user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/meta.json", mockBlockMetaJSON("01FN6CDF3PNEWWRY5MPGJPE3EX"), nil)
	bucketClient.MockGet("user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/deletion-mark.json", "", nil)
	bucketClient.MockGet("user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/no-compact-mark.json", "", nil)
	bucketClient.MockGet("user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/visit-mark.json", "", nil)
	bucketClient.MockUpload("user-1/01FN6CDF3PNEWWRY5MPGJPE3EX/visit-mark.json", nil)
	bucketClient.MockGet("user-2/01DTW0ZCPDDNV4BV83Q2SV4QAZ/meta.json", mockBlockMetaJSON("01DTW0ZCPDDNV4BV83Q2SV4QAZ"), nil)
	bucketClient.MockGet("user-2/01DTW0ZCPDDNV4BV83Q2SV4QAZ/deletion-mark.json", "", nil)
	bucketClient.MockGet("user-2/01DTW0ZCPDDNV4BV83Q2SV4QAZ/no-compact-mark.json", "", nil)
	bucketClient.MockGet("user-2/01DTW0ZCPDDNV4BV83Q2SV4QAZ/visit-mark.json", "", nil)
	bucketClient.MockUpload("user-2/01DTW0ZCPDDNV4BV83Q2SV4QAZ/visit-mark.json", nil)
	bucketClient.MockGet("user-2/01FN3V83ABR9992RF8WRJZ76ZQ/meta.json", mockBlockMetaJSON("01FN3V83ABR9992RF8WRJZ76ZQ"), nil)
	bucketClient.MockGet("user-2/01FN3V83ABR9992RF8WRJZ76ZQ/deletion-mark.json", "", nil)
	bucketClient.MockGet("user-2/01FN3V83ABR9992RF8WRJZ76ZQ/no-compact-mark.json", "", nil)
	bucketClient.MockGet("user-2/01FN3V83ABR9992RF8WRJZ76ZQ/visit-mark.json", "", nil)
	bucketClient.MockUpload("user-2/01FN3V83ABR9992RF8WRJZ76ZQ/visit-mark.json", nil)
	bucketClient.MockGet("user-1/bucket-index.json.gz", "", nil)
	bucketClient.MockGet("user-2/bucket-index.json.gz", "", nil)
	bucketClient.MockGet("user-1/bucket-index-sync-status.json", "", nil)
	bucketClient.MockGet("user-2/bucket-index-sync-status.json", "", nil)
	bucketClient.MockUpload("user-1/bucket-index.json.gz", nil)
	bucketClient.MockUpload("user-2/bucket-index.json.gz", nil)
	bucketClient.MockUpload("user-1/bucket-index-sync-status.json", nil)
	bucketClient.MockUpload("user-2/bucket-index-sync-status.json", nil)

	ringStore, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	t.Cleanup(func() { assert.NoError(t, closer.Close()) })

	cfg := prepareConfig()
	cfg.ShardingEnabled = true
	cfg.ShardingRing.InstanceID = "compactor-1"
	cfg.ShardingRing.InstanceAddr = "1.2.3.4"
	cfg.ShardingRing.KVStore.Mock = ringStore

	c, _, tsdbPlanner, logs, _ := prepare(t, cfg, bucketClient, nil)

	// Mock the planner as if there's no compaction to do,
	// in order to simplify tests (all in all, we just want to
	// test our logic and not TSDB compactor which we expect to
	// be already tested).
	tsdbPlanner.On("Plan", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*metadata.Meta{}, nil)

	require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))

	// Wait until a run has completed.
	cortex_testutil.Poll(t, 5*time.Second, 1.0, func() interface{} {
		return prom_testutil.ToFloat64(c.CompactionRunsCompleted)
	})

	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), c))

	// Ensure a plan has been executed for the blocks of each user.
	tsdbPlanner.AssertNumberOfCalls(t, "Plan", 2)

	assert.ElementsMatch(t, []string{
		`level=info component=compactor msg="auto joined with new tokens" ring=compactor state=ACTIVE`,
		`level=info component=compactor msg="waiting until compactor is ACTIVE in the ring"`,
		`level=info component=compactor msg="compactor is ACTIVE in the ring"`,
		`level=info component=compactor msg="compactor started"`,
		`level=info component=compactor msg="discovering users from bucket"`,
		`level=info component=compactor msg="discovered users from bucket" users=2`,
		`level=info component=compactor msg="starting compaction of user blocks" user=user-1`,
		`level=info component=compactor org_id=user-1 msg="start sync of metas"`,
		`level=info component=compactor org_id=user-1 msg="start of GC"`,
		`level=info component=compactor org_id=user-1 msg="start of compactions"`,
		`level=info component=compactor org_id=user-1 msg="compaction iterations done"`,
		`level=info component=compactor msg="successfully compacted user blocks" user=user-1`,
		`level=info component=compactor msg="starting compaction of user blocks" user=user-2`,
		`level=info component=compactor org_id=user-2 msg="start sync of metas"`,
		`level=info component=compactor org_id=user-2 msg="start of GC"`,
		`level=info component=compactor org_id=user-2 msg="start of compactions"`,
		`level=info component=compactor org_id=user-2 msg="compaction iterations done"`,
		`level=info component=compactor msg="successfully compacted user blocks" user=user-2`,
	}, removeIgnoredLogs(strings.Split(strings.TrimSpace(logs.String()), "\n")))
}

func TestCompactor_ShouldCompactOnlyUsersOwnedByTheInstanceOnShardingEnabledAndMultipleInstancesRunning(t *testing.T) {
	t.Parallel()

	numUsers := 100

	// Setup user IDs
	userIDs := make([]string, 0, numUsers)
	for i := 1; i <= numUsers; i++ {
		userIDs = append(userIDs, fmt.Sprintf("user-%d", i))
	}

	// Mock the bucket to contain all users, each one with one block.
	bucketClient := &bucket.ClientMock{}
	bucketClient.MockGet(users.UserIndexCompressedFilename, "", nil)
	bucketClient.MockIter("", userIDs, nil)
	bucketClient.MockIter("__markers__", []string{}, nil)
	for _, userID := range userIDs {
		bucketClient.MockIter(userID+"/", []string{userID + "/01DTVP434PA9VFXSW2JKB3392D"}, nil)
		bucketClient.MockIter(userID+"/markers/", nil, nil)
		bucketClient.MockGet(userID+"/markers/cleaner-visit-marker.json", "", nil)
		bucketClient.MockUpload(userID+"/markers/cleaner-visit-marker.json", nil)
		bucketClient.MockDelete(userID+"/markers/cleaner-visit-marker.json", nil)
		bucketClient.MockExists(cortex_tsdb.GetGlobalDeletionMarkPath(userID), false, nil)
		bucketClient.MockExists(cortex_tsdb.GetLocalDeletionMarkPath(userID), false, nil)
		bucketClient.MockGet(userID+"/01DTVP434PA9VFXSW2JKB3392D/meta.json", mockBlockMetaJSON("01DTVP434PA9VFXSW2JKB3392D"), nil)
		bucketClient.MockGet(userID+"/01DTVP434PA9VFXSW2JKB3392D/deletion-mark.json", "", nil)
		bucketClient.MockGet(userID+"/01DTVP434PA9VFXSW2JKB3392D/no-compact-mark.json", "", nil)
		bucketClient.MockGet(userID+"/01DTVP434PA9VFXSW2JKB3392D/visit-mark.json", "", nil)
		bucketClient.MockGet(userID+"/bucket-index-sync-status.json", "", nil)
		bucketClient.MockUpload(userID+"/01DTVP434PA9VFXSW2JKB3392D/visit-mark.json", nil)
		bucketClient.MockGet(userID+"/bucket-index.json.gz", "", nil)
		bucketClient.MockUpload(userID+"/bucket-index.json.gz", nil)
		bucketClient.MockUpload(userID+"/bucket-index-sync-status.json", nil)
	}

	// Create a shared KV Store
	kvstore, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	t.Cleanup(func() { assert.NoError(t, closer.Close()) })

	// Create two compactors
	var compactors []*Compactor
	var logs []*concurrency.SyncBuffer

	for i := 1; i <= 2; i++ {
		cfg := prepareConfig()
		cfg.ShardingEnabled = true
		cfg.ShardingRing.InstanceID = fmt.Sprintf("compactor-%d", i)
		cfg.ShardingRing.InstanceAddr = fmt.Sprintf("127.0.0.%d", i)
		cfg.ShardingRing.WaitStabilityMinDuration = time.Second
		cfg.ShardingRing.WaitStabilityMaxDuration = 5 * time.Second
		cfg.ShardingRing.KVStore.Mock = kvstore

		c, _, tsdbPlanner, l, _ := prepare(t, cfg, bucketClient, nil)
		defer services.StopAndAwaitTerminated(context.Background(), c) //nolint:errcheck

		compactors = append(compactors, c)
		logs = append(logs, l)

		// Mock the planner as if there's no compaction to do,
		// in order to simplify tests (all in all, we just want to
		// test our logic and not TSDB compactor which we expect to
		// be already tested).
		tsdbPlanner.On("Plan", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*metadata.Meta{}, nil)
	}

	// Start all compactors
	for _, c := range compactors {
		require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))
	}

	// Wait until a run has been completed on each compactor
	for _, c := range compactors {
		cortex_testutil.Poll(t, 120*time.Second, true, func() interface{} {
			return prom_testutil.ToFloat64(c.CompactionRunsCompleted) >= 1
		})
	}

	// Ensure that each user has been compacted by the correct instance
	for _, userID := range userIDs {
		_, l, err := findCompactorByUserID(compactors, logs, userID)
		require.NoError(t, err)
		assert.Contains(t, l.String(), fmt.Sprintf(`level=info component=compactor msg="successfully compacted user blocks" user=%s`, userID))
	}
}

func TestCompactor_ShouldCompactOnlyShardsOwnedByTheInstanceOnShardingEnabledWithShuffleShardingAndMultipleInstancesRunning(t *testing.T) {
	t.Parallel()

	numUsers := 3

	// Setup user IDs
	userIDs := make([]string, 0, numUsers)
	for i := 1; i <= numUsers; i++ {
		userIDs = append(userIDs, fmt.Sprintf("user-%d", i))
	}

	startTime := int64(1574776800000)
	// Define blocks mapping block IDs to start and end times
	blocks := map[string]map[string]int64{
		"01DTVP434PA9VFXSW2JKB3392D": {
			"startTime": startTime,
			"endTime":   startTime + time.Hour.Milliseconds()*2,
		},
		"01DTVP434PA9VFXSW2JKB3392E": {
			"startTime": startTime,
			"endTime":   startTime + time.Hour.Milliseconds()*2,
		},
		"01DTVP434PA9VFXSW2JKB3392F": {
			"startTime": startTime + time.Hour.Milliseconds()*2,
			"endTime":   startTime + time.Hour.Milliseconds()*4,
		},
		"01DTVP434PA9VFXSW2JKB3392G": {
			"startTime": startTime + time.Hour.Milliseconds()*2,
			"endTime":   startTime + time.Hour.Milliseconds()*4,
		},
		// Add another new block as the final block so that the previous groups will be planned for compaction
		"01DTVP434PA9VFXSW2JKB3392H": {
			"startTime": startTime + time.Hour.Milliseconds()*4,
			"endTime":   startTime + time.Hour.Milliseconds()*6,
		},
	}

	// Mock the bucket to contain all users, each one with five blocks, 2 sets of overlapping blocks and 1 separate block.
	bucketClient := &bucket.ClientMock{}
	bucketClient.MockGet(users.UserIndexCompressedFilename, "", nil)
	bucketClient.MockIter("", userIDs, nil)
	bucketClient.MockIter("__markers__", []string{}, nil)

	// Keys with a value greater than 1 will be groups that should be compacted
	groupHashes := make(map[uint32]int)
	for _, userID := range userIDs {
		blockFiles := []string{}

		for blockID, blockTimes := range blocks {
			blockVisitMarker := BlockVisitMarker{
				CompactorID: "test-compactor",
				VisitTime:   time.Now().Unix(),
				Version:     VisitMarkerVersion1,
			}
			visitMarkerFileContent, _ := json.Marshal(blockVisitMarker)
			bucketClient.MockGet(userID+"/bucket-index-sync-status.json", "", nil)
			bucketClient.MockGet(userID+"/"+blockID+"/meta.json", mockBlockMetaJSONWithTime(blockID, userID, blockTimes["startTime"], blockTimes["endTime"]), nil)
			bucketClient.MockGet(userID+"/"+blockID+"/deletion-mark.json", "", nil)
			bucketClient.MockGet(userID+"/"+blockID+"/no-compact-mark.json", "", nil)
			bucketClient.MockGet(userID+"/"+blockID+"/visit-mark.json", string(visitMarkerFileContent), nil)
			bucketClient.MockGetRequireUpload(userID+"/"+blockID+"/visit-mark.json", string(visitMarkerFileContent), nil)
			bucketClient.MockUpload(userID+"/"+blockID+"/visit-mark.json", nil)
			// Iter with recursive so expected to get objects rather than directories.
			blockFiles = append(blockFiles, path.Join(userID, blockID), path.Join(userID, blockID, block.MetaFilename))

			// Get all of the unique group hashes so that they can be used to ensure all groups were compacted
			groupHash := hashGroup(userID, blockTimes["startTime"], blockTimes["endTime"])
			groupHashes[groupHash]++
		}

		bucketClient.MockIter(userID+"/", blockFiles, nil)
		bucketClient.MockIter(userID+"/markers/", nil, nil)
		bucketClient.MockGet(userID+"/markers/cleaner-visit-marker.json", "", nil)
		bucketClient.MockUpload(userID+"/markers/cleaner-visit-marker.json", nil)
		bucketClient.MockDelete(userID+"/markers/cleaner-visit-marker.json", nil)
		bucketClient.MockExists(cortex_tsdb.GetGlobalDeletionMarkPath(userID), false, nil)
		bucketClient.MockExists(cortex_tsdb.GetLocalDeletionMarkPath(userID), false, nil)
		bucketClient.MockGet(userID+"/bucket-index.json.gz", "", nil)
		bucketClient.MockUpload(userID+"/bucket-index.json.gz", nil)
		bucketClient.MockUpload(userID+"/bucket-index-sync-status.json", nil)
	}

	// Create a shared KV Store
	kvstore, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	t.Cleanup(func() { assert.NoError(t, closer.Close()) })

	// Create four compactors
	var compactors []*Compactor
	var logs []*concurrency.SyncBuffer

	for i := 1; i <= 4; i++ {
		cfg := prepareConfig()
		cfg.ShardingEnabled = true
		cfg.CompactionInterval = 15 * time.Second
		cfg.ShardingStrategy = util.ShardingStrategyShuffle
		cfg.ShardingRing.InstanceID = fmt.Sprintf("compactor-%d", i)
		cfg.ShardingRing.InstanceAddr = fmt.Sprintf("127.0.0.%d", i)
		cfg.ShardingRing.WaitStabilityMinDuration = time.Second
		cfg.ShardingRing.WaitStabilityMaxDuration = 5 * time.Second
		cfg.ShardingRing.KVStore.Mock = kvstore

		limits := &validation.Limits{}
		flagext.DefaultValues(limits)
		limits.CompactorTenantShardSize = 3

		c, _, tsdbPlanner, l, _ := prepare(t, cfg, bucketClient, limits)
		defer services.StopAndAwaitTerminated(context.Background(), c) //nolint:errcheck

		compactors = append(compactors, c)
		logs = append(logs, l)

		// Mock the planner as if there's no compaction to do,
		// in order to simplify tests (all in all, we just want to
		// test our logic and not TSDB compactor which we expect to
		// be already tested).
		tsdbPlanner.On("Plan", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*metadata.Meta{}, nil)
	}

	// Start all compactors
	for _, c := range compactors {
		require.NoError(t, c.StartAsync(context.Background()))
	}
	// Wait for all the compactors to get into the Running state without errors.
	// Cannot use StartAndAwaitRunning as this would cause the compactions to start before
	// all the compactors are initialized
	for _, c := range compactors {
		require.NoError(t, c.AwaitRunning(context.Background()))
	}

	// Wait until a run has been completed on each compactor
	for _, c := range compactors {
		cortex_testutil.Poll(t, 60*time.Second, 2.0, func() interface{} {
			return prom_testutil.ToFloat64(c.CompactionRunsCompleted)
		})
	}

	// Ensure that each group was only compacted by exactly one compactor
	for groupHash, blockCount := range groupHashes {

		l, found, err := checkLogsForCompaction(compactors, logs, groupHash)
		require.NoError(t, err)

		// If the blockCount < 2 then the group shouldn't have been compacted, therefore not found in the logs
		if blockCount < 2 {
			assert.False(t, found)
		} else {
			assert.True(t, found)
			assert.Contains(t, l.String(), fmt.Sprintf(`msg="found compactable group for user" group_hash=%d`, groupHash))
		}
	}
}

// checkLogsForCompaction checks the logs to see if a compaction has happened on the groupHash,
// if there has been a compaction it will return the logs of the compactor that handled the group
// and will return true. Otherwise this function will return a nil value for the logs and false
// as the group was not compacted
func checkLogsForCompaction(compactors []*Compactor, logs []*concurrency.SyncBuffer, groupHash uint32) (*concurrency.SyncBuffer, bool, error) {
	var log *concurrency.SyncBuffer

	// Make sure that the group_hash is only owned by a single compactor
	for _, l := range logs {
		owned := strings.Contains(l.String(), fmt.Sprintf(`msg="found compactable group for user" group_hash=%d`, groupHash))

		// Ensure the group is not owned by multiple compactors
		if owned && log != nil {
			return nil, false, fmt.Errorf("group with group_hash=%d owned by multiple compactors", groupHash)
		}
		if owned {
			log = l
		}
	}

	// Return false if we've not been able to find it
	if log == nil {
		return nil, false, nil
	}

	return log, true, nil
}

func createTSDBBlock(t *testing.T, bkt objstore.Bucket, userID string, minT, maxT int64, externalLabels map[string]string) ulid.ULID {
	// Create a temporary dir for TSDB.
	tempDir := t.TempDir()

	// Create a temporary dir for the snapshot.
	snapshotDir := t.TempDir()

	// Create a new TSDB.
	db, err := tsdb.Open(tempDir, nil, nil, &tsdb.Options{
		MinBlockDuration:  int64(2 * 60 * 60 * 1000), // 2h period
		MaxBlockDuration:  int64(2 * 60 * 60 * 1000), // 2h period
		RetentionDuration: int64(15 * 86400 * 1000),  // 15 days
	}, nil)
	require.NoError(t, err)

	db.DisableCompactions()

	// Append a sample at the beginning and one at the end of the time range.
	for i, ts := range []int64{minT, maxT - 1} {
		lbls := labels.FromStrings("series_id", strconv.Itoa(i))

		app := db.Appender(context.Background())
		_, err := app.Append(0, lbls, ts, float64(i))
		require.NoError(t, err)

		err = app.Commit()
		require.NoError(t, err)
	}

	require.NoError(t, db.Compact(context.Background()))
	require.NoError(t, db.Snapshot(snapshotDir, true))

	// Look for the created block (we expect one).
	entries, err := os.ReadDir(snapshotDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.True(t, entries[0].IsDir())

	blockID, err := ulid.Parse(entries[0].Name())
	require.NoError(t, err)

	// Inject Thanos external labels to the block.
	meta := metadata.Thanos{
		Labels: externalLabels,
		Source: "test",
	}
	_, err = metadata.InjectThanos(log.NewNopLogger(), filepath.Join(snapshotDir, blockID.String()), meta, nil)
	require.NoError(t, err)

	// Copy the block files to the bucket.
	srcRoot := filepath.Join(snapshotDir, blockID.String())
	require.NoError(t, filepath.Walk(srcRoot, func(file string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Read the file content in memory.
		content, err := os.ReadFile(file)
		if err != nil {
			return err
		}

		// Upload it to the bucket.
		relPath, err := filepath.Rel(srcRoot, file)
		if err != nil {
			return err
		}

		return bkt.Upload(context.Background(), path.Join(userID, blockID.String(), relPath), bytes.NewReader(content))
	}))

	return blockID
}

func createDeletionMark(t *testing.T, bkt objstore.Bucket, userID string, blockID ulid.ULID, deletionTime time.Time) {
	content := mockDeletionMarkJSON(blockID.String(), deletionTime)
	blockPath := path.Join(userID, blockID.String())
	markPath := path.Join(blockPath, metadata.DeletionMarkFilename)

	require.NoError(t, bkt.Upload(context.Background(), markPath, strings.NewReader(content)))
}

func createNoCompactionMark(t *testing.T, bkt objstore.Bucket, userID string, blockID ulid.ULID) {
	content := mockNoCompactBlockJSON(blockID.String())
	blockPath := path.Join(userID, blockID.String())
	markPath := path.Join(blockPath, metadata.NoCompactMarkFilename)

	require.NoError(t, bkt.Upload(context.Background(), markPath, strings.NewReader(content)))
}

func createBlockVisitMarker(t *testing.T, bkt objstore.Bucket, userID string, blockID ulid.ULID) {
	content := mockBlockVisitMarker()
	blockPath := path.Join(userID, blockID.String())
	markPath := path.Join(blockPath, BlockVisitMarkerFile)

	require.NoError(t, bkt.Upload(context.Background(), markPath, strings.NewReader(content)))
}

func createParquetMarker(t *testing.T, bkt objstore.Bucket, userID string, blockID ulid.ULID) {
	content := mockParquetMarker()
	blockPath := path.Join(userID, blockID.String())
	markPath := path.Join(blockPath, parquet.ConverterMarkerFileName)

	require.NoError(t, bkt.Upload(context.Background(), markPath, strings.NewReader(content)))
}

func findCompactorByUserID(compactors []*Compactor, logs []*concurrency.SyncBuffer, userID string) (*Compactor, *concurrency.SyncBuffer, error) {
	var compactor *Compactor
	var log *concurrency.SyncBuffer

	for i, c := range compactors {
		owned, err := c.ownUserForCompaction(userID)
		if err != nil {
			return nil, nil, err
		}

		// Ensure the user is not owned by multiple compactors
		if owned && compactor != nil {
			return nil, nil, fmt.Errorf("user %s owned by multiple compactors", userID)
		}
		if owned {
			compactor = c
			log = logs[i]
		}
	}

	// Return an error if we've not been able to find it
	if compactor == nil {
		return nil, nil, fmt.Errorf("user %s not owned by any compactor", userID)
	}

	return compactor, log, nil
}

func removeIgnoredLogs(input []string) []string {
	ignoredLogStringsMap := map[string]struct{}{
		// Since we moved to the component logger from the global logger for the ring these lines are now expected, but are just ring setup information.
		`level=info component=compactor msg="ring doesn't exist in KV store yet"`:                                                                                 {},
		`level=info component=compactor msg="not loading tokens from file, tokens file path is empty"`:                                                            {},
		`level=info component=compactor msg="instance not found in ring, adding with no tokens" ring=compactor`:                                                   {},
		`level=debug component=compactor msg="JoinAfter expired" ring=compactor`:                                                                                  {},
		`level=info component=compactor msg="auto-joining cluster after timeout" ring=compactor`:                                                                  {},
		`level=info component=compactor msg="lifecycler loop() exited gracefully" ring=compactor`:                                                                 {},
		`level=info component=compactor msg="changing instance state from" old_state=ACTIVE new_state=LEAVING ring=compactor`:                                     {},
		`level=error component=compactor msg="failed to set state to LEAVING" ring=compactor err="changing instance state from LEAVING -> LEAVING is disallowed"`: {},
		`level=error component=compactor msg="failed to set state to LEAVING" ring=compactor err="changing instance state from JOINING -> LEAVING is disallowed"`: {},
		`level=info component=compactor msg="user index not found, fallback to base scanner"`:                                                                     {},
		`level=error component=compactor msg="context timeout, exit user index update loop" err="context canceled"`:                                               {},
		`level=debug component=compactor msg="unregistering instance from ring" ring=compactor`:                                                                   {},
		`level=info component=compactor msg="instance removed from the KV store" ring=compactor`:                                                                  {},
		`level=info component=compactor msg="observing tokens before going ACTIVE" ring=compactor`:                                                                {},
		`level=info component=compactor msg="lifecycler entering final sleep before shutdown" final_sleep=0s`:                                                     {},
		`level=info component=compactor msg="compactor stopped"`:                                                                                                  {},
	}

	ignoredLogStringsRegexList := []*regexp.Regexp{
		regexp.MustCompile(`^level=(info|debug|warn) component=cleaner .+$`),
		regexp.MustCompile(`^level=info component=compactor msg="set state" .+$`),
	}

	out := make([]string, 0, len(input))
	durationRe := regexp.MustCompile(`\s?duration(_ms)?=\S+`)
	executionIDRe := regexp.MustCompile(`\s?execution_id=\S+`)

main:
	for i := 0; i < len(input); i++ {
		log := input[i]

		// Remove any duration from logs.
		log = durationRe.ReplaceAllString(log, "")

		// Remove any execution_id from logs.
		log = executionIDRe.ReplaceAllString(log, "")

		if strings.Contains(log, "block.MetaFetcher") || strings.Contains(log, "block.BaseFetcher") {
			continue
		}

		if _, exists := ignoredLogStringsMap[log]; exists {
			continue
		}

		for _, ignoreRegex := range ignoredLogStringsRegexList {
			if ignoreRegex.MatchString(log) {
				continue main
			}
		}

		out = append(out, log)
	}

	return out
}

func prepareConfig() Config {
	compactorCfg := Config{}
	flagext.DefaultValues(&compactorCfg)

	compactorCfg.retryMinBackoff = 0
	compactorCfg.retryMaxBackoff = 0

	//Avoid jitter in startup
	compactorCfg.CompactionInterval = 5 * time.Second

	// The migration is tested in a dedicated test.
	compactorCfg.BlockDeletionMarksMigrationEnabled = false

	// Do not wait for ring stability by default, in order to speed up tests.
	compactorCfg.ShardingRing.WaitStabilityMinDuration = 0
	compactorCfg.ShardingRing.WaitStabilityMaxDuration = 0

	// Set lower timeout for waiting on compactor to become ACTIVE in the ring for unit tests
	compactorCfg.ShardingRing.WaitActiveInstanceTimeout = 5 * time.Second

	return compactorCfg
}

func prepare(t *testing.T, compactorCfg Config, bucketClient objstore.InstrumentedBucket, limits *validation.Limits) (*Compactor, *tsdbCompactorMock, *tsdbPlannerMock, *concurrency.SyncBuffer, prometheus.Gatherer) {
	storageCfg := cortex_tsdb.BlocksStorageConfig{}
	flagext.DefaultValues(&storageCfg)
	storageCfg.BucketStore.BlockDiscoveryStrategy = string(cortex_tsdb.RecursiveDiscovery)
	storageCfg.UsersScanner.Strategy = cortex_tsdb.UserScanStrategyUserIndex

	// Create a temporary directory for compactor data.
	compactorCfg.DataDir = t.TempDir()

	tsdbCompactor := &tsdbCompactorMock{}
	tsdbPlanner := &tsdbPlannerMock{
		noCompactMarkFilters: []*compact.GatherNoCompactionMarkFilter{},
	}
	logs := &concurrency.SyncBuffer{}
	logger := log.NewLogfmtLogger(logs)
	registry := prometheus.NewRegistry()

	if limits == nil {
		limits = &validation.Limits{}
		flagext.DefaultValues(limits)
	}

	overrides := validation.NewOverrides(*limits, nil)

	bucketClientFactory := func(ctx context.Context) (objstore.InstrumentedBucket, error) {
		return bucketClient, nil
	}

	blocksCompactorFactory := func(ctx context.Context, cfg Config, logger log.Logger, reg prometheus.Registerer) (compact.Compactor, PlannerFactory, error) {
		return tsdbCompactor,
			func(ctx context.Context, bkt objstore.InstrumentedBucket, _ log.Logger, _ Config, noCompactMarkFilter *compact.GatherNoCompactionMarkFilter, ringLifecycle *ring.Lifecycler, _ string, _ prometheus.Counter, _ prometheus.Counter, _ *compactorMetrics) compact.Planner {
				tsdbPlanner.noCompactMarkFilters = append(tsdbPlanner.noCompactMarkFilters, noCompactMarkFilter)
				return tsdbPlanner
			},
			nil
	}

	var blocksGrouperFactory BlocksGrouperFactory
	if compactorCfg.ShardingStrategy == util.ShardingStrategyShuffle {
		blocksGrouperFactory = ShuffleShardingGrouperFactory
	} else {
		blocksGrouperFactory = DefaultBlocksGrouperFactory
	}

	c, err := newCompactor(compactorCfg, storageCfg, logger, registry, bucketClientFactory, blocksGrouperFactory, blocksCompactorFactory, DefaultBlockDeletableCheckerFactory, DefaultCompactionLifecycleCallbackFactory, overrides, 1)
	require.NoError(t, err)

	return c, tsdbCompactor, tsdbPlanner, logs, registry
}

type tsdbCompactorMock struct {
	mock.Mock
}

func (m *tsdbCompactorMock) Plan(dir string) ([]string, error) {
	args := m.Called(dir)
	return args.Get(0).([]string), args.Error(1)
}

func (m *tsdbCompactorMock) Compact(dest string, dirs []string, open []*tsdb.Block) ([]ulid.ULID, error) {
	args := m.Called(dest, dirs, open)
	return args.Get(0).([]ulid.ULID), args.Error(1)
}

func (m *tsdbCompactorMock) CompactWithBlockPopulator(dest string, dirs []string, open []*tsdb.Block, blockPopulator tsdb.BlockPopulator) ([]ulid.ULID, error) {
	args := m.Called(dest, dirs, open, blockPopulator)
	return args.Get(0).([]ulid.ULID), args.Error(1)
}

type tsdbPlannerMock struct {
	mock.Mock
	noCompactMarkFilters []*compact.GatherNoCompactionMarkFilter
}

func (m *tsdbPlannerMock) Plan(ctx context.Context, metasByMinTime []*metadata.Meta, errChan chan error, extensions any) ([]*metadata.Meta, error) {
	args := m.Called(ctx, metasByMinTime, errChan, extensions)
	return args.Get(0).([]*metadata.Meta), args.Error(1)
}

func (m *tsdbPlannerMock) getNoCompactBlocks() []string {

	result := []string{}

	for _, noCompactMarkFilter := range m.noCompactMarkFilters {
		for _, mark := range noCompactMarkFilter.NoCompactMarkedBlocks() {
			result = append(result, mark.ID.String())
		}
	}

	return result
}

func mockBlockMeta(id string) tsdb.BlockMeta {
	return tsdb.BlockMeta{
		Version: 1,
		ULID:    ulid.MustParse(id),
		MinTime: 1574776800000,
		MaxTime: 1574784000000,
		Compaction: tsdb.BlockMetaCompaction{
			Level:   1,
			Sources: []ulid.ULID{ulid.MustParse(id)},
		},
	}
}

func mockBlockMetaJSON(id string) string {
	meta := mockBlockMeta(id)

	content, err := json.Marshal(meta)
	if err != nil {
		panic("failed to marshal mocked block meta")
	}

	return string(content)
}

func mockNoCompactBlockJSON(id string) string {
	meta := metadata.NoCompactMark{
		Version:       1,
		ID:            ulid.MustParse(id),
		NoCompactTime: time.Now().Unix(),
		Details:       "yolo",
		Reason:        "testing",
	}

	content, err := json.Marshal(meta)
	if err != nil {
		panic("failed to marshal mocked block no compact mark")
	}

	return string(content)
}

func mockDeletionMarkJSON(id string, deletionTime time.Time) string {
	meta := metadata.DeletionMark{
		Version:      metadata.DeletionMarkVersion1,
		ID:           ulid.MustParse(id),
		DeletionTime: deletionTime.Unix(),
	}

	content, err := json.Marshal(meta)
	if err != nil {
		panic("failed to marshal mocked block deletion mark")
	}

	return string(content)
}

func mockBlockMetaJSONWithTime(id string, orgID string, minTime int64, maxTime int64) string {
	meta := metadata.Meta{
		Thanos: metadata.Thanos{
			Labels: map[string]string{"__org_id__": orgID},
		},
	}

	meta.BlockMeta = tsdb.BlockMeta{
		Version: 1,
		ULID:    ulid.MustParse(id),
		MinTime: minTime,
		MaxTime: maxTime,
		Compaction: tsdb.BlockMetaCompaction{
			Level:   1,
			Sources: []ulid.ULID{ulid.MustParse(id)},
		},
	}

	content, err := json.Marshal(meta)
	if err != nil {
		panic("failed to marshal mocked block meta")
	}

	return string(content)
}

func mockBlockVisitMarker() string {
	blockVisitMarker := BlockVisitMarker{
		CompactorID: "dummy",
		VisitTime:   time.Now().Unix(),
		Version:     1,
	}

	content, err := json.Marshal(blockVisitMarker)
	if err != nil {
		panic("failed to marshal mocked block visit marker")
	}

	return string(content)
}

func mockParquetMarker() string {
	parquetMarker := parquet.ConverterMark{
		Version: 1,
	}

	content, err := json.Marshal(parquetMarker)
	if err != nil {
		panic("failed to marshal mocked block visit marker")
	}

	return string(content)
}

func TestCompactor_DeleteLocalSyncFiles(t *testing.T) {
	numUsers := 10

	// Setup user IDs
	userIDs := make([]string, 0, numUsers)
	for i := 1; i <= numUsers; i++ {
		userIDs = append(userIDs, fmt.Sprintf("user-%d", i))
	}

	inmem := objstore.WithNoopInstr(objstore.NewInMemBucket())
	for _, userID := range userIDs {
		id, err := ulid.New(ulid.Now(), rand.Reader)
		require.NoError(t, err)
		require.NoError(t, inmem.Upload(context.Background(), userID+"/"+id.String()+"/meta.json", strings.NewReader(mockBlockMetaJSON(id.String()))))
	}

	// Create a shared KV Store
	kvstore, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	t.Cleanup(func() { assert.NoError(t, closer.Close()) })

	// Create two compactors
	var compactors []*Compactor

	for i := 1; i <= 2; i++ {
		cfg := prepareConfig()

		cfg.ShardingEnabled = true
		cfg.ShardingRing.InstanceID = fmt.Sprintf("compactor-%d", i)
		cfg.ShardingRing.InstanceAddr = fmt.Sprintf("127.0.0.%d", i)
		cfg.ShardingRing.WaitStabilityMinDuration = time.Second
		cfg.ShardingRing.WaitStabilityMaxDuration = 5 * time.Second
		cfg.ShardingRing.KVStore.Mock = kvstore

		// Each compactor will get its own temp dir for storing local files.
		c, _, tsdbPlanner, _, _ := prepare(t, cfg, inmem, nil)
		t.Cleanup(func() {
			require.NoError(t, services.StopAndAwaitTerminated(context.Background(), c))
		})

		compactors = append(compactors, c)

		// Mock the planner as if there's no compaction to do,
		// in order to simplify tests (all in all, we just want to
		// test our logic and not TSDB compactor which we expect to
		// be already tested).
		tsdbPlanner.On("Plan", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*metadata.Meta{}, nil)
	}

	require.Equal(t, 2, len(compactors))
	c1 := compactors[0]
	c2 := compactors[1]

	// Start first compactor
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), c1))

	// Wait until a run has been completed on first compactor. This happens as soon as compactor starts.
	cortex_testutil.Poll(t, 10*time.Second, 1.0, func() interface{} {
		return prom_testutil.ToFloat64(c1.CompactionRunsCompleted)
	})

	require.NoError(t, os.Mkdir(c1.metaSyncDirForUser("new-user"), 0600))

	// Verify that first compactor has synced all the users, plus there is one extra we have just created.
	require.Equal(t, numUsers+1, len(c1.listTenantsWithMetaSyncDirectories()))

	// Now start second compactor, and wait until it runs compaction.
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), c2))
	cortex_testutil.Poll(t, 10*time.Second, 1.0, func() interface{} {
		return prom_testutil.ToFloat64(c2.CompactionRunsCompleted)
	})

	// Let's check how many users second compactor has.
	c2Users := len(c2.listTenantsWithMetaSyncDirectories())
	require.NotZero(t, c2Users)

	// Force new compaction cycle on first compactor. It will run the cleanup of un-owned users at the end of compaction cycle.
	c1.compactUsers(context.Background())
	c1Users := len(c1.listTenantsWithMetaSyncDirectories())

	// Now compactor 1 should have cleaned old sync files.
	require.NotEqual(t, numUsers, c1Users)
	require.Equal(t, numUsers, c1Users+c2Users)
}

func TestCompactor_ShouldFailCompactionOnTimeout(t *testing.T) {
	t.Parallel()

	// Mock the bucket
	bucketClient := &bucket.ClientMock{}
	bucketClient.MockGet(users.UserIndexCompressedFilename, "", nil)
	bucketClient.MockIter("", []string{}, nil)
	bucketClient.MockIter("__markers__", []string{}, nil)

	ringStore, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	t.Cleanup(func() { assert.NoError(t, closer.Close()) })

	cfg := prepareConfig()
	cfg.ShardingEnabled = true
	cfg.ShardingRing.InstanceID = "compactor-1"
	cfg.ShardingRing.InstanceAddr = "1.2.3.4"
	cfg.ShardingRing.KVStore.Mock = ringStore

	// Set ObservePeriod to longer than the timeout period to mock a timeout while waiting on ring to become ACTIVE
	cfg.ShardingRing.ObservePeriod = time.Second * 10

	c, _, _, logs, _ := prepare(t, cfg, bucketClient, nil)

	// Try to start the compactor with a bad consul kv-store. The
	err := services.StartAndAwaitRunning(context.Background(), c)

	// Assert that the compactor timed out
	assert.Equal(t, context.DeadlineExceeded, err)

	assert.ElementsMatch(t, []string{
		`level=info component=compactor msg="auto joined with new tokens" ring=compactor state=JOINING`,
		`level=info component=compactor msg="compactor started"`,
		`level=info component=compactor msg="waiting until compactor is ACTIVE in the ring"`,
		`level=error component=compactor msg="compactor failed to become ACTIVE in the ring" err="context deadline exceeded"`,
	}, removeIgnoredLogs(strings.Split(strings.TrimSpace(logs.String()), "\n")))
}

func TestCompactor_ShouldNotTreatInterruptionsAsErrors(t *testing.T) {
	bucketClient := objstore.WithNoopInstr(objstore.NewInMemBucket())
	id := ulid.MustNew(ulid.Now(), rand.Reader)
	require.NoError(t, bucketClient.Upload(context.Background(), "user-1/"+id.String()+"/meta.json", strings.NewReader(mockBlockMetaJSON(id.String()))))

	b1 := createTSDBBlock(t, bucketClient, "user-1", 10, 20, map[string]string{"__name__": "Teste"})
	b2 := createTSDBBlock(t, bucketClient, "user-1", 20, 30, map[string]string{"__name__": "Teste"})

	c, tsdbCompactor, tsdbPlanner, logs, registry := prepare(t, prepareConfig(), bucketClient, nil)

	ctx, cancel := context.WithCancel(context.Background())
	tsdbCompactor.On("CompactWithBlockPopulator", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]ulid.ULID{}, context.Canceled).Run(func(args mock.Arguments) {
		cancel()
	})
	tsdbPlanner.On("Plan", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*metadata.Meta{
		{
			BlockMeta: tsdb.BlockMeta{
				ULID:    b1,
				MinTime: 10,
				MaxTime: 20,
			},
		},
		{
			BlockMeta: tsdb.BlockMeta{
				ULID:    b2,
				MinTime: 20,
				MaxTime: 30,
			},
		},
	}, nil)
	require.NoError(t, services.StartAndAwaitRunning(ctx, c))

	cortex_testutil.Poll(t, 1*time.Second, 1.0, func() interface{} {
		return prom_testutil.ToFloat64(c.CompactionRunsInterrupted)
	})

	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), c))

	assert.NoError(t, prom_testutil.GatherAndCompare(registry, strings.NewReader(`
		# TYPE cortex_compactor_runs_completed_total counter
		# HELP cortex_compactor_runs_completed_total Total number of compaction runs successfully completed.
		cortex_compactor_runs_completed_total 0

		# TYPE cortex_compactor_runs_interrupted_total counter
		# HELP cortex_compactor_runs_interrupted_total Total number of compaction runs interrupted.
		cortex_compactor_runs_interrupted_total 1

		# TYPE cortex_compactor_runs_failed_total counter
		# HELP cortex_compactor_runs_failed_total Total number of compaction runs failed.
		cortex_compactor_runs_failed_total 0
	`),
		"cortex_compactor_runs_completed_total",
		"cortex_compactor_runs_interrupted_total",
		"cortex_compactor_runs_failed_total",
	))

	lines := strings.Split(logs.String(), "\n")
	require.Contains(t, lines, `level=info component=compactor msg="interrupting compaction of user blocks" user=user-1`)
	require.NotContains(t, logs.String(), `level=error`)
}

func TestCompactor_ShouldNotFailCompactionIfAccessDeniedErrDuringMetaSync(t *testing.T) {
	t.Parallel()

	ss := bucketindex.Status{Status: bucketindex.Ok, Version: bucketindex.SyncStatusFileVersion}
	content, err := json.Marshal(ss)
	require.NoError(t, err)

	bucketClient := &bucket.ClientMock{}
	bucketClient.MockGet(users.UserIndexCompressedFilename, "", nil)
	bucketClient.MockIter("__markers__", []string{}, nil)
	bucketClient.MockIter("", []string{"user-1"}, nil)
	bucketClient.MockIter("user-1/", []string{"user-1/01DTVP434PA9VFXSW2JKB3392D", "user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ", "user-1/01DTVP434PA9VFXSW2JKB3392D/meta.json", "user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/meta.json"}, nil)
	bucketClient.MockIter("user-1/markers/", nil, nil)
	bucketClient.MockGet("user-1/markers/cleaner-visit-marker.json", "", nil)
	bucketClient.MockUpload("user-1/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockDelete("user-1/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockExists(cortex_tsdb.GetGlobalDeletionMarkPath("user-1"), false, nil)
	bucketClient.MockExists(cortex_tsdb.GetLocalDeletionMarkPath("user-1"), false, nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/meta.json", mockBlockMetaJSON("01DTVP434PA9VFXSW2JKB3392D"), bucket.ErrKeyPermissionDenied)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/deletion-mark.json", "", bucket.ErrKeyPermissionDenied)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/no-compact-mark.json", "", bucket.ErrKeyPermissionDenied)
	bucketClient.MockGet("user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/meta.json", mockBlockMetaJSON("01DTW0ZCPDDNV4BV83Q2SV4QAZ"), bucket.ErrKeyPermissionDenied)
	bucketClient.MockGet("user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/deletion-mark.json", "", bucket.ErrKeyPermissionDenied)
	bucketClient.MockGet("user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/no-compact-mark.json", "", bucket.ErrKeyPermissionDenied)
	bucketClient.MockGet("user-1/bucket-index.json.gz", "", nil)
	bucketClient.MockGet("user-1/bucket-index-sync-status.json", string(content), nil)
	bucketClient.MockUpload("user-1/bucket-index.json.gz", nil)
	bucketClient.MockUpload("user-1/bucket-index-sync-status.json", nil)

	ringStore, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	t.Cleanup(func() { assert.NoError(t, closer.Close()) })

	cfg := prepareConfig()
	cfg.ShardingEnabled = true
	cfg.ShardingRing.InstanceID = "compactor-1"
	cfg.ShardingRing.InstanceAddr = "1.2.3.4"
	cfg.ShardingRing.KVStore.Mock = ringStore

	c, _, tsdbPlanner, _, _ := prepare(t, cfg, bucketClient, nil)
	tsdbPlanner.On("Plan", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*metadata.Meta{}, nil)

	require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))

	// Wait until a run has completed.
	cortex_testutil.Poll(t, 20*time.Second, 1.0, func() interface{} {
		return prom_testutil.ToFloat64(c.CompactionRunsCompleted)
	})

	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), c))
}

func TestCompactor_ShouldNotFailCompactionIfAccessDeniedErrReturnedFromBucket(t *testing.T) {
	t.Parallel()

	ss := bucketindex.Status{Status: bucketindex.Ok, Version: bucketindex.SyncStatusFileVersion}
	content, err := json.Marshal(ss)
	require.NoError(t, err)

	bucketClient := &bucket.ClientMock{}
	bucketClient.MockGet(users.UserIndexCompressedFilename, "", nil)
	bucketClient.MockIter("__markers__", []string{}, nil)
	bucketClient.MockIter("", []string{"user-1"}, nil)
	bucketClient.MockIter("user-1/", []string{"user-1/01DTVP434PA9VFXSW2JKB3392D", "user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ", "user-1/01DTVP434PA9VFXSW2JKB3392D/meta.json", "user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/meta.json"}, nil)
	bucketClient.MockIter("user-1/markers/", nil, nil)
	bucketClient.MockGet("user-1/markers/cleaner-visit-marker.json", "", nil)
	bucketClient.MockUpload("user-1/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockDelete("user-1/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockExists(cortex_tsdb.GetGlobalDeletionMarkPath("user-1"), false, nil)
	bucketClient.MockExists(cortex_tsdb.GetLocalDeletionMarkPath("user-1"), false, nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/meta.json", mockBlockMetaJSON("01DTVP434PA9VFXSW2JKB3392D"), nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/deletion-mark.json", "", nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/no-compact-mark.json", "", nil)
	bucketClient.MockGet("user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/meta.json", mockBlockMetaJSON("01DTW0ZCPDDNV4BV83Q2SV4QAZ"), nil)
	bucketClient.MockGet("user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/deletion-mark.json", "", nil)
	bucketClient.MockGet("user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/no-compact-mark.json", "", nil)
	bucketClient.MockGet("user-1/bucket-index.json.gz", "", nil)
	bucketClient.MockGet("user-1/bucket-index-sync-status.json", string(content), nil)
	bucketClient.MockUpload("user-1/bucket-index.json.gz", nil)
	bucketClient.MockUpload("user-1/bucket-index-sync-status.json", nil)

	ringStore, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	t.Cleanup(func() { assert.NoError(t, closer.Close()) })

	cfg := prepareConfig()
	cfg.ShardingEnabled = true
	cfg.ShardingRing.InstanceID = "compactor-1"
	cfg.ShardingRing.InstanceAddr = "1.2.3.4"
	cfg.ShardingRing.KVStore.Mock = ringStore

	c, _, tsdbPlanner, _, _ := prepare(t, cfg, bucketClient, nil)
	tsdbPlanner.On("Plan", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*metadata.Meta{}, bucket.ErrKeyPermissionDenied)

	require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))

	// Wait until a run has completed.
	cortex_testutil.Poll(t, 20*time.Second, 1.0, func() interface{} {
		return prom_testutil.ToFloat64(c.CompactionRunsCompleted)
	})

	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), c))
}

func TestCompactor_FailedWithRetriableError(t *testing.T) {
	t.Parallel()

	ss := bucketindex.Status{Status: bucketindex.Ok, Version: bucketindex.SyncStatusFileVersion}
	content, err := json.Marshal(ss)
	require.NoError(t, err)

	bucketClient := &bucket.ClientMock{}
	bucketClient.MockGet(users.UserIndexCompressedFilename, "", nil)
	bucketClient.MockIter("__markers__", []string{}, nil)
	bucketClient.MockIter("", []string{"user-1"}, nil)
	bucketClient.MockIter("user-1/", []string{"user-1/01DTVP434PA9VFXSW2JKB3392D", "user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ", "user-1/01DTVP434PA9VFXSW2JKB3392D/meta.json", "user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/meta.json"}, nil)
	bucketClient.MockIter("user-1/markers/", nil, nil)
	bucketClient.MockGet("user-1/markers/cleaner-visit-marker.json", "", nil)
	bucketClient.MockUpload("user-1/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockDelete("user-1/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockExists(cortex_tsdb.GetGlobalDeletionMarkPath("user-1"), false, nil)
	bucketClient.MockExists(cortex_tsdb.GetLocalDeletionMarkPath("user-1"), false, nil)
	bucketClient.MockIter("user-1/01DTVP434PA9VFXSW2JKB3392D", nil, errors.New("test retriable error"))
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/meta.json", mockBlockMetaJSON("01DTVP434PA9VFXSW2JKB3392D"), nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/deletion-mark.json", "", nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/no-compact-mark.json", "", nil)
	bucketClient.MockIter("user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ", nil, errors.New("test retriable error"))
	bucketClient.MockGet("user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/meta.json", mockBlockMetaJSON("01DTW0ZCPDDNV4BV83Q2SV4QAZ"), nil)
	bucketClient.MockGet("user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/deletion-mark.json", "", nil)
	bucketClient.MockGet("user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/no-compact-mark.json", "", nil)
	bucketClient.MockGet("user-1/bucket-index.json.gz", "", nil)
	bucketClient.MockGet("user-1/bucket-index-sync-status.json", string(content), nil)
	bucketClient.MockUpload("user-1/bucket-index.json.gz", nil)
	bucketClient.MockUpload("user-1/bucket-index-sync-status.json", nil)

	cfg := prepareConfig()
	cfg.CompactionRetries = 2

	c, _, tsdbPlanner, _, registry := prepare(t, cfg, bucketClient, nil)
	tsdbPlanner.On("Plan", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*metadata.Meta{{BlockMeta: mockBlockMeta("01DTVP434PA9VFXSW2JKB3392D")}, {BlockMeta: mockBlockMeta("01DTW0ZCPDDNV4BV83Q2SV4QAZ")}}, nil)

	require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))

	cortex_testutil.Poll(t, 1*time.Second, 2.0, func() interface{} {
		return prom_testutil.ToFloat64(c.compactorMetrics.compactionErrorsCount.WithLabelValues("user-1", retriableError))
	})

	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), c))

	assert.NoError(t, prom_testutil.GatherAndCompare(registry, strings.NewReader(`
		# HELP cortex_compactor_compaction_error_total Total number of errors from compactions.
		# TYPE cortex_compactor_compaction_error_total counter
		cortex_compactor_compaction_error_total{type="retriable", user="user-1"} 2
	`),
		"cortex_compactor_compaction_error_total",
	))
}

func TestCompactor_FailedWithHaltError(t *testing.T) {
	t.Parallel()

	ss := bucketindex.Status{Status: bucketindex.Ok, Version: bucketindex.SyncStatusFileVersion}
	content, err := json.Marshal(ss)
	require.NoError(t, err)

	bucketClient := &bucket.ClientMock{}
	bucketClient.MockGet(users.UserIndexCompressedFilename, "", nil)
	bucketClient.MockIter("__markers__", []string{}, nil)
	bucketClient.MockIter("", []string{"user-1"}, nil)
	bucketClient.MockIter("user-1/", []string{"user-1/01DTVP434PA9VFXSW2JKB3392D", "user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ", "user-1/01DTVP434PA9VFXSW2JKB3392D/meta.json", "user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/meta.json"}, nil)
	bucketClient.MockIter("user-1/markers/", nil, nil)
	bucketClient.MockGet("user-1/markers/cleaner-visit-marker.json", "", nil)
	bucketClient.MockUpload("user-1/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockDelete("user-1/markers/cleaner-visit-marker.json", nil)
	bucketClient.MockExists(cortex_tsdb.GetGlobalDeletionMarkPath("user-1"), false, nil)
	bucketClient.MockExists(cortex_tsdb.GetLocalDeletionMarkPath("user-1"), false, nil)
	bucketClient.MockIter("user-1/01DTVP434PA9VFXSW2JKB3392D", nil, compact.HaltError{})
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/meta.json", mockBlockMetaJSON("01DTVP434PA9VFXSW2JKB3392D"), nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/deletion-mark.json", "", nil)
	bucketClient.MockGet("user-1/01DTVP434PA9VFXSW2JKB3392D/no-compact-mark.json", "", nil)
	bucketClient.MockIter("user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ", nil, compact.HaltError{})
	bucketClient.MockGet("user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/meta.json", mockBlockMetaJSON("01DTW0ZCPDDNV4BV83Q2SV4QAZ"), nil)
	bucketClient.MockGet("user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/deletion-mark.json", "", nil)
	bucketClient.MockGet("user-1/01DTW0ZCPDDNV4BV83Q2SV4QAZ/no-compact-mark.json", "", nil)
	bucketClient.MockGet("user-1/bucket-index.json.gz", "", nil)
	bucketClient.MockGet("user-1/bucket-index-sync-status.json", string(content), nil)
	bucketClient.MockUpload("user-1/bucket-index.json.gz", nil)
	bucketClient.MockUpload("user-1/bucket-index-sync-status.json", nil)

	cfg := prepareConfig()
	cfg.CompactionRetries = 2

	c, _, tsdbPlanner, _, registry := prepare(t, cfg, bucketClient, nil)
	tsdbPlanner.On("Plan", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*metadata.Meta{{BlockMeta: mockBlockMeta("01DTVP434PA9VFXSW2JKB3392D")}, {BlockMeta: mockBlockMeta("01DTW0ZCPDDNV4BV83Q2SV4QAZ")}}, nil)

	require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))

	cortex_testutil.Poll(t, 1*time.Second, 1.0, func() interface{} {
		return prom_testutil.ToFloat64(c.compactorMetrics.compactionErrorsCount.WithLabelValues("user-1", haltError))
	})

	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), c))

	assert.NoError(t, prom_testutil.GatherAndCompare(registry, strings.NewReader(`
		# HELP cortex_compactor_compaction_error_total Total number of errors from compactions.
		# TYPE cortex_compactor_compaction_error_total counter
		cortex_compactor_compaction_error_total{type="halt", user="user-1"} 1
	`),
		"cortex_compactor_compaction_error_total",
	))
}

func TestCompactor_RingLifecyclerShouldAutoForgetUnhealthyInstances(t *testing.T) {
	// Setup user IDs
	userID := "user-0"
	inmem := objstore.WithNoopInstr(objstore.NewInMemBucket())

	id, err := ulid.New(ulid.Now(), rand.Reader)
	require.NoError(t, err)
	require.NoError(t, inmem.Upload(context.Background(), userID+"/"+id.String()+"/meta.json", strings.NewReader(mockBlockMetaJSON(id.String()))))

	// Create a shared KV Store
	kvstore, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	t.Cleanup(func() { assert.NoError(t, closer.Close()) })

	// Create two compactors
	var compactors []*Compactor

	for i := 0; i < 2; i++ {
		// Setup config
		cfg := prepareConfig()

		cfg.ShardingEnabled = true
		cfg.ShardingRing.InstanceID = fmt.Sprintf("compactor-%d", i)
		cfg.ShardingRing.InstanceAddr = fmt.Sprintf("127.0.0.%d", i)
		cfg.ShardingRing.WaitStabilityMinDuration = time.Second
		cfg.ShardingRing.WaitStabilityMaxDuration = 5 * time.Second
		cfg.ShardingRing.KVStore.Mock = kvstore
		cfg.ShardingRing.HeartbeatPeriod = 200 * time.Millisecond
		cfg.ShardingRing.UnregisterOnShutdown = false
		cfg.ShardingRing.AutoForgetDelay = 400 * time.Millisecond

		// Compactor will get its own temp dir for storing local files.
		compactor, _, tsdbPlanner, _, _ := prepare(t, cfg, inmem, nil)
		compactor.logger = log.NewNopLogger()

		compactors = append(compactors, compactor)

		// Mock the planner as if there's no compaction to do,
		// in order to simplify tests (all in all, we just want to
		// test our logic and not TSDB compactor which we expect to
		// be already tested).
		tsdbPlanner.On("Plan", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*metadata.Meta{}, nil)
	}

	require.Equal(t, 2, len(compactors))
	compactor := compactors[0]
	compactor2 := compactors[1]

	// Start compactors
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), compactor))
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), compactor2))

	// Wait until a run has completed.
	cortex_testutil.Poll(t, 20*time.Second, 1.0, func() interface{} {
		return prom_testutil.ToFloat64(compactor2.CompactionRunsCompleted)
	})

	cortex_testutil.Poll(t, 5000*time.Millisecond, true, func() interface{} {
		healthy, unhealthy, _ := compactor.ring.GetAllInstanceDescs(ring.Reporting)
		return len(healthy) == 2 && len(unhealthy) == 0
	})

	// Make one compactor unhealthy in ring by stopping the
	// compactor service while UnregisterOnShutdown is false
	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), compactor2))

	cortex_testutil.Poll(t, 5000*time.Millisecond, true, func() interface{} {
		healthy, unhealthy, _ := compactor.ring.GetAllInstanceDescs(ring.Reporting)
		return len(healthy) == 1 && len(unhealthy) == 0
	})
}

func TestCompactor_GetShardSizeForUser(t *testing.T) {

	// User to shardsize
	users := []struct {
		userID                        string
		tenantShardSize               float64
		expectedShardSize             int
		expectedShardSizeAfterScaleup int
	}{
		{
			userID:                        "user-1",
			tenantShardSize:               6,
			expectedShardSize:             6,
			expectedShardSizeAfterScaleup: 6,
		},
		{
			userID:                        "user-2",
			tenantShardSize:               1,
			expectedShardSize:             1,
			expectedShardSizeAfterScaleup: 1,
		},
		{
			userID:                        "user-3",
			tenantShardSize:               0.4,
			expectedShardSize:             2,
			expectedShardSizeAfterScaleup: 4,
		},
		{
			userID:                        "user-4",
			tenantShardSize:               0.01,
			expectedShardSize:             1,
			expectedShardSizeAfterScaleup: 1,
		},
	}

	inmem := objstore.WithNoopInstr(objstore.NewInMemBucket())
	tenantLimits := newMockTenantLimits(map[string]*validation.Limits{})

	for _, user := range users {
		id, err := ulid.New(ulid.Now(), rand.Reader)
		require.NoError(t, err)
		require.NoError(t, inmem.Upload(context.Background(), user.userID+"/"+id.String()+"/meta.json", strings.NewReader(mockBlockMetaJSON(id.String()))))
		limits := validation.Limits{}
		flagext.DefaultValues(&limits)
		limits.CompactorTenantShardSize = user.tenantShardSize
		tenantLimits.setLimits(user.userID, &limits)
	}

	// Create a shared KV Store
	kvstore, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	t.Cleanup(func() { assert.NoError(t, closer.Close()) })

	// Create compactors
	var compactors []*Compactor
	for i := 0; i < 5; i++ {
		// Setup config
		cfg := prepareConfig()

		cfg.ShardingEnabled = true
		cfg.ShardingRing.InstanceID = fmt.Sprintf("compactor-%d", i)
		cfg.ShardingRing.InstanceAddr = fmt.Sprintf("127.0.0.%d", i)
		cfg.ShardingRing.WaitStabilityMinDuration = time.Second
		cfg.ShardingRing.WaitStabilityMaxDuration = 5 * time.Second
		cfg.ShardingRing.KVStore.Mock = kvstore

		// Compactor will get its own temp dir for storing local files.
		overrides := validation.NewOverrides(validation.Limits{}, tenantLimits)
		compactor, _, tsdbPlanner, _, _ := prepare(t, cfg, inmem, nil)
		compactor.limits = overrides
		//compactor.limits.tenantLimits = tenantLimits
		compactor.logger = log.NewNopLogger()
		defer services.StopAndAwaitTerminated(context.Background(), compactor) //nolint:errcheck

		compactors = append(compactors, compactor)

		// Mock the planner as if there's no compaction to do,
		// in order to simplify tests (all in all, we just want to
		// test our logic and not TSDB compactor which we expect to
		// be already tested).
		tsdbPlanner.On("Plan", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*metadata.Meta{}, nil)
	}

	// Start all compactors
	for _, c := range compactors {
		require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))
	}

	// Wait until a run has been completed on each compactor
	for _, c := range compactors {
		cortex_testutil.Poll(t, 120*time.Second, true, func() interface{} {
			return prom_testutil.ToFloat64(c.CompactionRunsCompleted) >= 1
		})
	}

	assert.Equal(t, 5, compactors[0].ring.InstancesCount())

	for _, user := range users {
		assert.Equal(t, user.expectedShardSize, compactors[0].getShardSizeForUser(user.userID))
	}

	// Scaleup compactors
	// Create compactors
	var compactors2 []*Compactor
	for i := 5; i < 10; i++ {
		// Setup config
		cfg := prepareConfig()

		cfg.ShardingEnabled = true
		cfg.ShardingRing.InstanceID = fmt.Sprintf("compactor-%d", i)
		cfg.ShardingRing.InstanceAddr = fmt.Sprintf("127.0.0.%d", i)
		cfg.ShardingRing.WaitStabilityMinDuration = time.Second
		cfg.ShardingRing.WaitStabilityMaxDuration = 5 * time.Second
		cfg.ShardingRing.KVStore.Mock = kvstore

		// Compactor will get its own temp dir for storing local files.
		overrides := validation.NewOverrides(validation.Limits{}, tenantLimits)
		compactor, _, tsdbPlanner, _, _ := prepare(t, cfg, inmem, nil)
		compactor.limits = overrides
		//compactor.limits.tenantLimits = tenantLimits
		compactor.logger = log.NewNopLogger()
		defer services.StopAndAwaitTerminated(context.Background(), compactor) //nolint:errcheck

		compactors2 = append(compactors2, compactor)

		// Mock the planner as if there's no compaction to do,
		// in order to simplify tests (all in all, we just want to
		// test our logic and not TSDB compactor which we expect to
		// be already tested).
		tsdbPlanner.On("Plan", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return([]*metadata.Meta{}, nil)
	}

	// Start all compactors
	for _, c := range compactors2 {
		require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))
	}

	// Wait until a run has been completed on each compactor
	for _, c := range compactors2 {
		cortex_testutil.Poll(t, 120*time.Second, true, func() interface{} {
			return prom_testutil.ToFloat64(c.CompactionRunsCompleted) >= 1
		})
	}

	assert.Equal(t, 10, compactors[0].ring.InstancesCount())

	for _, user := range users {
		assert.Equal(t, user.expectedShardSizeAfterScaleup, compactors[0].getShardSizeForUser(user.userID))
	}
}

type mockTenantLimits struct {
	limits map[string]*validation.Limits
	m      sync.Mutex
}

// newMockTenantLimits creates a new mockTenantLimits that returns per-tenant limits based on
// the given map
func newMockTenantLimits(limits map[string]*validation.Limits) *mockTenantLimits {
	return &mockTenantLimits{
		limits: limits,
	}
}

func (l *mockTenantLimits) ByUserID(userID string) *validation.Limits {
	l.m.Lock()
	defer l.m.Unlock()
	return l.limits[userID]
}

func (l *mockTenantLimits) AllByUserID() map[string]*validation.Limits {
	l.m.Lock()
	defer l.m.Unlock()
	return l.limits
}

func (l *mockTenantLimits) setLimits(userID string, limits *validation.Limits) {
	l.m.Lock()
	defer l.m.Unlock()
	l.limits[userID] = limits
}

func TestCompactor_UserIndexUpdateLoop(t *testing.T) {
	// Prepare test dependencies
	bucketClient, _ := cortex_storage_testutil.PrepareFilesystemBucket(t)
	bucketClient = bucketindex.BucketWithGlobalMarkers(bucketClient)

	ringStore, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	t.Cleanup(func() { assert.NoError(t, closer.Close()) })

	cfg := prepareConfig()
	cfg.ShardingEnabled = true
	cfg.ShardingRing.InstanceID = "compactor-1"
	cfg.ShardingRing.InstanceAddr = "1.2.3.4"
	cfg.ShardingRing.KVStore.Mock = ringStore
	cfg.CleanupInterval = 100 * time.Millisecond // Short interval for testing

	compactor, _, _, _, _ := prepare(t, cfg, bucketClient, &validation.Limits{})

	// Start the compactor service
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), compactor))

	// Wait for the user index file to be created
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Poll for the user index file
	for {
		exists, err := bucketClient.Exists(ctx, "user-index.json.gz")
		require.NoError(t, err)
		if exists {
			break
		}

		select {
		case <-ctx.Done():
			t.Fatal("Timeout waiting for user index file to be created")
		case <-time.After(100 * time.Millisecond):
			// Continue polling
		}
	}
}
