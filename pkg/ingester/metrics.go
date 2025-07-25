package ingester

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/cortexproject/cortex/pkg/storage/tsdb"
	"github.com/cortexproject/cortex/pkg/util"
	util_math "github.com/cortexproject/cortex/pkg/util/math"
)

const (
	memSeriesCreatedTotalName = "cortex_ingester_memory_series_created_total"
	memSeriesCreatedTotalHelp = "The total number of series that were created per user."

	memSeriesRemovedTotalName = "cortex_ingester_memory_series_removed_total"
	memSeriesRemovedTotalHelp = "The total number of series that were removed per user."
)

const (
	sampleMetricTypeFloat     = "float"
	sampleMetricTypeHistogram = "histogram"
)

type ingesterMetrics struct {
	ingestedSamples         prometheus.Counter
	ingestedHistograms      prometheus.Counter
	ingestedExemplars       prometheus.Counter
	ingestedMetadata        prometheus.Counter
	ingestedSamplesFail     prometheus.Counter
	ingestedHistogramsFail  prometheus.Counter
	ingestedExemplarsFail   prometheus.Counter
	ingestedMetadataFail    prometheus.Counter
	oooLabelsTotal          *prometheus.CounterVec
	queries                 prometheus.Counter
	queriedSamples          prometheus.Histogram
	queriedExemplars        prometheus.Histogram
	queriedSeries           prometheus.Histogram
	queriedChunks           prometheus.Histogram
	memSeries               prometheus.Gauge
	memMetadata             prometheus.Gauge
	memUsers                prometheus.Gauge
	memSeriesCreatedTotal   *prometheus.CounterVec
	memMetadataCreatedTotal *prometheus.CounterVec
	memSeriesRemovedTotal   *prometheus.CounterVec
	memMetadataRemovedTotal *prometheus.CounterVec
	pushErrorsTotal         *prometheus.CounterVec

	activeSeriesPerUser   *prometheus.GaugeVec
	activeNHSeriesPerUser *prometheus.GaugeVec
	limitsPerLabelSet     *prometheus.GaugeVec
	usagePerLabelSet      *prometheus.GaugeVec

	// Global limit metrics
	maxUsersGauge           prometheus.GaugeFunc
	maxSeriesGauge          prometheus.GaugeFunc
	maxIngestionRate        prometheus.GaugeFunc
	maxInflightPushRequests prometheus.GaugeFunc

	// Current Usage
	ingestionRate         prometheus.GaugeFunc
	inflightRequests      prometheus.GaugeFunc
	inflightQueryRequests prometheus.GaugeFunc

	// Posting Cache Metrics
	expandedPostingsCacheMetrics *tsdb.ExpandedPostingsCacheMetrics
}

