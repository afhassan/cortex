package querier

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/thanos-io/thanos/pkg/strutil"
	"golang.org/x/sync/errgroup"

	"github.com/cortexproject/cortex/pkg/engine"
	"github.com/cortexproject/cortex/pkg/querier/batch"
	"github.com/cortexproject/cortex/pkg/querier/lazyquery"
	"github.com/cortexproject/cortex/pkg/querier/partialdata"
	querier_stats "github.com/cortexproject/cortex/pkg/querier/stats"
	"github.com/cortexproject/cortex/pkg/tenant"
	"github.com/cortexproject/cortex/pkg/util"
	"github.com/cortexproject/cortex/pkg/util/flagext"
	"github.com/cortexproject/cortex/pkg/util/limiter"
	util_log "github.com/cortexproject/cortex/pkg/util/log"
	"github.com/cortexproject/cortex/pkg/util/spanlogger"
	"github.com/cortexproject/cortex/pkg/util/validation"
)

// Config contains the configuration require to create a querier
type Config struct {
	MaxConcurrent                  int           `yaml:"max_concurrent"`
	Timeout                        time.Duration `yaml:"timeout"`
	IngesterStreaming              bool          `yaml:"ingester_streaming" doc:"hidden"`
	IngesterMetadataStreaming      bool          `yaml:"ingester_metadata_streaming"`
	IngesterLabelNamesWithMatchers bool          `yaml:"ingester_label_names_with_matchers"`
	MaxSamples                     int           `yaml:"max_samples"`
	QueryIngestersWithin           time.Duration `yaml:"query_ingesters_within"`
	EnablePerStepStats             bool          `yaml:"per_step_stats_enabled"`

	// Use compression for metrics query API or instant and range query APIs.
	ResponseCompression string `yaml:"response_compression"`

	// QueryStoreAfter the time after which queries should also be sent to the store and not just ingesters.
	QueryStoreAfter    time.Duration `yaml:"query_store_after"`
	MaxQueryIntoFuture time.Duration `yaml:"max_query_into_future"`

	// The default evaluation interval for the promql engine.
	// Needs to be configured for subqueries to work as it is the default
	// step if not specified.
	DefaultEvaluationInterval time.Duration `yaml:"default_evaluation_interval"`

	// Limit of number of steps allowed for every subquery expression in a query.
	MaxSubQuerySteps int64 `yaml:"max_subquery_steps"`

	// Directory for ActiveQueryTracker. If empty, ActiveQueryTracker will be disabled and MaxConcurrent will not be applied (!).
	// ActiveQueryTracker logs queries that were active during the last crash, but logs them on the next startup.
	// However, we need to use active query tracker, otherwise we cannot limit Max Concurrent queries in the PromQL
	// engine.
	ActiveQueryTrackerDir string `yaml:"active_query_tracker_dir"`
	// LookbackDelta determines the time since the last sample after which a time
	// series is considered stale.
	LookbackDelta time.Duration `yaml:"lookback_delta"`

	// Blocks storage only.
	StoreGatewayAddresses         string       `yaml:"store_gateway_addresses"`
	StoreGatewayClient            ClientConfig `yaml:"store_gateway_client"`
	StoreGatewayQueryStatsEnabled bool         `yaml:"store_gateway_query_stats"`

	// The maximum number of times we attempt fetching missing blocks from different Store Gateways.
	StoreGatewayConsistencyCheckMaxAttempts int `yaml:"store_gateway_consistency_check_max_attempts"`

	// The maximum number of times we attempt fetching data from Ingesters.
	IngesterQueryMaxAttempts int `yaml:"ingester_query_max_attempts"`

	ShuffleShardingIngestersLookbackPeriod time.Duration `yaml:"shuffle_sharding_ingesters_lookback_period"`

	ThanosEngine engine.ThanosEngineConfig `yaml:"thanos_engine"`

	// Ignore max query length check at Querier.
	IgnoreMaxQueryLength              bool `yaml:"ignore_max_query_length"`
	EnablePromQLExperimentalFunctions bool `yaml:"enable_promql_experimental_functions"`

	// Query Parquet files if available
	EnableParquetQueryable            bool   `yaml:"enable_parquet_queryable"`
	ParquetQueryableShardCacheSize    int    `yaml:"parquet_queryable_shard_cache_size"`
	ParquetQueryableDefaultBlockStore string `yaml:"parquet_queryable_default_block_store"`
	ParquetQueryableFallbackDisabled  bool   `yaml:"parquet_queryable_fallback_disabled"`
}

