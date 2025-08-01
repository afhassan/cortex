package queryrange

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/uber/jaeger-client-go"
	"github.com/weaveworks/common/httpgrpc"

	"github.com/cortexproject/cortex/pkg/chunk/cache"
	"github.com/cortexproject/cortex/pkg/cortexpb"
	cortexparser "github.com/cortexproject/cortex/pkg/parser"
	"github.com/cortexproject/cortex/pkg/querier"
	"github.com/cortexproject/cortex/pkg/querier/partialdata"
	querier_stats "github.com/cortexproject/cortex/pkg/querier/stats"
	"github.com/cortexproject/cortex/pkg/querier/tripperware"
	"github.com/cortexproject/cortex/pkg/tenant"
	"github.com/cortexproject/cortex/pkg/util/flagext"
	util_log "github.com/cortexproject/cortex/pkg/util/log"
	"github.com/cortexproject/cortex/pkg/util/spanlogger"
	"github.com/cortexproject/cortex/pkg/util/validation"
)

var (
	// Value that cacheControlHeader has if the response indicates that the results should not be cached.
	noStoreValue = "no-store"
)

// ResultsCacheConfig is the config for the results cache.
type ResultsCacheConfig struct {
	CacheConfig                cache.Config `yaml:"cache"`
	Compression                string       `yaml:"compression"`
	CacheQueryableSamplesStats bool         `yaml:"cache_queryable_samples_stats"`
}

// RegisterFlags registers flags.
func (cfg *ResultsCacheConfig) RegisterFlags(f *flag.FlagSet) {
	cfg.CacheConfig.RegisterFlagsWithPrefix("frontend.", "", f)

	f.StringVar(&cfg.Compression, "frontend.compression", "", "Use compression in results cache. Supported values are: 'snappy' and '' (disable compression).")
	f.BoolVar(&cfg.CacheQueryableSamplesStats, "frontend.cache-queryable-samples-stats", false, "Cache Statistics queryable samples on results cache.")
	//lint:ignore faillint Need to pass the global logger like this for warning on deprecated methods
	flagext.DeprecatedFlag(f, "frontend.cache-split-interval", "Deprecated: The maximum interval expected for each request, results will be cached per single interval. This behavior is now determined by querier.split-queries-by-interval.", util_log.Logger)
}

func (cfg *ResultsCacheConfig) Validate(qCfg querier.Config) error {
	switch cfg.Compression {
	case "snappy", "":
		// valid
	default:
		return errors.Errorf("unsupported compression type: %s", cfg.Compression)
	}

	if cfg.CacheQueryableSamplesStats && !qCfg.EnablePerStepStats {
		return errors.New("frontend.cache-queryable-samples-stats may only be enabled in conjunction with querier.per-step-stats-enabled. Please set the latter")
	}

	return cfg.CacheConfig.Validate()
}

// Extractor is used by the cache to extract a subset of a response from a cache entry.
type Extractor interface {
	// Extract extracts a subset of a response from the `start` and `end` timestamps in milliseconds in the `from` response.
	Extract(start, end int64, from tripperware.Response) tripperware.Response
	ResponseWithoutHeaders(resp tripperware.Response) tripperware.Response
	ResponseWithoutStats(resp tripperware.Response) tripperware.Response
}

// PrometheusResponseExtractor helps extracting specific info from Query Response.
type PrometheusResponseExtractor struct{}

// Extract extracts response for specific a range from a response.
func (PrometheusResponseExtractor) Extract(start, end int64, from tripperware.Response) tripperware.Response {
	promRes := from.(*tripperware.PrometheusResponse)
	return &tripperware.PrometheusResponse{
		Status: StatusSuccess,
		Data: tripperware.PrometheusData{
			ResultType: promRes.Data.ResultType,
			Result: tripperware.PrometheusQueryResult{
				Result: &tripperware.PrometheusQueryResult_Matrix{
					Matrix: &tripperware.Matrix{
						SampleStreams: extractSampleStreams(start, end, promRes.Data.Result.GetMatrix().GetSampleStreams()),
					},
				},
			},
			Stats: extractStats(start, end, promRes.Data.Stats),
		},
		Headers:  promRes.Headers,
		Warnings: promRes.Warnings,
		Infos:    promRes.Infos,
	}
}