func newIngesterMetrics(r prometheus.Registerer,
	createMetricsConflictingWithTSDB bool,
	activeSeriesEnabled bool,
	instanceLimitsFn func() *InstanceLimits,
	ingestionRate *util_math.EwmaRate,
	inflightPushRequests *util_math.MaxTracker,
	maxInflightQueryRequests *util_math.MaxTracker,
	postingsCacheEnabled bool,
) *ingesterMetrics {
	const (
		instanceLimits     = "cortex_ingester_instance_limits"
		instanceLimitsHelp = "Instance limits used by this ingester." // Must be same for all registrations.
		limitLabel         = "limit"
	)

	m := &ingesterMetrics{
		ingestedSamples: promauto.With(r).NewCounter(prometheus.CounterOpts{
			Name: "cortex_ingester_ingested_samples_total",
			Help: "The total number of samples ingested.",
		}),
		ingestedHistograms: promauto.With(r).NewCounter(prometheus.CounterOpts{
			Name: "cortex_ingester_ingested_native_histograms_total",
			Help: "The total number of native histograms ingested.",
		}),
		ingestedExemplars: promauto.With(r).NewCounter(prometheus.CounterOpts{
			Name: "cortex_ingester_ingested_exemplars_total",
			Help: "The total number of exemplars ingested.",
		}),
		ingestedMetadata: promauto.With(r).NewCounter(prometheus.CounterOpts{
			Name: "cortex_ingester_ingested_metadata_total",
			Help: "The total number of metadata ingested.",
		}),
		ingestedSamplesFail: promauto.With(r).NewCounter(prometheus.CounterOpts{
			Name: "cortex_ingester_ingested_samples_failures_total",
			Help: "The total number of samples that errored on ingestion.",
		}),
		ingestedHistogramsFail: promauto.With(r).NewCounter(prometheus.CounterOpts{
			Name: "cortex_ingester_ingested_native_histograms_failures_total",
			Help: "The total number of native histograms that errored on ingestion.",
		}),
		ingestedExemplarsFail: promauto.With(r).NewCounter(prometheus.CounterOpts{
			Name: "cortex_ingester_ingested_exemplars_failures_total",
			Help: "The total number of exemplars that errored on ingestion.",
		}),
		ingestedMetadataFail: promauto.With(r).NewCounter(prometheus.CounterOpts{
			Name: "cortex_ingester_ingested_metadata_failures_total",
			Help: "The total number of metadata that errored on ingestion.",
		}),
		oooLabelsTotal: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "cortex_ingester_out_of_order_labels_total",
			Help: "The total number of out of order label found per user.",
		}, []string{"user"}),
		queries: promauto.With(r).NewCounter(prometheus.CounterOpts{
			Name: "cortex_ingester_queries_total",
			Help: "The total number of queries the ingester has handled.",
		}),
		queriedSamples: promauto.With(r).NewHistogram(prometheus.HistogramOpts{
			Name: "cortex_ingester_queried_samples",
			Help: "The total number of samples returned from queries.",
			// Could easily return 10m samples per query - 10*(8^(8-1)) = 20.9m.
			Buckets: prometheus.ExponentialBuckets(10, 8, 8),
		}),
		queriedExemplars: promauto.With(r).NewHistogram(prometheus.HistogramOpts{
			Name: "cortex_ingester_queried_exemplars",
			Help: "The total number of exemplars returned from queries.",
			// A reasonable upper bound is around 6k - 10*(5^(5-1)) = 6250.
			Buckets: prometheus.ExponentialBuckets(10, 5, 5),
		}),
		queriedSeries: promauto.With(r).NewHistogram(prometheus.HistogramOpts{
			Name: "cortex_ingester_queried_series",
			Help: "The total number of series returned from queries.",
			// A reasonable upper bound is around 100k - 10*(8^(6-1)) = 327k.
			Buckets: prometheus.ExponentialBuckets(10, 8, 6),
		}),
		queriedChunks: promauto.With(r).NewHistogram(prometheus.HistogramOpts{
			Name: "cortex_ingester_queried_chunks",
			Help: "The total number of chunks returned from queries.",
			// A small number of chunks per series - 10*(8^(7-1)) = 2.6m.
			Buckets: prometheus.ExponentialBuckets(10, 8, 7),
		}),
		memSeries: promauto.With(r).NewGauge(prometheus.GaugeOpts{
			Name: "cortex_ingester_memory_series",
			Help: "The current number of series in memory.",
		}),
		memMetadata: promauto.With(r).NewGauge(prometheus.GaugeOpts{
			Name: "cortex_ingester_memory_metadata",
			Help: "The current number of metadata in memory.",
		}),
		memUsers: promauto.With(r).NewGauge(prometheus.GaugeOpts{
			Name: "cortex_ingester_memory_users",
			Help: "The current number of users in memory.",
		}),
		memMetadataCreatedTotal: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "cortex_ingester_memory_metadata_created_total",
			Help: "The total number of metadata that were created per user",
		}, []string{"user"}),
		memMetadataRemovedTotal: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "cortex_ingester_memory_metadata_removed_total",
			Help: "The total number of metadata that were removed per user.",
		}, []string{"user"}),
		pushErrorsTotal: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: "cortex_ingester_push_errors_total",
			Help: "The total number of push errors per user.",
		}, []string{"user", "reason"}),

		maxUsersGauge: promauto.With(r).NewGaugeFunc(prometheus.GaugeOpts{
			Name:        instanceLimits,
			Help:        instanceLimitsHelp,
			ConstLabels: map[string]string{limitLabel: "max_tenants"},
		}, func() float64 {
			if g := instanceLimitsFn(); g != nil {
				return float64(g.MaxInMemoryTenants)
			}
			return 0
		}),

		maxSeriesGauge: promauto.With(r).NewGaugeFunc(prometheus.GaugeOpts{
			Name:        instanceLimits,
			Help:        instanceLimitsHelp,
			ConstLabels: map[string]string{limitLabel: "max_series"},
		}, func() float64 {
			if g := instanceLimitsFn(); g != nil {
				return float64(g.MaxInMemorySeries)
			}
			return 0
		}),

		maxIngestionRate: promauto.With(r).NewGaugeFunc(prometheus.GaugeOpts{
			Name:        instanceLimits,
			Help:        instanceLimitsHelp,
			ConstLabels: map[string]string{limitLabel: "max_ingestion_rate"},
		}, func() float64 {
			if g := instanceLimitsFn(); g != nil {
				return float64(g.MaxIngestionRate)
			}
			return 0
		}),

		maxInflightPushRequests: promauto.With(r).NewGaugeFunc(prometheus.GaugeOpts{
			Name:        instanceLimits,
			Help:        instanceLimitsHelp,
			ConstLabels: map[string]string{limitLabel: "max_inflight_push_requests"},
		}, func() float64 {
			if g := instanceLimitsFn(); g != nil {
				return float64(g.MaxInflightPushRequests)
			}
			return 0
		}),

		ingestionRate: promauto.With(r).NewGaugeFunc(prometheus.GaugeOpts{
			Name: "cortex_ingester_ingestion_rate_samples_per_second",
			Help: "Current ingestion rate in samples/sec that ingester is using to limit access.",
		}, func() float64 {
			if ingestionRate != nil {
				return ingestionRate.Rate()
			}
			return 0
		}),

		inflightRequests: promauto.With(r).NewGaugeFunc(prometheus.GaugeOpts{
			Name: "cortex_ingester_inflight_push_requests",
			Help: "Max number of inflight push requests in ingester in the last minute.",
		}, func() float64 {
			if inflightPushRequests != nil {
				return float64(inflightPushRequests.Load())
			}
			return 0
		}),

		inflightQueryRequests: promauto.With(r).NewGaugeFunc(prometheus.GaugeOpts{
			Name: "cortex_ingester_max_inflight_query_requests",
			Help: "Max number of inflight query requests in ingester.",
		}, func() float64 {
			if maxInflightQueryRequests != nil {
				return float64(maxInflightQueryRequests.Load())
			}
			return 0
		}),

		limitsPerLabelSet: promauto.With(r).NewGaugeVec(prometheus.GaugeOpts{
			Name: "cortex_ingester_limits_per_labelset",
			Help: "Limits per user and labelset.",
		}, []string{"user", "limit", "labelset"}),

		usagePerLabelSet: promauto.With(r).NewGaugeVec(prometheus.GaugeOpts{
			Name: "cortex_ingester_usage_per_labelset",
			Help: "Current usage per user and labelset.",
		}, []string{"user", "limit", "labelset"}),

		// Not registered automatically, but only if activeSeriesEnabled is true.
		activeSeriesPerUser: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "cortex_ingester_active_series",
			Help: "Number of currently active series per user.",
		}, []string{"user"}),

		// Not registered automatically, but only if activeSeriesEnabled is true.
		activeNHSeriesPerUser: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "cortex_ingester_active_native_histogram_series",
			Help: "Number of currently active native histogram series per user.",
		}, []string{"user"}),
	}

	if postingsCacheEnabled && r != nil {
		m.expandedPostingsCacheMetrics = tsdb.NewPostingCacheMetrics(r)
	}

	if activeSeriesEnabled && r != nil {
		r.MustRegister(m.activeSeriesPerUser)
		r.MustRegister(m.activeNHSeriesPerUser)
	}

	if createMetricsConflictingWithTSDB {
		m.memSeriesCreatedTotal = promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: memSeriesCreatedTotalName,
			Help: memSeriesCreatedTotalHelp,
		}, []string{"user"})

		m.memSeriesRemovedTotal = promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Name: memSeriesRemovedTotalName,
			Help: memSeriesRemovedTotalHelp,
		}, []string{"user"})
	}

	return m
}