var (
	errBadLookbackConfigs                             = errors.New("bad settings, query_store_after >= query_ingesters_within which can result in queries not being sent")
	errShuffleShardingLookbackLessThanQueryStoreAfter = errors.New("the shuffle-sharding lookback period should be greater or equal than the configured 'query store after'")
	errEmptyTimeRange                                 = errors.New("empty time range")
	errUnsupportedResponseCompression                 = errors.New("unsupported response compression. Supported compression 'gzip', 'snappy', 'zstd' and '' (disable compression)")
	errInvalidConsistencyCheckAttempts                = errors.New("store gateway consistency check max attempts should be greater or equal than 1")
	errInvalidIngesterQueryMaxAttempts                = errors.New("ingester query max attempts should be greater or equal than 1")
	errInvalidParquetQueryableDefaultBlockStore       = errors.New("unsupported parquet queryable default block store. Supported options are tsdb and parquet")
)

// RegisterFlags adds the flags required to config this to the given FlagSet.
func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	cfg.ThanosEngine.RegisterFlagsWithPrefix("querier.", f)

	//lint:ignore faillint Need to pass the global logger like this for warning on deprecated methods
	flagext.DeprecatedFlag(f, "querier.ingester-streaming", "Deprecated: Use streaming RPCs to query ingester. QueryStream is always enabled and the flag is not effective anymore.", util_log.Logger)
	//lint:ignore faillint Need to pass the global logger like this for warning on deprecated methods
	flagext.DeprecatedFlag(f, "querier.iterators", "Deprecated: Use iterators to execute query. This flag is no longer functional; Batch iterator is always enabled instead.", util_log.Logger)
	//lint:ignore faillint Need to pass the global logger like this for warning on deprecated methods
	flagext.DeprecatedFlag(f, "querier.batch-iterators", "Deprecated: Use batch iterators to execute query. This flag is no longer functional; Batch iterator is always enabled now.", util_log.Logger)
	//lint:ignore faillint Need to pass the global logger like this for warning on deprecated methods
	flagext.DeprecatedFlag(f, "querier.query-store-for-labels-enabled", "Deprecated: Querying long-term store is always enabled.", util_log.Logger)

	cfg.StoreGatewayClient.RegisterFlagsWithPrefix("querier.store-gateway-client", f)
	f.IntVar(&cfg.MaxConcurrent, "querier.max-concurrent", 20, "The maximum number of concurrent queries.")
	f.DurationVar(&cfg.Timeout, "querier.timeout", 2*time.Minute, "The timeout for a query.")
	f.BoolVar(&cfg.IngesterMetadataStreaming, "querier.ingester-metadata-streaming", true, "Deprecated (This feature will be always on after v1.18): Use streaming RPCs for metadata APIs from ingester.")
	f.BoolVar(&cfg.IngesterLabelNamesWithMatchers, "querier.ingester-label-names-with-matchers", false, "Use LabelNames ingester RPCs with match params.")
	f.IntVar(&cfg.MaxSamples, "querier.max-samples", 50e6, "Maximum number of samples a single query can load into memory.")
	f.DurationVar(&cfg.QueryIngestersWithin, "querier.query-ingesters-within", 0, "Maximum lookback beyond which queries are not sent to ingester. 0 means all queries are sent to ingester.")
	f.BoolVar(&cfg.EnablePerStepStats, "querier.per-step-stats-enabled", false, "Enable returning samples stats per steps in query response.")
	f.StringVar(&cfg.ResponseCompression, "querier.response-compression", "gzip", "Use compression for metrics query API or instant and range query APIs. Supported compression 'gzip', 'snappy', 'zstd' and '' (disable compression)")
	f.DurationVar(&cfg.MaxQueryIntoFuture, "querier.max-query-into-future", 10*time.Minute, "Maximum duration into the future you can query. 0 to disable.")
	f.DurationVar(&cfg.DefaultEvaluationInterval, "querier.default-evaluation-interval", time.Minute, "The default evaluation interval or step size for subqueries.")
	f.DurationVar(&cfg.QueryStoreAfter, "querier.query-store-after", 0, "The time after which a metric should be queried from storage and not just ingesters. 0 means all queries are sent to store. When running the blocks storage, if this option is enabled, the time range of the query sent to the store will be manipulated to ensure the query end is not more recent than 'now - query-store-after'.")
	f.StringVar(&cfg.ActiveQueryTrackerDir, "querier.active-query-tracker-dir", "./active-query-tracker", "Active query tracker monitors active queries, and writes them to the file in given directory. If Cortex discovers any queries in this log during startup, it will log them to the log file. Setting to empty value disables active query tracker, which also disables -querier.max-concurrent option.")
	f.StringVar(&cfg.StoreGatewayAddresses, "querier.store-gateway-addresses", "", "Comma separated list of store-gateway addresses in DNS Service Discovery format. This option should be set when using the blocks storage and the store-gateway sharding is disabled (when enabled, the store-gateway instances form a ring and addresses are picked from the ring).")
	f.BoolVar(&cfg.StoreGatewayQueryStatsEnabled, "querier.store-gateway-query-stats-enabled", true, "If enabled, store gateway query stats will be logged using `info` log level.")
	f.IntVar(&cfg.StoreGatewayConsistencyCheckMaxAttempts, "querier.store-gateway-consistency-check-max-attempts", maxFetchSeriesAttempts, "The maximum number of times we attempt fetching missing blocks from different store-gateways. If no more store-gateways are left (ie. due to lower replication factor) than we'll end the retries earlier")
	f.IntVar(&cfg.IngesterQueryMaxAttempts, "querier.ingester-query-max-attempts", 1, "The maximum number of times we attempt fetching data from ingesters for retryable errors (ex. partial data returned).")
	f.DurationVar(&cfg.LookbackDelta, "querier.lookback-delta", 5*time.Minute, "Time since the last sample after which a time series is considered stale and ignored by expression evaluations.")
	f.DurationVar(&cfg.ShuffleShardingIngestersLookbackPeriod, "querier.shuffle-sharding-ingesters-lookback-period", 0, "When distributor's sharding strategy is shuffle-sharding and this setting is > 0, queriers fetch in-memory series from the minimum set of required ingesters, selecting only ingesters which may have received series since 'now - lookback period'. The lookback period should be greater or equal than the configured 'query store after' and 'query ingesters within'. If this setting is 0, queriers always query all ingesters (ingesters shuffle sharding on read path is disabled).")
	f.Int64Var(&cfg.MaxSubQuerySteps, "querier.max-subquery-steps", 0, "Max number of steps allowed for every subquery expression in query. Number of steps is calculated using subquery range / step. A value > 0 enables it.")
	f.BoolVar(&cfg.IgnoreMaxQueryLength, "querier.ignore-max-query-length", false, "If enabled, ignore max query length check at Querier select method. Users can choose to ignore it since the validation can be done before Querier evaluation like at Query Frontend or Ruler.")
	f.BoolVar(&cfg.EnablePromQLExperimentalFunctions, "querier.enable-promql-experimental-functions", false, "[Experimental] If true, experimental promQL functions are enabled.")
	f.BoolVar(&cfg.EnableParquetQueryable, "querier.enable-parquet-queryable", false, "[Experimental] If true, querier will try to query the parquet files if available.")
	f.IntVar(&cfg.ParquetQueryableShardCacheSize, "querier.parquet-queryable-shard-cache-size", 512, "[Experimental] Maximum size of the Parquet queryable shard cache. 0 to disable.")
	f.StringVar(&cfg.ParquetQueryableDefaultBlockStore, "querier.parquet-queryable-default-block-store", string(parquetBlockStore), "[Experimental] Parquet queryable's default block store to query. Valid options are tsdb and parquet. If it is set to tsdb, parquet queryable always fallback to store gateway.")
	f.BoolVar(&cfg.ParquetQueryableFallbackDisabled, "querier.parquet-queryable-fallback-disabled", false, "[Experimental] Disable Parquet queryable to fallback queries to Store Gateway if the block is not available as Parquet files but available in TSDB. Setting this to true will disable the fallback and users can remove Store Gateway. But need to make sure Parquet files are created before it is queryable.")
}