// ResponseWithoutHeaders is useful in caching data without headers since
// we anyways do not need headers for sending back the response so this saves some space by reducing size of the objects.
func (PrometheusResponseExtractor) ResponseWithoutHeaders(resp tripperware.Response) tripperware.Response {
	promRes := resp.(*tripperware.PrometheusResponse)
	return &tripperware.PrometheusResponse{
		Status: StatusSuccess,
		Data: tripperware.PrometheusData{
			ResultType: promRes.Data.ResultType,
			Result:     promRes.Data.Result,
			Stats:      promRes.Data.Stats,
		},
		Warnings: promRes.Warnings,
		Infos:    promRes.Infos,
	}
}

// ResponseWithoutStats is returns the response without the stats information
func (PrometheusResponseExtractor) ResponseWithoutStats(resp tripperware.Response) tripperware.Response {
	promRes := resp.(*tripperware.PrometheusResponse)
	return &tripperware.PrometheusResponse{
		Status: StatusSuccess,
		Data: tripperware.PrometheusData{
			ResultType: promRes.Data.ResultType,
			Result:     promRes.Data.Result,
		},
		Headers:  promRes.Headers,
		Warnings: promRes.Warnings,
		Infos:    promRes.Infos,
	}
}

// CacheSplitter generates cache keys. This is a useful interface for downstream
// consumers who wish to implement their own strategies.
type CacheSplitter interface {
	GenerateCacheKey(ctx context.Context, userID string, r tripperware.Request) string
}

// splitter is a utility for using split interval when determining cache keys
type splitter time.Duration

// GenerateCacheKey generates a cache key based on the userID, Request and interval.
func (t splitter) GenerateCacheKey(ctx context.Context, userID string, r tripperware.Request) string {
	stats := querier_stats.FromContext(ctx)
	interval := stats.LoadSplitInterval()
	if interval == 0 {
		interval = time.Duration(t)
	}

	currentInterval := r.GetStart() / int64(interval/time.Millisecond)
	return fmt.Sprintf("%s:%s:%d:%d", userID, r.GetQuery(), r.GetStep(), currentInterval)
}

// ShouldCacheFn checks whether the current request should go to cache
// or not. If not, just send the request to next handler.
type ShouldCacheFn func(r tripperware.Request) bool

type resultsCache struct {
	logger   log.Logger
	cfg      ResultsCacheConfig
	next     tripperware.Handler
	cache    cache.Cache
	limits   tripperware.Limits
	splitter CacheSplitter

	extractor                  Extractor
	minCacheExtent             int64 // discard any cache extent smaller than this
	merger                     tripperware.Merger
	shouldCache                ShouldCacheFn
	cacheQueryableSamplesStats bool
}

// NewResultsCacheMiddleware creates results cache middleware from config.
// The middleware cache result using a unique cache key for a given request (step,query,user) and interval.
// The cache assumes that each request length (end-start) is below or equal the interval.
// Each request starting from within the same interval will hit the same cache entry.
// If the cache doesn't have the entire duration of the request cached, it will query the uncached parts and append them to the cache entries.
// see `generateKey`.
func NewResultsCacheMiddleware(
	logger log.Logger,
	cfg ResultsCacheConfig,
	splitter CacheSplitter,
	limits tripperware.Limits,
	merger tripperware.Merger,
	extractor Extractor,
	shouldCache ShouldCacheFn,
	reg prometheus.Registerer,
) (tripperware.Middleware, cache.Cache, error) {
	c, err := cache.New(cfg.CacheConfig, reg, logger)
	if err != nil {
		return nil, nil, err
	}
	if cfg.Compression == "snappy" {
		c = cache.NewSnappy(c, logger)
	}

	return tripperware.MiddlewareFunc(func(next tripperware.Handler) tripperware.Handler {
		return &resultsCache{
			logger:                     logger,
			cfg:                        cfg,
			next:                       next,
			cache:                      c,
			limits:                     limits,
			merger:                     merger,
			extractor:                  extractor,
			minCacheExtent:             (5 * time.Minute).Milliseconds(),
			splitter:                   splitter,
			shouldCache:                shouldCache,
			cacheQueryableSamplesStats: cfg.CacheQueryableSamplesStats,
		}
	}), c, nil
}