func (m *ingesterMetrics) deletePerUserMetrics(userID string) {
	m.oooLabelsTotal.DeleteLabelValues(userID)
	m.memMetadataCreatedTotal.DeleteLabelValues(userID)
	m.memMetadataRemovedTotal.DeleteLabelValues(userID)
	m.activeSeriesPerUser.DeleteLabelValues(userID)
	m.activeNHSeriesPerUser.DeleteLabelValues(userID)
	m.usagePerLabelSet.DeletePartialMatch(prometheus.Labels{"user": userID})
	m.limitsPerLabelSet.DeletePartialMatch(prometheus.Labels{"user": userID})
	m.pushErrorsTotal.DeletePartialMatch(prometheus.Labels{"user": userID})

	if m.memSeriesCreatedTotal != nil {
		m.memSeriesCreatedTotal.DeleteLabelValues(userID)
	}

	if m.memSeriesRemovedTotal != nil {
		m.memSeriesRemovedTotal.DeleteLabelValues(userID)
	}
}

// TSDB metrics collector. Each tenant has its own registry, that TSDB code uses.
type tsdbMetrics struct {
	// Metrics aggregated from Thanos shipper.
	dirSyncs        *prometheus.Desc // sum(thanos_shipper_dir_syncs_total)
	dirSyncFailures *prometheus.Desc // sum(thanos_shipper_dir_sync_failures_total)
	uploads         *prometheus.Desc // sum(thanos_shipper_uploads_total)
	uploadFailures  *prometheus.Desc // sum(thanos_shipper_upload_failures_total)
	corruptedBlocks *prometheus.Desc // sum(thanos_shipper_corrupted_blocks_total)

	// Metrics aggregated from TSDB.
	tsdbCompactionsTotal               *prometheus.Desc
	tsdbCompactionDuration             *prometheus.Desc
	tsdbFsyncDuration                  *prometheus.Desc
	tsdbPageFlushes                    *prometheus.Desc
	tsdbPageCompletions                *prometheus.Desc
	tsdbWALTruncateFail                *prometheus.Desc
	tsdbWALTruncateTotal               *prometheus.Desc
	tsdbWALTruncateDuration            *prometheus.Desc
	tsdbWALCorruptionsTotal            *prometheus.Desc
	tsdbWALWritesFailed                *prometheus.Desc
	tsdbHeadTruncateFail               *prometheus.Desc
	tsdbHeadTruncateTotal              *prometheus.Desc
	tsdbHeadGcDuration                 *prometheus.Desc
	tsdbActiveAppenders                *prometheus.Desc
	tsdbSeriesNotFound                 *prometheus.Desc
	tsdbChunks                         *prometheus.Desc
	tsdbChunksCreatedTotal             *prometheus.Desc
	tsdbChunksRemovedTotal             *prometheus.Desc
	tsdbMmapChunkCorruptionTotal       *prometheus.Desc
	tsdbChunkwriteQueueOperationsTotal *prometheus.Desc
	tsdbSamplesAppended                *prometheus.Desc
	// Although there is an existing sample-out-of-order discarded samples metric, some samples can still
	// be dropped silently due to OOO at commit phase, and it doesn't increment the discarded samples metric.
	tsdbOOOSamples                *prometheus.Desc
	tsdbOutOfOrderSamplesAppended *prometheus.Desc
	tsdbSnapshotReplayErrorTotal  *prometheus.Desc
	tsdbOOOHistogram              *prometheus.Desc
	tsdbMmapChunksTotal           *prometheus.Desc
	tsdbDataTotalReplayDuration   *prometheus.Desc

	tsdbExemplarsTotal          *prometheus.Desc
	tsdbExemplarsInStorage      *prometheus.Desc
	tsdbExemplarSeriesInStorage *prometheus.Desc
	tsdbExemplarLastTs          *prometheus.Desc
	tsdbExemplarsOutOfOrder     *prometheus.Desc

	// Follow metrics are from https://github.com/prometheus/prometheus/blob/fbe960f2c1ad9d6f5fe2f267d2559bf7ecfab6df/tsdb/db.go#L179
	tsdbLoadedBlocks       *prometheus.Desc
	tsdbSymbolTableSize    *prometheus.Desc
	tsdbReloads            *prometheus.Desc
	tsdbReloadsFailed      *prometheus.Desc
	tsdbTimeRetentionCount *prometheus.Desc
	tsdbBlocksBytes        *prometheus.Desc

	checkpointDeleteFail    *prometheus.Desc
	checkpointDeleteTotal   *prometheus.Desc
	checkpointCreationFail  *prometheus.Desc
	checkpointCreationTotal *prometheus.Desc

	// These two metrics replace metrics in ingesterMetrics, as we count them differently
	memSeriesCreatedTotal *prometheus.Desc
	memSeriesRemovedTotal *prometheus.Desc

	regs *util.UserRegistries
}