// Validate the config
func (cfg *Config) Validate() error {
	// Ensure the config won't create a situation where no queriers are returned.
	if cfg.QueryIngestersWithin != 0 && cfg.QueryStoreAfter != 0 {
		if cfg.QueryStoreAfter >= cfg.QueryIngestersWithin {
			return errBadLookbackConfigs
		}
	}

	if cfg.ResponseCompression != "" && cfg.ResponseCompression != "gzip" && cfg.ResponseCompression != "snappy" && cfg.ResponseCompression != "zstd" {
		return errUnsupportedResponseCompression
	}

	if cfg.ShuffleShardingIngestersLookbackPeriod > 0 {
		if cfg.ShuffleShardingIngestersLookbackPeriod < cfg.QueryStoreAfter {
			return errShuffleShardingLookbackLessThanQueryStoreAfter
		}
	}

	if cfg.StoreGatewayConsistencyCheckMaxAttempts < 1 {
		return errInvalidConsistencyCheckAttempts
	}

	if cfg.IngesterQueryMaxAttempts < 1 {
		return errInvalidIngesterQueryMaxAttempts
	}

	if cfg.EnableParquetQueryable {
		if !slices.Contains(validBlockStoreTypes, blockStoreType(cfg.ParquetQueryableDefaultBlockStore)) {
			return errInvalidParquetQueryableDefaultBlockStore
		}
	}

	if err := cfg.ThanosEngine.Validate(); err != nil {
		return err
	}

	return nil
}