func (s resultsCache) Do(ctx context.Context, r tripperware.Request) (tripperware.Response, error) {
	tenantIDs, err := tenant.TenantIDs(ctx)
	respWithStats := r.GetStats() != "" && s.cacheQueryableSamplesStats
	if err != nil {
		return nil, httpgrpc.Errorf(http.StatusBadRequest, "%s", err.Error())
	}

	// If cache_queryable_samples_stats is enabled we always need request the status upstream
	if s.cacheQueryableSamplesStats {
		r = r.WithStats("all")
	} else {
		r = r.WithStats("")
	}

	if s.shouldCache != nil && !s.shouldCache(r) {
		level.Debug(util_log.WithContext(ctx, s.logger)).Log("msg", "should not cache", "start", r.GetStart(), "spanID", jaegerSpanID(ctx))
		return s.next.Do(ctx, r)
	}

	key := s.splitter.GenerateCacheKey(ctx, tenant.JoinTenantIDs(tenantIDs), r)

	var (
		extents  []tripperware.Extent
		response tripperware.Response
	)

	maxCacheFreshness := validation.MaxDurationPerTenant(tenantIDs, s.limits.MaxCacheFreshness)
	maxCacheTime := int64(model.Now().Add(-maxCacheFreshness))
	if r.GetStart() > maxCacheTime {
		level.Debug(util_log.WithContext(ctx, s.logger)).Log("msg", "cache miss", "start", r.GetStart(), "spanID", jaegerSpanID(ctx))
		return s.next.Do(ctx, r)
	}

	cached, ok := s.get(ctx, key)
	if ok {
		response, extents, err = s.handleHit(ctx, r, cached, maxCacheTime)
	} else {
		response, extents, err = s.handleMiss(ctx, r, maxCacheTime)
	}

	if err == nil && len(extents) > 0 {
		extents, err := s.filterRecentExtents(r, maxCacheFreshness, extents)
		if err != nil {
			return nil, err
		}
		// Make sure we only cache old response format for backward compatibility.
		// TODO: expose a flag to switch to write new format.
		for i, ext := range extents {
			resp, err := extentToResponse(ext)
			if err != nil {
				return nil, err
			}
			// Convert response in extent to old format.
			resp = convertFromTripperwarePrometheusResponse(resp)
			any, err := types.MarshalAny(resp)
			if err != nil {
				return nil, err
			}
			extents[i].Response = any
		}
		s.put(ctx, key, extents)
	}

	if err == nil && !respWithStats {
		response = s.extractor.ResponseWithoutStats(response)
	}
	return response, err
}

// shouldCacheResponse says whether the response should be cached or not.
func (s resultsCache) shouldCacheResponse(ctx context.Context, req tripperware.Request, r tripperware.Response, maxCacheTime int64) bool {
	headerValues := getHeaderValuesWithName(r, cacheControlHeader)
	for _, v := range headerValues {
		if v == noStoreValue {
			level.Debug(util_log.WithContext(ctx, s.logger)).Log("msg", fmt.Sprintf("%s header in response is equal to %s, not caching the response", cacheControlHeader, noStoreValue))
			return false
		}
	}

	if !s.isAtModifierCachable(ctx, req, maxCacheTime) {
		return false
	}
	if !s.isOffsetCachable(ctx, req) {
		return false
	}
	if res, ok := r.(*tripperware.PrometheusResponse); ok {
		return !slices.Contains(res.Warnings, partialdata.ErrPartialData.Error())
	}

	return true
}