func newTSDBMetrics(r prometheus.Registerer) *tsdbMetrics {
	m := &tsdbMetrics{
		regs: util.NewUserRegistries(),

		dirSyncs: prometheus.NewDesc(
			"cortex_ingester_shipper_dir_syncs_total",
			"Total number of TSDB dir syncs",
			nil, nil),
		dirSyncFailures: prometheus.NewDesc(
			"cortex_ingester_shipper_dir_sync_failures_total",
			"Total number of failed TSDB dir syncs",
			nil, nil),
		uploads: prometheus.NewDesc(
			"cortex_ingester_shipper_uploads_total",
			"Total number of uploaded TSDB blocks",
			nil, nil),
		uploadFailures: prometheus.NewDesc(
			"cortex_ingester_shipper_upload_failures_total",
			"Total number of TSDB block upload failures",
			nil, nil),
		corruptedBlocks: prometheus.NewDesc(
			"cortex_ingester_shipper_corrupted_blocks_total",
			"Total number of TSDB blocks corrupted",
			nil, nil),
		tsdbCompactionsTotal: prometheus.NewDesc(
			"cortex_ingester_tsdb_compactions_total",
			"Total number of TSDB compactions that were executed.",
			nil, nil),
		tsdbCompactionDuration: prometheus.NewDesc(
			"cortex_ingester_tsdb_compaction_duration_seconds",
			"Duration of TSDB compaction runs.",
			nil, nil),
		tsdbFsyncDuration: prometheus.NewDesc(
			"cortex_ingester_tsdb_wal_fsync_duration_seconds",
			"Duration of TSDB WAL fsync.",
			nil, nil),
		tsdbPageFlushes: prometheus.NewDesc(
			"cortex_ingester_tsdb_wal_page_flushes_total",
			"Total number of TSDB WAL page flushes.",
			nil, nil),
		tsdbPageCompletions: prometheus.NewDesc(
			"cortex_ingester_tsdb_wal_completed_pages_total",
			"Total number of TSDB WAL completed pages.",
			nil, nil),
		tsdbWALTruncateFail: prometheus.NewDesc(
			"cortex_ingester_tsdb_wal_truncations_failed_total",
			"Total number of TSDB WAL truncations that failed.",
			nil, nil),
		tsdbWALTruncateTotal: prometheus.NewDesc(
			"cortex_ingester_tsdb_wal_truncations_total",
			"Total number of TSDB  WAL truncations attempted.",
			nil, nil),
		tsdbWALTruncateDuration: prometheus.NewDesc(
			"cortex_ingester_tsdb_wal_truncate_duration_seconds",
			"Duration of TSDB WAL truncation.",
			nil, nil),
		tsdbWALCorruptionsTotal: prometheus.NewDesc(
			"cortex_ingester_tsdb_wal_corruptions_total",
			"Total number of TSDB WAL corruptions.",
			nil, nil),
		tsdbWALWritesFailed: prometheus.NewDesc(
			"cortex_ingester_tsdb_wal_writes_failed_total",
			"Total number of TSDB WAL writes that failed.",
			nil, nil),
		tsdbHeadTruncateFail: prometheus.NewDesc(
			"cortex_ingester_tsdb_head_truncations_failed_total",
			"Total number of TSDB head truncations that failed.",
			nil, nil),
		tsdbHeadTruncateTotal: prometheus.NewDesc(
			"cortex_ingester_tsdb_head_truncations_total",
			"Total number of TSDB head truncations attempted.",
			nil, nil),
		tsdbHeadGcDuration: prometheus.NewDesc(
			"cortex_ingester_tsdb_head_gc_duration_seconds",
			"Runtime of garbage collection in the TSDB head.",
			nil, nil),
		tsdbActiveAppenders: prometheus.NewDesc(
			"cortex_ingester_tsdb_head_active_appenders",
			"Number of currently active TSDB appender transactions.",
			nil, nil),
		tsdbSeriesNotFound: prometheus.NewDesc(
			"cortex_ingester_tsdb_head_series_not_found_total",
			"Total number of TSDB requests for series that were not found.",
			nil, nil),
		tsdbChunks: prometheus.NewDesc(
			"cortex_ingester_tsdb_head_chunks",
			"Total number of chunks in the TSDB head block.",
			nil, nil),
		tsdbChunksCreatedTotal: prometheus.NewDesc(
			"cortex_ingester_tsdb_head_chunks_created_total",
			"Total number of series created in the TSDB head.",
			[]string{"user"}, nil),
		tsdbChunksRemovedTotal: prometheus.NewDesc(
			"cortex_ingester_tsdb_head_chunks_removed_total",
			"Total number of series removed in the TSDB head.",
			[]string{"user"}, nil),
		tsdbMmapChunkCorruptionTotal: prometheus.NewDesc(
			"cortex_ingester_tsdb_mmap_chunk_corruptions_total",
			"Total number of memory-mapped TSDB chunk corruptions.",
			nil, nil),
		tsdbChunkwriteQueueOperationsTotal: prometheus.NewDesc(
			"cortex_ingester_tsdb_chunk_write_queue_operations_total",
			"Number of currently tsdb chunk write queues.",
			[]string{"user", "operation"}, nil),
		tsdbDataTotalReplayDuration: prometheus.NewDesc(
			"cortex_ingester_tsdb_data_replay_duration_seconds",
			"Time taken to replay the tsdb data on disk.",
			[]string{"user"}, nil),
		tsdbLoadedBlocks: prometheus.NewDesc(
			"cortex_ingester_tsdb_blocks_loaded",
			"Number of currently loaded data blocks",
			nil, nil),
		tsdbReloads: prometheus.NewDesc(
			"cortex_ingester_tsdb_reloads_total",
			"Number of times the database reloaded block data from disk.",
			nil, nil),
		tsdbReloadsFailed: prometheus.NewDesc(
			"cortex_ingester_tsdb_reloads_failures_total",
			"Number of times the database failed to reloadBlocks block data from disk.",
			nil, nil),
		tsdbSymbolTableSize: prometheus.NewDesc(
			"cortex_ingester_tsdb_symbol_table_size_bytes",
			"Size of symbol table in memory for loaded blocks",
			[]string{"user"}, nil),
		tsdbBlocksBytes: prometheus.NewDesc(
			"cortex_ingester_tsdb_storage_blocks_bytes",
			"The number of bytes that are currently used for local storage by all blocks.",
			[]string{"user"}, nil),
		tsdbTimeRetentionCount: prometheus.NewDesc(
			"cortex_ingester_tsdb_time_retentions_total",
			"The number of times that blocks were deleted because the maximum time limit was exceeded.",
			nil, nil),
		checkpointDeleteFail: prometheus.NewDesc(
			"cortex_ingester_tsdb_checkpoint_deletions_failed_total",
			"Total number of TSDB checkpoint deletions that failed.",
			nil, nil),
		checkpointDeleteTotal: prometheus.NewDesc(
			"cortex_ingester_tsdb_checkpoint_deletions_total",
			"Total number of TSDB checkpoint deletions attempted.",
			nil, nil),
		checkpointCreationFail: prometheus.NewDesc(
			"cortex_ingester_tsdb_checkpoint_creations_failed_total",
			"Total number of TSDB checkpoint creations that failed.",
			nil, nil),
		checkpointCreationTotal: prometheus.NewDesc(
			"cortex_ingester_tsdb_checkpoint_creations_total",
			"Total number of TSDB checkpoint creations attempted.",
			nil, nil),
		tsdbSamplesAppended: prometheus.NewDesc(
			"cortex_ingester_tsdb_head_samples_appended_total",
			"Total number of appended samples.",
			[]string{"user", "type"}, nil),
		tsdbOOOSamples: prometheus.NewDesc(
			"cortex_ingester_tsdb_out_of_order_samples_total",
			"Total number of out of order samples ingestion failed attempts due to out of order being disabled.",
			[]string{"user", "type"}, nil),
		tsdbOutOfOrderSamplesAppended: prometheus.NewDesc(
			"cortex_ingester_tsdb_head_out_of_order_samples_appended_total",
			"Total number of appended out of order samples.",
			[]string{"user", "type"}, nil),
		tsdbSnapshotReplayErrorTotal: prometheus.NewDesc(
			"cortex_ingester_tsdb_snapshot_replay_error_total",
			"Total number snapshot replays that failed.",
			nil, nil),
		tsdbOOOHistogram: prometheus.NewDesc(
			"cortex_ingester_tsdb_sample_ooo_delta",
			"Delta in seconds by which a sample is considered out of order (reported regardless of OOO time window and whether sample is accepted or not).",
			nil, nil),
		tsdbMmapChunksTotal: prometheus.NewDesc(
			"cortex_ingester_tsdb_mmap_chunks_total",
			"Total number of chunks that were memory-mapped.",
			nil, nil),

		// The most useful exemplar metrics are per-user. The rest
		// are global to reduce metrics overhead.
		tsdbExemplarsTotal: prometheus.NewDesc(
			"cortex_ingester_tsdb_exemplar_exemplars_appended_total",
			"Total number of TSDB exemplars appended.",
			nil, nil), // see distributor_exemplars_in for per-user rate
		tsdbExemplarsInStorage: prometheus.NewDesc(
			"cortex_ingester_tsdb_exemplar_exemplars_in_storage",
			"Number of TSDB exemplars currently in storage.",
			nil, nil),
		tsdbExemplarSeriesInStorage: prometheus.NewDesc(
			"cortex_ingester_tsdb_exemplar_series_with_exemplars_in_storage",
			"Number of TSDB series with exemplars currently in storage.",
			[]string{"user"}, nil),
		tsdbExemplarLastTs: prometheus.NewDesc(
			"cortex_ingester_tsdb_exemplar_last_exemplars_timestamp_seconds",
			"The timestamp of the oldest exemplar stored in circular storage. Useful to check for what time "+
				"range the current exemplar buffer limit allows. This usually means the last timestamp "+
				"for all exemplars for a typical setup. This is not true though if one of the series timestamp is in future compared to rest series.",
			[]string{"user"}, nil),
		tsdbExemplarsOutOfOrder: prometheus.NewDesc(
			"cortex_ingester_tsdb_exemplar_out_of_order_exemplars_total",
			"Total number of out of order exemplar ingestion failed attempts.",
			nil, nil),

		memSeriesCreatedTotal: prometheus.NewDesc(memSeriesCreatedTotalName, memSeriesCreatedTotalHelp, []string{"user"}, nil),
		memSeriesRemovedTotal: prometheus.NewDesc(memSeriesRemovedTotalName, memSeriesRemovedTotalHelp, []string{"user"}, nil),
	}

	if r != nil {
		r.MustRegister(m)
	}
	return m
}