func (cfg *Config) GetStoreGatewayAddresses() []string {
	if cfg.StoreGatewayAddresses == "" {
		return nil
	}

	return strings.Split(cfg.StoreGatewayAddresses, ",")
}

func getChunksIteratorFunction(_ Config) chunkIteratorFunc {
	return batch.NewChunkMergeIterator
}

// New builds a queryable and promql engine.
func New(cfg Config, limits *validation.Overrides, distributor Distributor, stores []QueryableWithFilter, reg prometheus.Registerer, logger log.Logger, isPartialDataEnabled partialdata.IsCfgEnabledFunc) (storage.SampleAndChunkQueryable, storage.ExemplarQueryable, promql.QueryEngine) {
	iteratorFunc := getChunksIteratorFunction(cfg)

	distributorQueryable := newDistributorQueryable(distributor, cfg.IngesterMetadataStreaming, cfg.IngesterLabelNamesWithMatchers, iteratorFunc, cfg.QueryIngestersWithin, isPartialDataEnabled, cfg.IngesterQueryMaxAttempts)

	ns := make([]QueryableWithFilter, len(stores))
	for ix, s := range stores {
		ns[ix] = storeQueryable{
			QueryableWithFilter: s,
			QueryStoreAfter:     cfg.QueryStoreAfter,
		}
	}
	queryable := NewQueryable(distributorQueryable, ns, cfg, limits)
	exemplarQueryable := newDistributorExemplarQueryable(distributor)

	lazyQueryable := storage.QueryableFunc(func(mint int64, maxt int64) (storage.Querier, error) {
		querier, err := queryable.Querier(mint, maxt)
		if err != nil {
			return nil, err
		}
		return lazyquery.NewLazyQuerier(querier), nil
	})

	// Emit max_concurrent config as a metric.
	maxConcurrentMetric := promauto.With(reg).NewGauge(prometheus.GaugeOpts{
		Namespace: "cortex",
		Name:      "max_concurrent_queries",
		Help:      "The maximum number of concurrent queries.",
	})
	maxConcurrentMetric.Set(float64(cfg.MaxConcurrent))

	opts := promql.EngineOpts{
		Logger:               util_log.GoKitLogToSlog(logger),
		Reg:                  reg,
		ActiveQueryTracker:   createActiveQueryTracker(cfg, logger),
		MaxSamples:           cfg.MaxSamples,
		Timeout:              cfg.Timeout,
		LookbackDelta:        cfg.LookbackDelta,
		EnablePerStepStats:   cfg.EnablePerStepStats,
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
		NoStepSubqueryIntervalFn: func(int64) int64 {
			return cfg.DefaultEvaluationInterval.Milliseconds()
		},
	}
	queryEngine := engine.New(opts, cfg.ThanosEngine, reg)
	return NewSampleAndChunkQueryable(lazyQueryable), exemplarQueryable, queryEngine
}