// isAtModifierCachable returns true if the @ modifier result
// is safe to cache.
func (s resultsCache) isAtModifierCachable(ctx context.Context, r tripperware.Request, maxCacheTime int64) bool {
	// There are 2 cases when @ modifier is not safe to cache:
	//   1. When @ modifier points to time beyond the maxCacheTime.
	//   2. If the @ modifier time is > the query range end while being
	//      below maxCacheTime. In such cases if any tenant is intentionally
	//      playing with old data, we could cache empty result if we look
	//      beyond query end.
	var errAtModifierAfterEnd = errors.New("at modifier after end")
	query := r.GetQuery()
	if !strings.Contains(query, "@") {
		return true
	}
	expr, err := cortexparser.ParseExpr(query)
	if err != nil {
		// We are being pessimistic in such cases.
		level.Warn(util_log.WithContext(ctx, s.logger)).Log("msg", "failed to parse query, considering @ modifier as not cacheable", "query", query, "err", err)
		return false
	}

	// This resolves the start() and end() used with the @ modifier.
	expr, err = promql.PreprocessExpr(expr, timestamp.Time(r.GetStart()), timestamp.Time(r.GetEnd()), time.Duration(r.GetStep())*time.Millisecond)
	if err != nil {
		// We are being pessimistic in such cases.
		level.Warn(util_log.WithContext(ctx, s.logger)).Log("msg", "failed to preprocess expr", "query", query, "err", err)
		return false
	}

	end := r.GetEnd()
	atModCachable := true
	parser.Inspect(expr, func(n parser.Node, _ []parser.Node) error {
		switch e := n.(type) {
		case *parser.VectorSelector:
			if e.Timestamp != nil && (*e.Timestamp > end || *e.Timestamp > maxCacheTime) {
				atModCachable = false
				return errAtModifierAfterEnd
			}
		case *parser.MatrixSelector:
			ts := e.VectorSelector.(*parser.VectorSelector).Timestamp
			if ts != nil && (*ts > end || *ts > maxCacheTime) {
				atModCachable = false
				return errAtModifierAfterEnd
			}
		case *parser.SubqueryExpr:
			if e.Timestamp != nil && (*e.Timestamp > end || *e.Timestamp > maxCacheTime) {
				atModCachable = false
				return errAtModifierAfterEnd
			}
		}
		return nil
	})

	return atModCachable
}

// isOffsetCachable returns true if the offset is positive, result is safe to cache.
// and false when offset is negative, result is not cached.
func (s resultsCache) isOffsetCachable(ctx context.Context, r tripperware.Request) bool {
	var errNegativeOffset = errors.New("negative offset")
	query := r.GetQuery()
	if !strings.Contains(query, "offset") {
		return true
	}
	expr, err := cortexparser.ParseExpr(query)
	if err != nil {
		level.Warn(util_log.WithContext(ctx, s.logger)).Log("msg", "failed to parse query, considering offset as not cacheable", "query", query, "err", err)
		return false
	}

	offsetCachable := true
	parser.Inspect(expr, func(n parser.Node, _ []parser.Node) error {
		switch e := n.(type) {
		case *parser.VectorSelector:
			if e.OriginalOffset < 0 {
				offsetCachable = false
				return errNegativeOffset
			}
		case *parser.MatrixSelector:
			offset := e.VectorSelector.(*parser.VectorSelector).OriginalOffset
			if offset < 0 {
				offsetCachable = false
				return errNegativeOffset
			}
		case *parser.SubqueryExpr:
			if e.OriginalOffset < 0 {
				offsetCachable = false
				return errNegativeOffset
			}
		}
		return nil
	})

	return offsetCachable
}

func getHeaderValuesWithName(r tripperware.Response, headerName string) (headerValues []string) {
	for name, hv := range r.HTTPHeaders() {
		if name != headerName {
			continue
		}

		headerValues = append(headerValues, hv...)
	}

	return
}

func (s resultsCache) handleMiss(ctx context.Context, r tripperware.Request, maxCacheTime int64) (tripperware.Response, []tripperware.Extent, error) {
	level.Debug(util_log.WithContext(ctx, s.logger)).Log("msg", "handle miss", "start", r.GetStart(), "spanID", jaegerSpanID(ctx))
	response, err := s.next.Do(ctx, r)
	if err != nil {
		return nil, nil, err
	}

	if !s.shouldCacheResponse(ctx, r, response, maxCacheTime) {
		return response, []tripperware.Extent{}, nil
	}

	extent, err := toExtent(ctx, r, s.extractor.ResponseWithoutHeaders(response))
	if err != nil {
		return nil, nil, err
	}

	extents := []tripperware.Extent{
		extent,
	}
	return response, extents, nil
}