func (sm *tsdbMetrics) Describe(out chan<- *prometheus.Desc) {
	out <- sm.dirSyncs
	out <- sm.dirSyncFailures
	out <- sm.uploads
	out <- sm.uploadFailures
	out <- sm.corruptedBlocks

	out <- sm.tsdbCompactionsTotal
	out <- sm.tsdbCompactionDuration
	out <- sm.tsdbFsyncDuration
	out <- sm.tsdbPageFlushes
	out <- sm.tsdbPageCompletions
	out <- sm.tsdbWALTruncateFail
	out <- sm.tsdbWALTruncateTotal
	out <- sm.tsdbWALTruncateDuration
	out <- sm.tsdbWALCorruptionsTotal
	out <- sm.tsdbWALWritesFailed
	out <- sm.tsdbHeadTruncateFail
	out <- sm.tsdbHeadTruncateTotal
	out <- sm.tsdbHeadGcDuration
	out <- sm.tsdbActiveAppenders
	out <- sm.tsdbSeriesNotFound
	out <- sm.tsdbChunks
	out <- sm.tsdbChunksCreatedTotal
	out <- sm.tsdbChunksRemovedTotal
	out <- sm.tsdbMmapChunkCorruptionTotal
	out <- sm.tsdbChunkwriteQueueOperationsTotal
	out <- sm.tsdbDataTotalReplayDuration
	out <- sm.tsdbLoadedBlocks
	out <- sm.tsdbSymbolTableSize
	out <- sm.tsdbReloads
	out <- sm.tsdbReloadsFailed
	out <- sm.tsdbTimeRetentionCount
	out <- sm.tsdbBlocksBytes
	out <- sm.tsdbSamplesAppended
	out <- sm.tsdbOOOSamples
	out <- sm.tsdbOutOfOrderSamplesAppended
	out <- sm.tsdbSnapshotReplayErrorTotal
	out <- sm.tsdbOOOHistogram
	out <- sm.tsdbMmapChunksTotal
	out <- sm.checkpointDeleteFail
	out <- sm.checkpointDeleteTotal
	out <- sm.checkpointCreationFail
	out <- sm.checkpointCreationTotal

	out <- sm.tsdbExemplarsTotal
	out <- sm.tsdbExemplarsInStorage
	out <- sm.tsdbExemplarSeriesInStorage
	out <- sm.tsdbExemplarLastTs
	out <- sm.tsdbExemplarsOutOfOrder

	out <- sm.memSeriesCreatedTotal
	out <- sm.memSeriesRemovedTotal
}