// NewSampleAndChunkQueryable creates a SampleAndChunkQueryable from a
// Queryable with a ChunkQueryable stub, that errors once it gets called.
func NewSampleAndChunkQueryable(q storage.Queryable) storage.SampleAndChunkQueryable {
	return &sampleAndChunkQueryable{q}
}

type sampleAndChunkQueryable struct {
	storage.Queryable
}

func (q *sampleAndChunkQueryable) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	return nil, errors.New("ChunkQuerier not implemented")
}

func createActiveQueryTracker(cfg Config, logger log.Logger) promql.QueryTracker {
	dir := cfg.ActiveQueryTrackerDir

	if dir != "" {
		return promql.NewActiveQueryTracker(dir, cfg.MaxConcurrent, util_log.GoKitLogToSlog(logger))
	}

	return nil
}

// QueryableWithFilter extends Queryable interface with `UseQueryable` filtering function.
type QueryableWithFilter interface {
	storage.Queryable

	// UseQueryable returns true if this queryable should be used to satisfy the query for given time range.
	// Query min and max time are in milliseconds since epoch.
	UseQueryable(now time.Time, queryMinT, queryMaxT int64) bool
}

type limiterHolder struct {
	limiter            *limiter.QueryLimiter
	limiterInitializer sync.Once
}

// NewQueryable creates a new Queryable for cortex.
func NewQueryable(distributor QueryableWithFilter, stores []QueryableWithFilter, cfg Config, limits *validation.Overrides) storage.Queryable {
	return storage.QueryableFunc(func(mint, maxt int64) (storage.Querier, error) {
		q := querier{
			now:                  time.Now(),
			mint:                 mint,
			maxt:                 maxt,
			limits:               limits,
			maxQueryIntoFuture:   cfg.MaxQueryIntoFuture,
			ignoreMaxQueryLength: cfg.IgnoreMaxQueryLength,
			distributor:          distributor,
			stores:               stores,
			limiterHolder:        &limiterHolder{},
		}

		return q, nil
	})
}

type querier struct {
	now                time.Time
	mint, maxt         int64
	limits             *validation.Overrides
	maxQueryIntoFuture time.Duration
	distributor        QueryableWithFilter
	stores             []QueryableWithFilter
	limiterHolder      *limiterHolder

	ignoreMaxQueryLength bool
}

func (q querier) setupFromCtx(ctx context.Context) (context.Context, *querier_stats.QueryStats, string, int64, int64, storage.Querier, []storage.Querier, error) {
	stats := querier_stats.FromContext(ctx)
	userID, err := tenant.TenantID(ctx)
	if err != nil {
		return ctx, stats, userID, 0, 0, nil, nil, err
	}

	q.limiterHolder.limiterInitializer.Do(func() {
		q.limiterHolder.limiter = limiter.NewQueryLimiter(q.limits.MaxFetchedSeriesPerQuery(userID), q.limits.MaxFetchedChunkBytesPerQuery(userID), q.limits.MaxChunksPerQuery(userID), q.limits.MaxFetchedDataBytesPerQuery(userID))
	})

	ctx = limiter.AddQueryLimiterToContext(ctx, q.limiterHolder.limiter)

	mint, maxt, err := validateQueryTimeRange(ctx, userID, q.mint, q.maxt, q.limits, q.maxQueryIntoFuture)
	if err != nil {
		return ctx, stats, userID, 0, 0, nil, nil, err
	}

	dqr, err := q.distributor.Querier(mint, maxt)
	if err != nil {
		return ctx, stats, userID, 0, 0, nil, nil, err
	}
	metadataQuerier := dqr

	queriers := make([]storage.Querier, 0)
	if q.distributor.UseQueryable(q.now, mint, maxt) {
		queriers = append(queriers, dqr)
	}

	for _, s := range q.stores {
		if !s.UseQueryable(q.now, mint, maxt) {
			continue
		}

		cqr, err := s.Querier(mint, maxt)
		if err != nil {
			return ctx, stats, userID, 0, 0, nil, nil, err
		}

		queriers = append(queriers, cqr)
	}
	return ctx, stats, userID, mint, maxt, metadataQuerier, queriers, nil
}