func (s resultsCache) handleHit(ctx context.Context, r tripperware.Request, extents []tripperware.Extent, maxCacheTime int64) (tripperware.Response, []tripperware.Extent, error) {
	var (
		reqResps []tripperware.RequestResponse
		err      error
	)
	log, ctx := spanlogger.New(ctx, "handleHit")
	defer log.Finish()

	level.Debug(util_log.WithContext(ctx, log)).Log("msg", "handle hit", "start", r.GetStart(), "spanID", jaegerSpanID(ctx))

	requests, responses, err := s.partition(ctx, r, extents)
	if err != nil {
		return nil, nil, err
	}
	if len(requests) == 0 {
		response, err := s.merger.MergeResponse(ctx, r, responses...)
		// No downstream requests so no need to write back to the cache.
		return response, nil, err
	}

	reqResps, err = tripperware.DoRequests(ctx, s.next, requests, s.limits)
	if err != nil {
		return nil, nil, err
	}

	for _, reqResp := range reqResps {
		responses = append(responses, reqResp.Response)
		if !s.shouldCacheResponse(ctx, r, reqResp.Response, maxCacheTime) {
			continue
		}
		extent, err := toExtent(ctx, reqResp.Request, s.extractor.ResponseWithoutHeaders(reqResp.Response))
		if err != nil {
			return nil, nil, err
		}
		extents = append(extents, extent)
	}
	sort.Slice(extents, func(i, j int) bool {
		if extents[i].Start == extents[j].Start {
			// as an optimization, for two extents starts at the same time, we
			// put bigger extent at the front of the slice, which helps
			// to reduce the amount of merge we have to do later.
			return extents[i].End > extents[j].End
		}

		return extents[i].Start < extents[j].Start
	})

	// Merge any extents - potentially overlapping
	accumulator, err := newAccumulator(extents[0])
	if err != nil {
		return nil, nil, err
	}
	mergedExtents := make([]tripperware.Extent, 0, len(extents))

	for i := 1; i < len(extents); i++ {
		if accumulator.End+r.GetStep() < extents[i].Start {
			mergedExtents, err = merge(mergedExtents, accumulator)
			if err != nil {
				return nil, nil, err
			}
			accumulator, err = newAccumulator(extents[i])
			if err != nil {
				return nil, nil, err
			}
			continue
		}

		if accumulator.End >= extents[i].End {
			continue
		}

		accumulator.TraceId = jaegerTraceID(ctx)
		accumulator.End = extents[i].End
		currentRes, err := extentToResponse(extents[i])
		if err != nil {
			return nil, nil, err
		}
		merged, err := s.merger.MergeResponse(ctx, r, accumulator.Response, currentRes)
		if err != nil {
			return nil, nil, err
		}
		accumulator.Response = merged
	}

	mergedExtents, err = merge(mergedExtents, accumulator)
	if err != nil {
		return nil, nil, err
	}

	response, err := s.merger.MergeResponse(ctx, r, responses...)
	return response, mergedExtents, err
}

type accumulator struct {
	tripperware.Response
	tripperware.Extent
}

func merge(extents []tripperware.Extent, acc *accumulator) ([]tripperware.Extent, error) {
	any, err := types.MarshalAny(acc.Response)
	if err != nil {
		return nil, err
	}
	return append(extents, tripperware.Extent{
		Start:    acc.Start,
		End:      acc.End,
		Response: any,
		TraceId:  acc.TraceId,
	}), nil
}

func newAccumulator(base tripperware.Extent) (*accumulator, error) {
	res, err := extentToResponse(base)
	if err != nil {
		return nil, err
	}
	return &accumulator{
		Response: res,
		Extent:   base,
	}, nil
}

func toExtent(ctx context.Context, req tripperware.Request, res tripperware.Response) (tripperware.Extent, error) {
	any, err := types.MarshalAny(res)
	if err != nil {
		return tripperware.Extent{}, err
	}
	return tripperware.Extent{
		Start:    req.GetStart(),
		End:      req.GetEnd(),
		Response: any,
		TraceId:  jaegerTraceID(ctx),
	}, nil
}

func extentToResponse(e tripperware.Extent) (tripperware.Response, error) {
	msg, err := types.EmptyAny(e.Response)
	if err != nil {
		return nil, err
	}

	if err := types.UnmarshalAny(e.Response, msg); err != nil {
		return nil, err
	}

	resp, ok := msg.(tripperware.Response)
	if ok {
		return convertToTripperwarePrometheusResponse(resp), nil
	}
	return nil, fmt.Errorf("bad cached type")
}