func (sm *tsdbMetrics) Collect(out chan<- prometheus.Metric) {
	data := sm.regs.BuildMetricFamiliesPerUser()

	// OK, we have it all. Let's build results.
	data.SendSumOfCounters(out, sm.dirSyncs, "thanos_shipper_dir_syncs_total")
	data.SendSumOfCounters(out, sm.dirSyncFailures, "thanos_shipper_dir_sync_failures_total")
	data.SendSumOfCounters(out, sm.uploads, "thanos_shipper_uploads_total")
	data.SendSumOfCounters(out, sm.uploadFailures, "thanos_shipper_upload_failures_total")
	data.SendSumOfCounters(out, sm.corruptedBlocks, "thanos_shipper_corrupted_blocks_total")

	data.SendSumOfCounters(out, sm.tsdbCompactionsTotal, "prometheus_tsdb_compactions_total")
	data.SendSumOfHistograms(out, sm.tsdbCompactionDuration, "prometheus_tsdb_compaction_duration_seconds")
	data.SendSumOfSummaries(out, sm.tsdbFsyncDuration, "prometheus_tsdb_wal_fsync_duration_seconds")
	data.SendSumOfCounters(out, sm.tsdbPageFlushes, "prometheus_tsdb_wal_page_flushes_total")
	data.SendSumOfCounters(out, sm.tsdbPageCompletions, "prometheus_tsdb_wal_completed_pages_total")
	data.SendSumOfCounters(out, sm.tsdbWALTruncateFail, "prometheus_tsdb_wal_truncations_failed_total")
	data.SendSumOfCounters(out, sm.tsdbWALTruncateTotal, "prometheus_tsdb_wal_truncations_total")
	data.SendSumOfSummaries(out, sm.tsdbWALTruncateDuration, "prometheus_tsdb_wal_truncate_duration_seconds")
	data.SendSumOfCounters(out, sm.tsdbWALCorruptionsTotal, "prometheus_tsdb_wal_corruptions_total")
	data.SendSumOfCounters(out, sm.tsdbWALWritesFailed, "prometheus_tsdb_wal_writes_failed_total")
	data.SendSumOfCounters(out, sm.tsdbHeadTruncateFail, "prometheus_tsdb_head_truncations_failed_total")
	data.SendSumOfCounters(out, sm.tsdbHeadTruncateTotal, "prometheus_tsdb_head_truncations_total")
	data.SendSumOfSummaries(out, sm.tsdbHeadGcDuration, "prometheus_tsdb_head_gc_duration_seconds")
	data.SendSumOfGauges(out, sm.tsdbActiveAppenders, "prometheus_tsdb_head_active_appenders")
	data.SendSumOfCounters(out, sm.tsdbSeriesNotFound, "prometheus_tsdb_head_series_not_found_total")
	data.SendSumOfGauges(out, sm.tsdbChunks, "prometheus_tsdb_head_chunks")
	data.SendSumOfCountersPerUser(out, sm.tsdbChunksCreatedTotal, "prometheus_tsdb_head_chunks_created_total")
	data.SendSumOfCountersPerUser(out, sm.tsdbChunksRemovedTotal, "prometheus_tsdb_head_chunks_removed_total")
	data.SendSumOfCounters(out, sm.tsdbMmapChunkCorruptionTotal, "prometheus_tsdb_mmap_chunk_corruptions_total")
	data.SendSumOfCountersPerUserWithLabels(out, sm.tsdbChunkwriteQueueOperationsTotal, "prometheus_tsdb_chunk_write_queue_operations_total", "operation")
	data.SendSumOfGaugesPerUser(out, sm.tsdbDataTotalReplayDuration, "prometheus_tsdb_data_replay_duration_seconds")
	data.SendSumOfGauges(out, sm.tsdbLoadedBlocks, "prometheus_tsdb_blocks_loaded")
	data.SendSumOfGaugesPerUser(out, sm.tsdbSymbolTableSize, "prometheus_tsdb_symbol_table_size_bytes")
	data.SendSumOfCounters(out, sm.tsdbReloads, "prometheus_tsdb_reloads_total")
	data.SendSumOfCounters(out, sm.tsdbReloadsFailed, "prometheus_tsdb_reloads_failures_total")
	data.SendSumOfCounters(out, sm.tsdbTimeRetentionCount, "prometheus_tsdb_time_retentions_total")
	data.SendSumOfGaugesPerUser(out, sm.tsdbBlocksBytes, "prometheus_tsdb_storage_blocks_bytes")
	data.SendSumOfCountersPerUserWithLabels(out, sm.tsdbSamplesAppended, "prometheus_tsdb_head_samples_appended_total", "type")
	data.SendSumOfCountersPerUserWithLabels(out, sm.tsdbOOOSamples, "prometheus_tsdb_out_of_order_samples_total", "type")
	data.SendSumOfCountersPerUserWithLabels(out, sm.tsdbOutOfOrderSamplesAppended, "prometheus_tsdb_head_out_of_order_samples_appended_total", "type")
	data.SendSumOfCounters(out, sm.tsdbSnapshotReplayErrorTotal, "prometheus_tsdb_snapshot_replay_error_total")
	data.SendSumOfHistograms(out, sm.tsdbOOOHistogram, "prometheus_tsdb_sample_ooo_delta")
	data.SendSumOfCounters(out, sm.tsdbMmapChunksTotal, "prometheus_tsdb_mmap_chunks_total")
	data.SendSumOfCounters(out, sm.checkpointDeleteFail, "prometheus_tsdb_checkpoint_deletions_failed_total")
	data.SendSumOfCounters(out, sm.checkpointDeleteTotal, "prometheus_tsdb_checkpoint_deletions_total")
	data.SendSumOfCounters(out, sm.checkpointCreationFail, "prometheus_tsdb_checkpoint_creations_failed_total")
	data.SendSumOfCounters(out, sm.checkpointCreationTotal, "prometheus_tsdb_checkpoint_creations_total")
	data.SendSumOfCounters(out, sm.tsdbExemplarsTotal, "prometheus_tsdb_exemplar_exemplars_appended_total")
	data.SendSumOfGauges(out, sm.tsdbExemplarsInStorage, "prometheus_tsdb_exemplar_exemplars_in_storage")
	data.SendSumOfGaugesPerUser(out, sm.tsdbExemplarSeriesInStorage, "prometheus_tsdb_exemplar_series_with_exemplars_in_storage")
	data.SendSumOfGaugesPerUser(out, sm.tsdbExemplarLastTs, "prometheus_tsdb_exemplar_last_exemplars_timestamp_seconds")
	data.SendSumOfCounters(out, sm.tsdbExemplarsOutOfOrder, "prometheus_tsdb_exemplar_out_of_order_exemplars_total")

	data.SendSumOfCountersPerUser(out, sm.memSeriesCreatedTotal, "prometheus_tsdb_head_series_created_total")
	data.SendSumOfCountersPerUser(out, sm.memSeriesRemovedTotal, "prometheus_tsdb_head_series_removed_total")
}

func (sm *tsdbMetrics) setRegistryForUser(userID string, registry *prometheus.Registry) {
	sm.regs.AddUserRegistry(userID, registry)
}

func (sm *tsdbMetrics) removeRegistryForUser(userID string) {
	sm.regs.RemoveUserRegistry(userID, false)
}