// Select implements storage.Querier interface.
// The bool passed is ignored because the series is always sorted.
func (q querier) Select(ctx context.Context, sortSeries bool, sp *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	ctx, stats, userID, mint, maxt, metadataQuerier, queriers, err := q.setupFromCtx(ctx)
	if err == errEmptyTimeRange {
		return storage.EmptySeriesSet()
	} else if err != nil {
		return storage.ErrSeriesSet(err)
	}
	startT := time.Now()
	defer func() {
		stats.AddQueryStorageWallTime(time.Since(startT))
	}()

	log, ctx := spanlogger.New(ctx, "querier.Select")
	defer log.Finish()

	if sp != nil {
		level.Debug(log).Log("start", util.TimeFromMillis(sp.Start).UTC().String(), "end", util.TimeFromMillis(sp.End).UTC().String(), "step", sp.Step, "matchers", matchers)
	}

	if sp == nil {
		mint, maxt, err = validateQueryTimeRange(ctx, userID, mint, maxt, q.limits, q.maxQueryIntoFuture)
		if err == errEmptyTimeRange {
			return storage.EmptySeriesSet()
		} else if err != nil {
			return storage.ErrSeriesSet(err)
		}
		// if SelectHints is null, rely on minT, maxT of querier to scope in range for Select stmt
		sp = &storage.SelectHints{Start: mint, End: maxt}
	}

	// Validate query time range. Even if the time range has already been validated when we created
	// the querier, we need to check it again here because the time range specified in hints may be
	// different.
	startMs, endMs, err := validateQueryTimeRange(ctx, userID, sp.Start, sp.End, q.limits, q.maxQueryIntoFuture)
	if err == errEmptyTimeRange {
		return storage.NoopSeriesSet()
	} else if err != nil {
		return storage.ErrSeriesSet(err)
	}

	// The time range may have been manipulated during the validation,
	// so we make sure changes are reflected back to hints.
	sp.Start = startMs
	sp.End = endMs
	getSeries := sp.Func == "series"

	// For series queries without specifying the start time, we prefer to
	// only query ingesters and not to query maxQueryLength to avoid OOM kill.
	if getSeries && startMs == 0 {
		return metadataQuerier.Select(ctx, true, sp, matchers...)
	}

	startTime := model.Time(startMs)
	endTime := model.Time(endMs)

	// Validate query time range. This validation for instant / range queries can be done either at Query Frontend
	// or here at Querier. When the check is done at Query Frontend, we still want to enforce the max query length
	// check for /api/v1/series request since there is no specific tripperware for series.
	if !q.ignoreMaxQueryLength || getSeries {
		if maxQueryLength := q.limits.MaxQueryLength(userID); maxQueryLength > 0 && endTime.Sub(startTime) > maxQueryLength {
			limitErr := validation.LimitError(fmt.Sprintf(validation.ErrQueryTooLong, endTime.Sub(startTime), maxQueryLength))
			return storage.ErrSeriesSet(limitErr)
		}
	}

	if len(queriers) == 1 {
		return queriers[0].Select(ctx, sortSeries, sp, matchers...)
	}

	sets := make(chan storage.SeriesSet, len(queriers))
	for _, querier := range queriers {
		go func(querier storage.Querier) {
			// We should always select sorted here as we will need to merge the series
			sets <- querier.Select(ctx, true, sp, matchers...)
		}(querier)
	}

	var result []storage.SeriesSet
	for range queriers {
		select {
		case set := <-sets:
			result = append(result, set)
		case <-ctx.Done():
			return storage.ErrSeriesSet(ctx.Err())
		}
	}

	return storage.NewMergeSeriesSet(result, 0, storage.ChainedSeriesMerge)
}