// convertToTripperwarePrometheusResponse converts response from queryrange.PrometheusResponse format to tripperware.PrometheusResponse
func convertToTripperwarePrometheusResponse(resp tripperware.Response) tripperware.Response {
	r, ok := resp.(*PrometheusResponse)
	if !ok {
		// Should be tripperware.PrometheusResponse so we can return directly.
		return resp
	}
	if r.Data.GetResult() == nil {
		return tripperware.NewEmptyPrometheusResponse(false)
	}
	return &tripperware.PrometheusResponse{
		Status: r.Status,
		Data: tripperware.PrometheusData{
			ResultType: r.Data.ResultType,
			Result: tripperware.PrometheusQueryResult{
				Result: &tripperware.PrometheusQueryResult_Matrix{
					Matrix: &tripperware.Matrix{
						SampleStreams: r.Data.GetResult(),
					},
				},
			},
			Stats: r.Data.Stats,
		},
		ErrorType: r.ErrorType,
		Error:     r.Error,
		Headers:   r.Headers,
		Warnings:  r.Warnings,
		Infos:     r.Infos,
	}
}

// convertFromTripperwarePrometheusResponse converts response from tripperware.PrometheusResponse format to queryrange.PrometheusResponse
func convertFromTripperwarePrometheusResponse(resp tripperware.Response) tripperware.Response {
	r, ok := resp.(*tripperware.PrometheusResponse)
	if !ok {
		// Should be queryrange.PrometheusResponse so we can return directly.
		return resp
	}
	return &PrometheusResponse{
		Status: r.Status,
		Data: PrometheusData{
			ResultType: r.Data.ResultType,
			Result:     r.Data.Result.GetMatrix().GetSampleStreams(),
			Stats:      r.Data.Stats,
		},
		ErrorType: r.ErrorType,
		Error:     r.Error,
		Headers:   r.Headers,
		Warnings:  r.Warnings,
		Infos:     r.Infos,
	}
}

// partition calculates the required requests to satisfy req given the cached data.
// extents must be in order by start time.
func (s resultsCache) partition(ctx context.Context, req tripperware.Request, extents []tripperware.Extent) ([]tripperware.Request, []tripperware.Response, error) {
	var requests []tripperware.Request
	var cachedResponses []tripperware.Response
	start := req.GetStart()

	for _, extent := range extents {
		// If there is no overlap, ignore this extent.
		if extent.GetEnd() < start || extent.Start > req.GetEnd() {
			continue
		}

		// If this extent is tiny and request is not tiny, discard it: more efficient to do a few larger queries.
		// Hopefully tiny request can make tiny extent into not-so-tiny extent.

		// However if the step is large enough, the split_query_by_interval middleware would generate a query with same start and end.
		// For example, if the step size is more than 12h and the interval is 24h.
		// This means the extent's start and end time would be same, even if the timerange covers several hours.
		if (req.GetStart() != req.GetEnd()) && (req.GetEnd()-req.GetStart() > s.minCacheExtent) && (extent.End-extent.Start < s.minCacheExtent) {
			continue
		}

		// If there is a bit missing at the front, make a request for that.
		if start < extent.Start {
			r := req.WithStartEnd(start, extent.Start)
			requests = append(requests, r)
		}
		res, err := extentToResponse(extent)
		if err != nil {
			return nil, nil, err
		}
		// extract the overlap from the cached extent.
		promRes := s.extractor.Extract(start, req.GetEnd(), res).(*tripperware.PrometheusResponse)
		cachedResponses = append(cachedResponses, promRes)

		if queryStats := querier_stats.FromContext(ctx); queryStats != nil && promRes.Data.Stats != nil {
			queryStats.AddScannedSamples(uint64(promRes.Data.Stats.Samples.TotalQueryableSamples))
			queryStats.SetPeakSamples(max(queryStats.LoadPeakSamples(), uint64(promRes.Data.Stats.Samples.PeakSamples)))
		}

		start = extent.End
	}

	// Lastly, make a request for any data missing at the end.
	if start < req.GetEnd() {
		r := req.WithStartEnd(start, req.GetEnd())
		requests = append(requests, r)
	}

	// If start and end are the same (valid in promql), start == req.GetEnd() and we won't do the query.
	// But we should only do the request if we don't have a valid cached response for it.
	if req.GetStart() == req.GetEnd() && len(cachedResponses) == 0 {
		requests = append(requests, req)
	}

	return requests, cachedResponses, nil
}