// LabelValues implements storage.Querier.
func (q querier) LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	ctx, stats, userID, mint, maxt, metadataQuerier, queriers, err := q.setupFromCtx(ctx)
	if err == errEmptyTimeRange {
		return nil, nil, nil
	} else if err != nil {
		return nil, nil, err
	}
	startT := time.Now()
	defer func() {
		stats.AddQueryStorageWallTime(time.Since(startT))
	}()

	// For label values queries without specifying the start time, we prefer to
	// only query ingesters and not to query maxQueryLength to avoid OOM kill.
	if mint == 0 {
		return metadataQuerier.LabelValues(ctx, name, hints, matchers...)
	}

	startTime := model.Time(mint)
	endTime := model.Time(maxt)

	if maxQueryLength := q.limits.MaxQueryLength(userID); maxQueryLength > 0 && endTime.Sub(startTime) > maxQueryLength {
		limitErr := validation.LimitError(fmt.Sprintf(validation.ErrQueryTooLong, endTime.Sub(startTime), maxQueryLength))
		return nil, nil, limitErr
	}

	if len(queriers) == 1 {
		return queriers[0].LabelValues(ctx, name, hints, matchers...)
	}

	var (
		g, _     = errgroup.WithContext(ctx)
		sets     = [][]string{}
		warnings = annotations.Annotations(nil)

		resMtx sync.Mutex
	)

	for _, querier := range queriers {
		// Need to reassign as the original variable will change and can't be relied on in a goroutine.
		querier := querier
		g.Go(func() error {
			// NB: Values are sorted in Cortex already.
			myValues, myWarnings, err := querier.LabelValues(ctx, name, hints, matchers...)
			if err != nil {
				return err
			}

			resMtx.Lock()
			sets = append(sets, myValues)
			warnings.Merge(myWarnings)
			resMtx.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, nil, err
	}

	limit := 0
	if hints != nil {
		limit = hints.Limit
	}

	return strutil.MergeSlices(limit, sets...), warnings, nil
}

func (q querier) LabelNames(ctx context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	ctx, stats, userID, mint, maxt, metadataQuerier, queriers, err := q.setupFromCtx(ctx)
	if err == errEmptyTimeRange {
		return nil, nil, nil
	} else if err != nil {
		return nil, nil, err
	}
	startT := time.Now()
	defer func() {
		stats.AddQueryStorageWallTime(time.Since(startT))
	}()

	// For label names queries without specifying the start time, we prefer to
	// only query ingesters and not to query maxQueryLength to avoid OOM kill.
	if mint == 0 {
		return metadataQuerier.LabelNames(ctx, hints, matchers...)
	}

	startTime := model.Time(mint)
	endTime := model.Time(maxt)

	if maxQueryLength := q.limits.MaxQueryLength(userID); maxQueryLength > 0 && endTime.Sub(startTime) > maxQueryLength {
		limitErr := validation.LimitError(fmt.Sprintf(validation.ErrQueryTooLong, endTime.Sub(startTime), maxQueryLength))
		return nil, nil, limitErr
	}

	if len(queriers) == 1 {
		return queriers[0].LabelNames(ctx, hints, matchers...)
	}

	var (
		g, _     = errgroup.WithContext(ctx)
		sets     = [][]string{}
		warnings = annotations.Annotations(nil)

		resMtx sync.Mutex
	)

	for _, querier := range queriers {
		// Need to reassign as the original variable will change and can't be relied on in a goroutine.
		querier := querier
		g.Go(func() error {
			// NB: Names are sorted in Cortex already.
			myNames, myWarnings, err := querier.LabelNames(ctx, hints, matchers...)
			if err != nil {
				return err
			}

			resMtx.Lock()
			sets = append(sets, myNames)
			warnings.Merge(myWarnings)
			resMtx.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, nil, err
	}

	limit := 0
	if hints != nil {
		limit = hints.Limit
	}

	return strutil.MergeSlices(limit, sets...), warnings, nil
}

func (querier) Close() error {
	return nil
}

type storeQueryable struct {
	QueryableWithFilter
	QueryStoreAfter time.Duration
}

func (s storeQueryable) UseQueryable(now time.Time, queryMinT, queryMaxT int64) bool {
	// Include this store only if mint is within QueryStoreAfter w.r.t current time.
	if s.QueryStoreAfter != 0 && queryMinT > util.TimeToMillis(now.Add(-s.QueryStoreAfter)) {
		return false
	}
	return s.QueryableWithFilter.UseQueryable(now, queryMinT, queryMaxT)
}

type alwaysTrueFilterQueryable struct {
	storage.Queryable
}

func (alwaysTrueFilterQueryable) UseQueryable(_ time.Time, _, _ int64) bool {
	return true
}

// Wraps storage.Queryable into QueryableWithFilter, with no query filtering.
func UseAlwaysQueryable(q storage.Queryable) QueryableWithFilter {
	return alwaysTrueFilterQueryable{Queryable: q}
}

type useBeforeTimestampQueryable struct {
	storage.Queryable
	ts int64 // Timestamp in milliseconds
}

func (u useBeforeTimestampQueryable) UseQueryable(_ time.Time, queryMinT, _ int64) bool {
	if u.ts == 0 {
		return true
	}
	return queryMinT < u.ts
}

// Returns QueryableWithFilter, that is used only if query starts before given timestamp.
// If timestamp is zero (time.IsZero), queryable is always used.
func UseBeforeTimestampQueryable(queryable storage.Queryable, ts time.Time) QueryableWithFilter {
	t := int64(0)
	if !ts.IsZero() {
		t = util.TimeToMillis(ts)
	}
	return useBeforeTimestampQueryable{
		Queryable: queryable,
		ts:        t,
	}
}

func validateQueryTimeRange(ctx context.Context, userID string, startMs, endMs int64, limits *validation.Overrides, maxQueryIntoFuture time.Duration) (int64, int64, error) {
	now := model.Now()
	startTime := model.Time(startMs)
	endTime := model.Time(endMs)

	// Clamp time range based on max query into future.
	if maxQueryIntoFuture > 0 && endTime.After(now.Add(maxQueryIntoFuture)) {
		origEndTime := endTime
		endTime = now.Add(maxQueryIntoFuture)

		// Make sure to log it in traces to ease debugging.
		level.Debug(spanlogger.FromContext(ctx)).Log(
			"msg", "the end time of the query has been manipulated because of the 'max query into future' setting",
			"original", util.FormatTimeModel(origEndTime),
			"updated", util.FormatTimeModel(endTime))

		if endTime.Before(startTime) {
			return 0, 0, errEmptyTimeRange
		}
	}

	// Clamp the time range based on the max query lookback.
	if maxQueryLookback := limits.MaxQueryLookback(userID); maxQueryLookback > 0 && startTime.Before(now.Add(-maxQueryLookback)) {
		origStartTime := startTime
		startTime = now.Add(-maxQueryLookback)

		// Make sure to log it in traces to ease debugging.
		level.Debug(spanlogger.FromContext(ctx)).Log(
			"msg", "the start time of the query has been manipulated because of the 'max query lookback' setting",
			"original", util.FormatTimeModel(origStartTime),
			"updated", util.FormatTimeModel(startTime))

		if endTime.Before(startTime) {
			return 0, 0, errEmptyTimeRange
		}
	}

	// start time should be at least non-negative to avoid int64 overflow.
	if startTime < 0 {
		startTime = 0
	}

	return int64(startTime), int64(endTime), nil
}

func EnableExperimentalPromQLFunctions(enablePromQLExperimentalFunctions, enableHoltWinters bool) {
	parser.EnableExperimentalFunctions = enablePromQLExperimentalFunctions

	if enableHoltWinters {
		holtWinters := *parser.Functions["double_exponential_smoothing"]
		holtWinters.Experimental = false
		holtWinters.Name = "holt_winters"
		parser.Functions["holt_winters"] = &holtWinters
		promql.FunctionCalls["holt_winters"] = promql.FunctionCalls["double_exponential_smoothing"]
	}
}