func (s resultsCache) filterRecentExtents(req tripperware.Request, maxCacheFreshness time.Duration, extents []tripperware.Extent) ([]tripperware.Extent, error) {
	maxCacheTime := (int64(model.Now().Add(-maxCacheFreshness)) / req.GetStep()) * req.GetStep()
	for i := range extents {
		// Never cache data for the latest freshness period.
		if extents[i].End > maxCacheTime {
			extents[i].End = maxCacheTime
			res, err := extentToResponse(extents[i])
			if err != nil {
				return nil, err
			}
			extracted := s.extractor.Extract(extents[i].Start, maxCacheTime, res)
			any, err := types.MarshalAny(extracted)
			if err != nil {
				return nil, err
			}
			extents[i].Response = any
		}
	}
	return extents, nil
}

func (s resultsCache) get(ctx context.Context, key string) ([]tripperware.Extent, bool) {
	found, bufs, _ := s.cache.Fetch(ctx, []string{cache.HashKey(key)})
	if len(found) != 1 {
		return nil, false
	}

	var resp tripperware.CachedResponse
	log, ctx := spanlogger.New(ctx, "unmarshal-extent") //nolint:ineffassign,staticcheck
	defer log.Finish()

	log.LogFields(otlog.Int("bytes", len(bufs[0])))

	if err := proto.Unmarshal(bufs[0], &resp); err != nil {
		level.Error(log).Log("msg", "error unmarshalling cached value", "err", err)
		log.Error(err)
		return nil, false
	}

	if resp.Key != key {
		return nil, false
	}

	// Refreshes the cache if it contains an old proto schema.
	for _, e := range resp.Extents {
		if e.Response == nil {
			return nil, false
		}
	}

	return resp.Extents, true
}

func (s resultsCache) put(ctx context.Context, key string, extents []tripperware.Extent) {
	buf, err := proto.Marshal(&tripperware.CachedResponse{
		Key:     key,
		Extents: extents,
	})
	if err != nil {
		level.Error(util_log.WithContext(ctx, s.logger)).Log("msg", "error marshalling cached value", "err", err)
		return
	}

	s.cache.Store(ctx, []string{cache.HashKey(key)}, [][]byte{buf})
}

func jaegerSpanID(ctx context.Context) string {
	spanContext, ok := getSpanContext(ctx)
	if !ok {
		return ""
	}

	return spanContext.SpanID().String()
}

func jaegerTraceID(ctx context.Context) string {
	spanContext, ok := getSpanContext(ctx)
	if !ok {
		return ""
	}

	return spanContext.TraceID().String()
}

func getSpanContext(ctx context.Context) (jaeger.SpanContext, bool) {
	span := opentracing.SpanFromContext(ctx)
	if span == nil {
		return jaeger.SpanContext{}, false
	}

	spanContext, ok := span.Context().(jaeger.SpanContext)
	if !ok {
		return jaeger.SpanContext{}, false
	}

	return spanContext, true
}

// extractStats returns the stats for a given time range
// this function is similar to extractSampleStream
func extractStats(start, end int64, stats *tripperware.PrometheusResponseStats) *tripperware.PrometheusResponseStats {
	if stats == nil || stats.Samples == nil {
		return stats
	}

	result := &tripperware.PrometheusResponseStats{Samples: &tripperware.PrometheusResponseSamplesStats{}}
	for _, s := range stats.Samples.TotalQueryableSamplesPerStep {
		if start <= s.TimestampMs && s.TimestampMs <= end {
			result.Samples.TotalQueryableSamplesPerStep = append(result.Samples.TotalQueryableSamplesPerStep, s)
			result.Samples.TotalQueryableSamples += s.Value
			result.Samples.PeakSamples = max(result.Samples.PeakSamples, s.Value)
		}
	}
	return result
}

func extractSampleStreams(start, end int64, matrix []tripperware.SampleStream) []tripperware.SampleStream {
	result := make([]tripperware.SampleStream, 0, len(matrix))
	for _, stream := range matrix {
		extracted, ok := extractSampleStream(start, end, stream)
		if ok {
			result = append(result, extracted)
		}
	}
	return result
}

func extractSampleStream(start, end int64, stream tripperware.SampleStream) (tripperware.SampleStream, bool) {
	result := tripperware.SampleStream{
		Labels:  stream.Labels,
		Samples: make([]cortexpb.Sample, 0, len(stream.Samples)),
	}
	for _, sample := range stream.Samples {
		if start <= sample.TimestampMs && sample.TimestampMs <= end {
			result.Samples = append(result.Samples, sample)
		}
	}
	if len(result.Samples) == 0 {
		return tripperware.SampleStream{}, false
	}
	return result, true
}
