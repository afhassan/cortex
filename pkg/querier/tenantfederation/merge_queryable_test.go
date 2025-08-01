package tenantfederation

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
	"github.com/weaveworks/common/user"

	"github.com/cortexproject/cortex/pkg/querier/series"
	"github.com/cortexproject/cortex/pkg/storage/bucket"
	cortex_tsdb "github.com/cortexproject/cortex/pkg/storage/tsdb"
	"github.com/cortexproject/cortex/pkg/tenant"
	"github.com/cortexproject/cortex/pkg/util/services"
	"github.com/cortexproject/cortex/pkg/util/spanlogger"
	"github.com/cortexproject/cortex/pkg/util/test"
)

const (
	maxt, mint = 0, 10
	// mockMatchersNotImplemented is a message used to indicate that the mockTenantQueryable used in the tests does not support filtering by matchers.
	mockMatchersNotImplemented = "matchers are not implemented in the mockTenantQueryable"
	// originalDefaultTenantLabel is the default tenant label with a prefix.
	// It is used to prevent matcher clashes for timeseries that happen to have a label with the same name as the default tenant label.
	originalDefaultTenantLabel = retainExistingPrefix + defaultTenantLabel
	// seriesWithLabelNames can be used as matcher's label name in LabelNames call.
	// If it's the only matcher provided then the result label names are the pipe-split value of the matcher.
	// Matcher type is ignored.
	seriesWithLabelNames = "series_with_label_names"
)

// mockTenantQueryableWithFilter is a storage.Queryable that can be use to return specific warnings or errors by tenant.
type mockTenantQueryableWithFilter struct {
	// extraLabels are labels added to all series for all tenants.
	extraLabels []string
	// warningsByTenant are warnings that will be returned for queries of that tenant.
	warningsByTenant map[string]annotations.Annotations
	// queryErrByTenant is an error that will be returne for queries of that tenant.
	queryErrByTenant map[string]error
}

// Querier implements the storage.Queryable interface.
func (m *mockTenantQueryableWithFilter) Querier(_, _ int64) (storage.Querier, error) {
	q := mockTenantQuerier{
		extraLabels:      m.extraLabels,
		warnings:         annotations.Annotations(nil),
		warningsByTenant: m.warningsByTenant,
		queryErrByTenant: m.queryErrByTenant,
	}

	return q, nil
}

// UseQueryable implements the querier.QueryableWithFilter interface.
// It ensures the mockTenantQueryableWithFilter storage.Queryable is always used.
func (m *mockTenantQueryableWithFilter) UseQueryable(_ time.Time, _, _ int64) bool {
	return true
}

type mockTenantQuerier struct {
	extraLabels []string

	warnings annotations.Annotations
	queryErr error

	// warningsByTenant are warnings that will be returned for queries of that tenant.
	warningsByTenant map[string]annotations.Annotations
	// queryErrByTenant is an error that will be returne for queries of that tenant.
	queryErrByTenant map[string]error
}

func (m mockTenantQuerier) matrix(tenant string) model.Matrix {
	matrix := model.Matrix{
		&model.SampleStream{
			Metric: model.Metric{
				"instance":                          "host1",
				"tenant-" + model.LabelName(tenant): "static",
			},
		},
		&model.SampleStream{
			Metric: model.Metric{
				"instance": "host2." + model.LabelValue(tenant),
			},
		},
	}

	// Add extra labels to every sample stream in the matrix.
	for pos := range m.extraLabels {
		if pos%2 == 0 {
			continue
		}

		for mPos := range matrix {
			matrix[mPos].Metric[model.LabelName(m.extraLabels[pos-1])] = model.LabelValue(m.extraLabels[pos])
		}
	}

	return matrix

}

// metricMatches returns whether or not the selector matches the provided metric.
func metricMatches(m model.Metric, selector labels.Selector) bool {
	var labelStrings []string
	for key, value := range m {
		labelStrings = append(labelStrings, string(key), string(value))
	}

	return selector.Matches(labels.FromStrings(labelStrings...))
}

type mockSeriesSet struct {
	upstream storage.SeriesSet
	warnings annotations.Annotations
	queryErr error
}

func (m *mockSeriesSet) Next() bool {
	return m.upstream.Next()
}

// At implements the storage.SeriesSet interface. It returns full series. Returned series should be iterable even after Next is called.
func (m *mockSeriesSet) At() storage.Series {
	return m.upstream.At()
}

// Err implements the storage.SeriesSet interface. It returns the error that iteration as failed with.
// When an error occurs, set cannot continue to iterate.
func (m *mockSeriesSet) Err() error {
	return m.queryErr
}

// Warnings implements the storage.SeriesSet interface. It returns a collection of warnings for the whole set.
// Warnings could be returned even if iteration has not failed with error.
func (m *mockSeriesSet) Warnings() annotations.Annotations {
	return m.warnings
}

// Select implements the storage.Querier interface.
func (m mockTenantQuerier) Select(ctx context.Context, _ bool, sp *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return storage.ErrSeriesSet(err)
	}

	// set warning if exists
	if m.warningsByTenant != nil {
		if w, ok := m.warningsByTenant[tenantIDs[0]]; ok {
			m.warnings.Merge(w)
		}
	}

	// set queryErr if exists
	if m.queryErrByTenant != nil {
		if err, ok := m.queryErrByTenant[tenantIDs[0]]; ok {
			m.queryErr = err
		}
	}

	log, _ := spanlogger.New(ctx, "mockTenantQuerier.select")
	defer log.Finish()
	var matrix model.Matrix

	for _, s := range m.matrix(tenantIDs[0]) {
		if metricMatches(s.Metric, matchers) {
			matrix = append(matrix, s)
		}
	}

	return &mockSeriesSet{
		upstream: series.MatrixToSeriesSet(false, matrix),
		warnings: m.warnings,
		queryErr: m.queryErr,
	}
}

// LabelValues implements the storage.LabelQuerier interface.
// The mockTenantQuerier returns all a sorted slice of all label values and does not support reducing the result set with matchers.
func (m mockTenantQuerier) LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, nil, err
	}

	// set warning if exists
	if m.warningsByTenant != nil {
		if w, ok := m.warningsByTenant[tenantIDs[0]]; ok {
			m.warnings.Merge(w)
		}
	}

	// set queryErr if exists
	if m.queryErrByTenant != nil {
		if err, ok := m.queryErrByTenant[tenantIDs[0]]; ok {
			m.queryErr = err
		}
	}

	if len(matchers) > 0 {
		m.warnings.Add(errors.New(mockMatchersNotImplemented))
	}

	if m.queryErr != nil {
		return nil, nil, m.queryErr
	}

	labelValues := make(map[string]struct{})
	for _, s := range m.matrix(tenantIDs[0]) {
		for k, v := range s.Metric {
			if k == model.LabelName(name) {
				labelValues[string(v)] = struct{}{}
			}
		}
	}
	var results []string
	for k := range labelValues {
		results = append(results, k)
	}
	sort.Strings(results)
	return results, m.warnings, nil
}

// LabelNames implements the storage.LabelQuerier interface.
// It returns a sorted slice of all label names in the querier.
// If only one matcher is provided with label Name=seriesWithLabelNames then the resulting set will have the values of that matchers pipe-split appended.
// I.e. querying for {seriesWithLabelNames="foo|bar|baz"} will have as result [bar, baz, foo, <rest of label names from querier matrix> ]
func (m mockTenantQuerier) LabelNames(ctx context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, nil, err
	}
	// set warning if exists
	if m.warningsByTenant != nil {
		if w, ok := m.warningsByTenant[tenantIDs[0]]; ok {
			m.warnings.Merge(w)
		}
	}

	// set queryErr if exists
	if m.queryErrByTenant != nil {
		if err, ok := m.queryErrByTenant[tenantIDs[0]]; ok {
			m.queryErr = err
		}
	}

	var results []string

	if len(matchers) == 1 && matchers[0].Name == seriesWithLabelNames {
		if matchers[0].Value == "" {
			return nil, m.warnings, nil
		}
		results = strings.Split(matchers[0].Value, "|")
	} else if len(matchers) > 1 {
		m.warnings.Add(errors.New(mockMatchersNotImplemented))
	}

	if m.queryErr != nil {
		return nil, nil, m.queryErr
	}

	labelValues := make(map[string]struct{})
	for _, s := range m.matrix(tenantIDs[0]) {
		for k := range s.Metric {
			labelValues[string(k)] = struct{}{}
		}
	}

	for k := range labelValues {
		results = append(results, k)
	}
	sort.Strings(results)
	return results, m.warnings, nil
}

// Close implements the storage.LabelQuerier interface.
func (mockTenantQuerier) Close() error {
	return nil
}

// mergeQueryableScenario is a setup for testing a a MergeQueryable.
type mergeQueryableScenario struct {
	// name is a description of the scenario.
	name string
	// tenants are the tenants over which queries will be merged.
	tenants   []string
	queryable mockTenantQueryableWithFilter
	// doNotByPassSingleQuerier determines whether the MergeQueryable is by-passed in favor of a single querier.
	doNotByPassSingleQuerier bool
}

func (s *mergeQueryableScenario) init() (storage.Querier, prometheus.Gatherer, error) {
	// initialize with default tenant label
	reg := prometheus.NewPedanticRegistry()
	q := NewQueryable(&s.queryable, defaultMaxConcurrency, !s.doNotByPassSingleQuerier, reg)

	// retrieve querier
	querier, err := q.Querier(mint, maxt)

	return querier, reg, err
}

// selectTestCase is the inputs and expected outputs of a call to Select.
type selectTestCase struct {
	// name is a description of the test case.
	name string
	// matchers is a slice of label matchers used to filter series in the test case.
	matchers []*labels.Matcher
	// expectedSeriesCount is the expected number of series returned by a Select filtered by the Matchers in selector.
	expectedSeriesCount int
	// expectedLabels is the expected label sets returned by a Select filtered by the Matchers in selector.
	expectedLabels []labels.Labels
	// expectedWarnings is a slice of annotations.Annotations messages expected when querying.
	expectedWarnings []string
	// expectedQueryErr is the error expected when querying.
	expectedQueryErr error
	// expectedMetrics is the expected metrics.
	expectedMetrics string
}

// selectScenario tests a call to Select over a range of test cases in a specific scenario.
type selectScenario struct {
	mergeQueryableScenario
	selectTestCases []selectTestCase
}

// labelNamesTestCase is the inputs and expected outputs of a call to LabelNames.
type labelNamesTestCase struct {
	// name is a description of the test case.
	name string
	// matchers is a slice of label matchers used to filter series in the test case.
	matchers []*labels.Matcher
	// expectedLabelNames are the expected label names returned from the queryable.
	expectedLabelNames []string
	// expectedWarnings is a slice of annotations.Annotations messages expected when querying.
	expectedWarnings []string
	// expectedQueryErr is the error expected when querying.
	expectedQueryErr error
	// expectedMetrics is the expected metrics.
	expectedMetrics string
}

// labelNamesScenario tests a call to LabelNames in a specific scenario.
type labelNamesScenario struct {
	mergeQueryableScenario
	labelNamesTestCase
}

// labeValuesTestCase is the inputs and expected outputs of a call to LabelValues.
type labelValuesTestCase struct {
	// name is a description of the test case.
	name string
	// labelName is the name of the label to query values for.
	labelName string
	// matchers is a slice of label matchers used to filter series in the test case.
	matchers []*labels.Matcher
	// expectedLabelValues are the expected label values returned from the queryable.
	expectedLabelValues []string
	// expectedWarnings is a slice of annotations.Annotations messages expected when querying.
	expectedWarnings []string
	// expectedQueryErr is the error expected when querying.
	expectedQueryErr error
	// expectedMetrics is the expected metrics.
	expectedMetrics string
}

// labelValuesScenario tests a call to LabelValues over a range of test cases in a specific scenario.
type labelValuesScenario struct {
	mergeQueryableScenario
	labelValuesTestCases []labelValuesTestCase
}

func TestMergeQueryable_Querier(t *testing.T) {
	t.Run("querying without a tenant specified should error", func(t *testing.T) {
		t.Parallel()
		queryable := &mockTenantQueryableWithFilter{}
		q := NewQueryable(queryable, defaultMaxConcurrency, false /* byPassWithSingleQuerier */, nil)

		querier, err := q.Querier(mint, maxt)
		require.NoError(t, err)

		_, _, err = querier.LabelValues(context.Background(), "test", nil)
		require.EqualError(t, err, user.ErrNoOrgID.Error())
	})
}

var (
	singleTenantScenario = mergeQueryableScenario{
		name:    "single tenant",
		tenants: []string{"team-a"},
	}

	singleTenantNoBypassScenario = mergeQueryableScenario{
		name:                     "single tenant without bypass",
		tenants:                  []string{"team-a"},
		doNotByPassSingleQuerier: true,
	}

	threeTenantsScenario = mergeQueryableScenario{
		name:    "three tenants",
		tenants: []string{"team-a", "team-b", "team-c"},
	}

	threeTenantsWithDefaultTenantIDScenario = mergeQueryableScenario{
		name:    "three tenants and the __tenant_id__ label set",
		tenants: []string{"team-a", "team-b", "team-c"},
		queryable: mockTenantQueryableWithFilter{
			extraLabels: []string{defaultTenantLabel, "original-value"},
		},
	}

	threeTenantsWithWarningsScenario = mergeQueryableScenario{
		name:    "three tenants, two with warnings",
		tenants: []string{"team-a", "team-b", "team-c"},
		queryable: mockTenantQueryableWithFilter{
			warningsByTenant: map[string]annotations.Annotations{
				"team-b": annotations.New().Add(errors.New("don't like them")),
				"team-c": annotations.New().Add(errors.New("out of office")),
			},
		},
	}

	threeTenantsWithErrorScenario = mergeQueryableScenario{
		name:    "three tenants, one erroring",
		tenants: []string{"team-a", "team-b", "team-c"},
		queryable: mockTenantQueryableWithFilter{
			queryErrByTenant: map[string]error{
				"team-b": errors.New("failure xyz"),
			},
		},
	}

	expectedSingleTenantsMetrics = `
# HELP cortex_querier_federated_tenants_per_query Number of tenants per query.
# TYPE cortex_querier_federated_tenants_per_query histogram
cortex_querier_federated_tenants_per_query_bucket{le="1"} 1
cortex_querier_federated_tenants_per_query_bucket{le="2"} 1
cortex_querier_federated_tenants_per_query_bucket{le="4"} 1
cortex_querier_federated_tenants_per_query_bucket{le="8"} 1
cortex_querier_federated_tenants_per_query_bucket{le="16"} 1
cortex_querier_federated_tenants_per_query_bucket{le="32"} 1
cortex_querier_federated_tenants_per_query_bucket{le="64"} 1
cortex_querier_federated_tenants_per_query_bucket{le="+Inf"} 1
cortex_querier_federated_tenants_per_query_sum 1
cortex_querier_federated_tenants_per_query_count 1
`

	expectedThreeTenantsMetrics = `
# HELP cortex_querier_federated_tenants_per_query Number of tenants per query.
# TYPE cortex_querier_federated_tenants_per_query histogram
cortex_querier_federated_tenants_per_query_bucket{le="1"} 0
cortex_querier_federated_tenants_per_query_bucket{le="2"} 0
cortex_querier_federated_tenants_per_query_bucket{le="4"} 1
cortex_querier_federated_tenants_per_query_bucket{le="8"} 1
cortex_querier_federated_tenants_per_query_bucket{le="16"} 1
cortex_querier_federated_tenants_per_query_bucket{le="32"} 1
cortex_querier_federated_tenants_per_query_bucket{le="64"} 1
cortex_querier_federated_tenants_per_query_bucket{le="+Inf"} 1
cortex_querier_federated_tenants_per_query_sum 3
cortex_querier_federated_tenants_per_query_count 1
`
)

func TestMergeQueryable_Select(t *testing.T) {
	for _, scenario := range []selectScenario{
		{
			mergeQueryableScenario: threeTenantsScenario,
			selectTestCases: []selectTestCase{
				{
					name:                "should return all series when no matchers are provided",
					expectedSeriesCount: 6,
					expectedMetrics:     expectedThreeTenantsMetrics,
				},
				{
					name:                "should return only series for team-a and team-c tenants when there is a not-equals matcher for the team-b tenant",
					matchers:            []*labels.Matcher{{Name: defaultTenantLabel, Value: "team-b", Type: labels.MatchNotEqual}},
					expectedSeriesCount: 4,
					expectedLabels: []labels.Labels{
						labels.FromStrings(
							"__tenant_id__", "team-a",
							"instance", "host1",
							"tenant-team-a", "static",
						),
						labels.FromStrings(
							"__tenant_id__", "team-a",
							"instance", "host2.team-a",
						),
						labels.FromStrings(
							"__tenant_id__", "team-c",
							"instance", "host1",
							"tenant-team-c", "static",
						),
						labels.FromStrings(
							"__tenant_id__", "team-c",
							"instance", "host2.team-c",
						),
					},
					expectedMetrics: expectedThreeTenantsMetrics,
				},
				{
					name:                "should return only series for team-b when there is an equals matcher for the team-b tenant",
					matchers:            []*labels.Matcher{{Name: defaultTenantLabel, Value: "team-b", Type: labels.MatchEqual}},
					expectedSeriesCount: 2,
					expectedLabels: []labels.Labels{
						labels.FromStrings(
							"__tenant_id__", "team-b",
							"instance", "host1",
							"tenant-team-b", "static",
						),
						labels.FromStrings(
							"__tenant_id__", "team-b",
							"instance", "host2.team-b",
						),
					},
					expectedMetrics: expectedThreeTenantsMetrics,
				},
				{
					name:                "should return one series for each tenant when there is an equals matcher for the host1 instance",
					matchers:            []*labels.Matcher{{Name: "instance", Value: "host1", Type: labels.MatchEqual}},
					expectedSeriesCount: 3,
					expectedMetrics:     expectedThreeTenantsMetrics,
				},
			},
		},
		{
			mergeQueryableScenario: threeTenantsWithDefaultTenantIDScenario,
			selectTestCases: []selectTestCase{
				{
					name:                "should return all series when no matchers are provided",
					expectedSeriesCount: 6,
					expectedLabels: []labels.Labels{
						labels.FromStrings(
							"__tenant_id__", "team-a",
							"instance", "host1",
							"original___tenant_id__", "original-value",
							"tenant-team-a", "static",
						),
						labels.FromStrings(
							"__tenant_id__", "team-a",
							"instance", "host2.team-a",
							"original___tenant_id__", "original-value",
						),
						labels.FromStrings(
							"__tenant_id__", "team-b",
							"instance", "host1",
							"original___tenant_id__", "original-value",
							"tenant-team-b", "static",
						),
						labels.FromStrings(
							"__tenant_id__", "team-b",
							"instance", "host2.team-b",
							"original___tenant_id__", "original-value",
						),
						labels.FromStrings(
							"__tenant_id__", "team-c",
							"instance", "host1",
							"original___tenant_id__", "original-value",
							"tenant-team-c", "static",
						),
						labels.FromStrings(
							"__tenant_id__", "team-c",
							"instance", "host2.team-c",
							"original___tenant_id__", "original-value",
						),
					},
					expectedMetrics: expectedThreeTenantsMetrics,
				},
				{
					name:                "should return only series for team-a and team-c tenants when there is with not-equals matcher for the team-b tenant",
					matchers:            []*labels.Matcher{{Name: defaultTenantLabel, Value: "team-b", Type: labels.MatchNotEqual}},
					expectedSeriesCount: 4,
					expectedMetrics:     expectedThreeTenantsMetrics,
				},
				{
					name: "should return no series where there are conflicting tenant matchers",
					matchers: []*labels.Matcher{
						{Name: defaultTenantLabel, Value: "team-a", Type: labels.MatchEqual}, {Name: defaultTenantLabel, Value: "team-c", Type: labels.MatchEqual}},
					expectedSeriesCount: 0,
					expectedMetrics:     expectedThreeTenantsMetrics,
				},
				{
					name:                "should return only series for team-b when there is an equals matcher for team-b tenant",
					matchers:            []*labels.Matcher{{Name: defaultTenantLabel, Value: "team-b", Type: labels.MatchEqual}},
					expectedSeriesCount: 2,
					expectedLabels: []labels.Labels{
						labels.FromStrings(
							"__tenant_id__", "team-b",
							"instance", "host1",
							"original___tenant_id__", "original-value",
							"tenant-team-b", "static",
						),
						labels.FromStrings(
							"__tenant_id__", "team-b",
							"instance", "host2.team-b",
							"original___tenant_id__", "original-value",
						),
					},
					expectedMetrics: expectedThreeTenantsMetrics,
				},
				{
					name:                "should return all series when there is an equals matcher for the original value of __tenant_id__ using the revised tenant label",
					matchers:            []*labels.Matcher{{Name: originalDefaultTenantLabel, Value: "original-value", Type: labels.MatchEqual}},
					expectedSeriesCount: 6,
					expectedMetrics:     expectedThreeTenantsMetrics,
				},
				{
					name:                "should return all series when there is a regexp matcher for the original value of __tenant_id__ using the revised tenant label",
					matchers:            []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, originalDefaultTenantLabel, "original-value")},
					expectedSeriesCount: 6,
					expectedMetrics:     expectedThreeTenantsMetrics,
				},
				{
					name:                "should return no series when there is a not-equals matcher for the original value of __tenant_id__ using the revised tenant label",
					matchers:            []*labels.Matcher{{Name: originalDefaultTenantLabel, Value: "original-value", Type: labels.MatchNotEqual}},
					expectedSeriesCount: 0,
					expectedMetrics:     expectedThreeTenantsMetrics,
				},
			},
		},
		{
			mergeQueryableScenario: threeTenantsWithWarningsScenario,
			selectTestCases: []selectTestCase{{
				name: "should return warnings from all tenant queryables",
				expectedWarnings: []string{
					`warning querying tenant_id team-b: don't like them`,
					`warning querying tenant_id team-c: out of office`,
				},
				expectedSeriesCount: 6,
				expectedMetrics:     expectedThreeTenantsMetrics,
			},
			}},
		{
			mergeQueryableScenario: threeTenantsWithErrorScenario,
			selectTestCases: []selectTestCase{{
				name:             "should return any error encountered with any tenant",
				expectedQueryErr: errors.New("error querying tenant_id team-b: failure xyz"),
				expectedMetrics:  expectedThreeTenantsMetrics,
			}},
		},
	} {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			for _, useRegexResolver := range []bool{true, false} {
				for _, tc := range scenario.selectTestCases {
					tc := tc
					t.Run(fmt.Sprintf("%s, useRegexResolver: %v", tc.name, useRegexResolver), func(t *testing.T) {
						ctx := context.Background()
						if useRegexResolver {
							reg := prometheus.NewRegistry()
							bucketClient := &bucket.ClientMock{}
							bucketClient.MockIter("", scenario.tenants, nil)
							bucketClient.MockIter("__markers__", []string{}, nil)

							for _, tenant := range scenario.tenants {
								bucketClient.MockExists(cortex_tsdb.GetGlobalDeletionMarkPath(tenant), false, nil)
								bucketClient.MockExists(cortex_tsdb.GetLocalDeletionMarkPath(tenant), false, nil)
							}

							bucketClientFactory := func(ctx context.Context) (objstore.InstrumentedBucket, error) {
								return bucketClient, nil
							}

							usersScannerConfig := cortex_tsdb.UsersScannerConfig{Strategy: cortex_tsdb.UserScanStrategyList}
							tenantFederationConfig := Config{UserSyncInterval: time.Second}
							regexResolver, err := NewRegexResolver(usersScannerConfig, tenantFederationConfig, reg, bucketClientFactory, log.NewNopLogger())
							require.NoError(t, err)

							// set a regex tenant resolver
							tenant.WithDefaultResolver(regexResolver)
							require.NoError(t, services.StartAndAwaitRunning(context.Background(), regexResolver))

							// wait update knownUsers
							test.Poll(t, time.Second*10, true, func() interface{} {
								return testutil.ToFloat64(regexResolver.lastUpdateUserRun) > 0 && testutil.ToFloat64(regexResolver.discoveredUsers) == float64(len(scenario.tenants))
							})

							ctx = user.InjectOrgID(ctx, "team-.+")
						} else {
							// Set a multi tenant resolver.
							tenant.WithDefaultResolver(tenant.NewMultiResolver())

							// inject tenants into context
							if len(scenario.tenants) > 0 {
								ctx = user.InjectOrgID(ctx, strings.Join(scenario.tenants, "|"))
							}
						}

						querier, reg, err := scenario.init()
						require.NoError(t, err)

						seriesSet := querier.Select(ctx, true, &storage.SelectHints{Start: mint, End: maxt}, tc.matchers...)

						if tc.expectedQueryErr != nil {
							require.EqualError(t, seriesSet.Err(), tc.expectedQueryErr.Error())
						} else {
							require.NoError(t, seriesSet.Err())
							assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(tc.expectedMetrics), "cortex_querier_federated_tenants_per_query"))
							assertEqualWarnings(t, tc.expectedWarnings, seriesSet.Warnings())
						}

						if tc.expectedLabels != nil {
							require.Equal(t, len(tc.expectedLabels), tc.expectedSeriesCount)
						}

						count := 0
						for i := 0; seriesSet.Next(); i++ {
							count++
							if tc.expectedLabels != nil {
								require.Equal(t, tc.expectedLabels[i], seriesSet.At().Labels(), fmt.Sprintf("labels index: %d", i))
							}
						}
						require.Equal(t, tc.expectedSeriesCount, count)
					})
				}
			}
		})
	}
}

func TestMergeQueryable_LabelNames(t *testing.T) {
	for _, scenario := range []labelNamesScenario{
		{
			mergeQueryableScenario: singleTenantScenario,
			labelNamesTestCase: labelNamesTestCase{
				name:               "should not return the __tenant_id__ label as the MergeQueryable has been bypassed",
				expectedLabelNames: []string{"instance", "tenant-team-a"},
				expectedMetrics:    expectedSingleTenantsMetrics,
			},
		},
		{
			mergeQueryableScenario: singleTenantScenario,
			labelNamesTestCase: labelNamesTestCase{
				name:               "should not return the __tenant_id__ label as the MergeQueryable has been bypassed with matchers",
				matchers:           []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, seriesWithLabelNames, "bar|foo")},
				expectedLabelNames: []string{"bar", "foo", "instance", "tenant-team-a"},
				expectedMetrics:    expectedSingleTenantsMetrics,
			},
		},
		{
			mergeQueryableScenario: singleTenantNoBypassScenario,
			labelNamesTestCase: labelNamesTestCase{
				name:               "should return the __tenant_id__ label as the MergeQueryable has not been bypassed",
				expectedLabelNames: []string{defaultTenantLabel, "instance", "tenant-team-a"},
				expectedMetrics:    expectedSingleTenantsMetrics,
			},
		},
		{
			mergeQueryableScenario: threeTenantsScenario,
			labelNamesTestCase: labelNamesTestCase{
				name:               "should return the __tenant_id__ label and all tenant team labels",
				expectedLabelNames: []string{defaultTenantLabel, "instance", "tenant-team-a", "tenant-team-b", "tenant-team-c"},
				expectedMetrics:    expectedThreeTenantsMetrics,
			},
		},
		{
			mergeQueryableScenario: threeTenantsWithDefaultTenantIDScenario,
			labelNamesTestCase: labelNamesTestCase{
				name:               "should return  the __tenant_id__ label and all tenant team labels, and the __original_tenant_id__ label",
				expectedLabelNames: []string{defaultTenantLabel, "instance", originalDefaultTenantLabel, "tenant-team-a", "tenant-team-b", "tenant-team-c"},
				expectedMetrics:    expectedThreeTenantsMetrics,
			},
		},
		{
			mergeQueryableScenario: threeTenantsWithWarningsScenario,
			labelNamesTestCase: labelNamesTestCase{
				name:               "should return warnings from all tenant queryables",
				expectedLabelNames: []string{defaultTenantLabel, "instance", "tenant-team-a", "tenant-team-b", "tenant-team-c"},
				expectedWarnings: []string{
					`warning querying tenant_id team-b: don't like them`,
					`warning querying tenant_id team-c: out of office`,
				},
				expectedMetrics: expectedThreeTenantsMetrics,
			},
		},
		{
			mergeQueryableScenario: threeTenantsWithErrorScenario,
			labelNamesTestCase: labelNamesTestCase{
				name:             "should return any error encountered with any tenant",
				expectedQueryErr: errors.New("error querying tenant_id team-b: failure xyz"),
				expectedMetrics:  expectedThreeTenantsMetrics,
			},
		},
		{
			mergeQueryableScenario: threeTenantsWithWarningsScenario,
			labelNamesTestCase: labelNamesTestCase{
				name:               "should propagate non-tenant matchers to downstream queriers",
				matchers:           []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, seriesWithLabelNames, "bar|foo")},
				expectedLabelNames: []string{defaultTenantLabel, "bar", "foo", "instance", "tenant-team-a", "tenant-team-b", "tenant-team-c"},
				expectedWarnings: []string{
					`warning querying tenant_id team-b: don't like them`,
					`warning querying tenant_id team-c: out of office`,
				},
				expectedMetrics: expectedThreeTenantsMetrics,
			},
		},
		{
			mergeQueryableScenario: threeTenantsWithWarningsScenario,
			labelNamesTestCase: labelNamesTestCase{

				name: "should only query tenant-b when there is an equals matcher for team-b tenant",
				matchers: []*labels.Matcher{
					{Name: defaultTenantLabel, Value: "team-b", Type: labels.MatchEqual},
					labels.MustNewMatcher(labels.MatchRegexp, seriesWithLabelNames, "bar|foo"),
				},
				expectedLabelNames: []string{defaultTenantLabel, "bar", "foo", "instance", "tenant-team-b"},
				expectedWarnings: []string{
					`warning querying tenant_id team-b: don't like them`,
				},
				expectedMetrics: expectedThreeTenantsMetrics,
			},
		},
		{
			mergeQueryableScenario: threeTenantsWithWarningsScenario,
			labelNamesTestCase: labelNamesTestCase{
				name: "should only query tenant-b and tenant-c when there is an regex matcher for team-b|team-c tenant",
				matchers: []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchRegexp, defaultTenantLabel, "team-b|team-c"),
					labels.MustNewMatcher(labels.MatchRegexp, seriesWithLabelNames, "bar|foo"),
				},
				expectedLabelNames: []string{defaultTenantLabel, "bar", "foo", "instance", "tenant-team-b", "tenant-team-c"},
				expectedWarnings: []string{
					`warning querying tenant_id team-b: don't like them`,
					`warning querying tenant_id team-c: out of office`,
				},
				expectedMetrics: expectedThreeTenantsMetrics,
			},
		},
		{
			mergeQueryableScenario: threeTenantsWithWarningsScenario,
			labelNamesTestCase: labelNamesTestCase{
				name: "only tenant-b is selected and it already has a defaultTenantLabel which is prepended with original_ prefix",
				matchers: []*labels.Matcher{
					{Name: defaultTenantLabel, Value: "team-b", Type: labels.MatchEqual},
					labels.MustNewMatcher(labels.MatchRegexp, seriesWithLabelNames, defaultTenantLabel),
				},
				expectedLabelNames: []string{defaultTenantLabel, "instance", originalDefaultTenantLabel, "tenant-team-b"},
				expectedWarnings: []string{
					`warning querying tenant_id team-b: don't like them`,
				},
				expectedMetrics: expectedThreeTenantsMetrics,
			},
		},
	} {
		scenario := scenario
		for _, useRegexResolver := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s, useRegexResolver: %v", scenario.mergeQueryableScenario.name, useRegexResolver), func(t *testing.T) {
				ctx := context.Background()
				if useRegexResolver {
					reg := prometheus.NewRegistry()
					bucketClient := &bucket.ClientMock{}
					bucketClient.MockIter("", scenario.tenants, nil)
					bucketClient.MockIter("__markers__", []string{}, nil)

					for _, tenant := range scenario.tenants {
						bucketClient.MockExists(cortex_tsdb.GetGlobalDeletionMarkPath(tenant), false, nil)
						bucketClient.MockExists(cortex_tsdb.GetLocalDeletionMarkPath(tenant), false, nil)
					}

					bucketClientFactory := func(ctx context.Context) (objstore.InstrumentedBucket, error) {
						return bucketClient, nil
					}
					usersScannerConfig := cortex_tsdb.UsersScannerConfig{Strategy: cortex_tsdb.UserScanStrategyList}
					tenantFederationConfig := Config{UserSyncInterval: time.Second}
					regexResolver, err := NewRegexResolver(usersScannerConfig, tenantFederationConfig, reg, bucketClientFactory, log.NewNopLogger())
					require.NoError(t, err)

					// set a regex tenant resolver
					tenant.WithDefaultResolver(regexResolver)
					require.NoError(t, services.StartAndAwaitRunning(context.Background(), regexResolver))

					// wait update knownUsers
					test.Poll(t, time.Second*10, true, func() interface{} {
						return testutil.ToFloat64(regexResolver.lastUpdateUserRun) > 0 && testutil.ToFloat64(regexResolver.discoveredUsers) == float64(len(scenario.tenants))
					})

					ctx = user.InjectOrgID(ctx, "team-.+")
				} else {
					// Set a multi tenant resolver.
					tenant.WithDefaultResolver(tenant.NewMultiResolver())

					// inject tenants into context
					if len(scenario.tenants) > 0 {
						ctx = user.InjectOrgID(ctx, strings.Join(scenario.tenants, "|"))
					}
				}

				querier, reg, err := scenario.init()
				require.NoError(t, err)

				t.Run(scenario.labelNamesTestCase.name, func(t *testing.T) {
					t.Parallel()
					labelNames, warnings, err := querier.LabelNames(ctx, nil, scenario.matchers...)
					if scenario.expectedQueryErr != nil {
						require.EqualError(t, err, scenario.expectedQueryErr.Error())
					} else {
						require.NoError(t, err)
						assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(scenario.expectedMetrics), "cortex_querier_federated_tenants_per_query"))
						assert.Equal(t, scenario.expectedLabelNames, labelNames)
						assertEqualWarnings(t, scenario.expectedWarnings, warnings)
					}
				})
			})
		}
	}
}

func TestMergeQueryable_LabelValues(t *testing.T) {
	for _, scenario := range []labelValuesScenario{
		{
			mergeQueryableScenario: singleTenantScenario,
			labelValuesTestCases: []labelValuesTestCase{
				{
					name:                "should return all label values for instance when no matchers are provided",
					labelName:           "instance",
					expectedLabelValues: []string{"host1", "host2.team-a"},
					expectedMetrics:     expectedSingleTenantsMetrics,
				},
				{
					name:                "should return no tenant values for the __tenant_id__ label as the MergeQueryable has been bypassed",
					labelName:           defaultTenantLabel,
					expectedLabelValues: nil,
					expectedMetrics:     expectedSingleTenantsMetrics,
				},
			},
		},
		{
			mergeQueryableScenario: singleTenantNoBypassScenario,
			labelValuesTestCases: []labelValuesTestCase{
				{
					name:                "should return all label values for instance when no matchers are provided",
					labelName:           "instance",
					expectedLabelValues: []string{"host1", "host2.team-a"},
					expectedMetrics:     expectedSingleTenantsMetrics,
				},
				{
					name:                "should return a tenant team value for the __tenant_id__ label as the MergeQueryable has not been bypassed",
					labelName:           defaultTenantLabel,
					expectedLabelValues: []string{"team-a"},
					expectedMetrics:     expectedSingleTenantsMetrics,
				},
			},
		},
		{
			mergeQueryableScenario: threeTenantsScenario,
			labelValuesTestCases: []labelValuesTestCase{
				{
					name:                "should return all label values for instance when no matchers are provided",
					labelName:           "instance",
					expectedLabelValues: []string{"host1", "host2.team-a", "host2.team-b", "host2.team-c"},
					expectedMetrics:     expectedThreeTenantsMetrics,
				},
				{
					name:      "should propagate non-tenant matchers to downstream queriers",
					matchers:  []*labels.Matcher{{Name: "instance", Value: "host2.team-b", Type: labels.MatchEqual}},
					labelName: "instance",
					// All label values are returned as the downstream queryable does not implement matching.
					expectedLabelValues: []string{"host1", "host2.team-a", "host2.team-b", "host2.team-c"},
					expectedWarnings: []string{
						"warning querying tenant_id team-a: " + mockMatchersNotImplemented,
						"warning querying tenant_id team-b: " + mockMatchersNotImplemented,
						"warning querying tenant_id team-c: " + mockMatchersNotImplemented,
					},
					expectedMetrics: expectedThreeTenantsMetrics,
				},
				{
					name: "should return no values for the instance label when there are conflicting tenant matchers",
					matchers: []*labels.Matcher{
						{Name: defaultTenantLabel, Value: "team-a", Type: labels.MatchEqual},
						{Name: defaultTenantLabel, Value: "team-c", Type: labels.MatchEqual},
					},
					labelName:           "instance",
					expectedLabelValues: []string{},
					expectedMetrics:     expectedThreeTenantsMetrics,
				},
				{
					name:                "should only query tenant-b when there is an equals matcher for team-b tenant",
					matchers:            []*labels.Matcher{{Name: defaultTenantLabel, Value: "team-b", Type: labels.MatchEqual}},
					labelName:           "instance",
					expectedLabelValues: []string{"host1", "host2.team-b"},
					expectedMetrics:     expectedThreeTenantsMetrics,
				},
				{
					name:                "should return all tenant team values for the __tenant_id__ label when no matchers are provided",
					labelName:           defaultTenantLabel,
					expectedLabelValues: []string{"team-a", "team-b", "team-c"},
					expectedMetrics:     expectedThreeTenantsMetrics,
				},
				{
					name:                "should return only label values for team-a and team-c tenants when there is a not-equals matcher for team-b tenant",
					labelName:           defaultTenantLabel,
					matchers:            []*labels.Matcher{{Name: defaultTenantLabel, Value: "team-b", Type: labels.MatchNotEqual}},
					expectedLabelValues: []string{"team-a", "team-c"},
					expectedMetrics:     expectedThreeTenantsMetrics,
				},
				{
					name:                "should return only label values for team-b tenant when there is an equals matcher for team-b tenant",
					labelName:           defaultTenantLabel,
					matchers:            []*labels.Matcher{{Name: defaultTenantLabel, Value: "team-b", Type: labels.MatchEqual}},
					expectedLabelValues: []string{"team-b"},
					expectedMetrics:     expectedThreeTenantsMetrics,
				},
			},
		},
		{
			mergeQueryableScenario: threeTenantsWithDefaultTenantIDScenario,
			labelValuesTestCases: []labelValuesTestCase{
				{
					name:                "should return all label values for instance when no matchers are provided",
					labelName:           "instance",
					expectedLabelValues: []string{"host1", "host2.team-a", "host2.team-b", "host2.team-c"},
					expectedMetrics:     expectedThreeTenantsMetrics,
				},
				{
					name:                "should return all tenant values for __tenant_id__ label name",
					labelName:           defaultTenantLabel,
					expectedLabelValues: []string{"team-a", "team-b", "team-c"},
					expectedMetrics:     expectedThreeTenantsMetrics,
				},
				{
					name:                "should return the original value for the revised tenant label name when no matchers are provided",
					labelName:           originalDefaultTenantLabel,
					expectedLabelValues: []string{"original-value"},
					expectedMetrics:     expectedThreeTenantsMetrics,
				},
				{
					name:                "should return the original value for the revised tenant label name with matchers",
					matchers:            []*labels.Matcher{{Name: originalDefaultTenantLabel, Value: "original-value", Type: labels.MatchEqual}},
					labelName:           originalDefaultTenantLabel,
					expectedLabelValues: []string{"original-value"},
					expectedWarnings: []string{
						"warning querying tenant_id team-a: " + mockMatchersNotImplemented,
						"warning querying tenant_id team-b: " + mockMatchersNotImplemented,
						"warning querying tenant_id team-c: " + mockMatchersNotImplemented,
					},
					expectedMetrics: expectedThreeTenantsMetrics,
				},
			},
		},
		{
			mergeQueryableScenario: threeTenantsWithWarningsScenario,
			labelValuesTestCases: []labelValuesTestCase{{
				name:                "should return warnings from all tenant queryables",
				labelName:           "instance",
				expectedLabelValues: []string{"host1", "host2.team-a", "host2.team-b", "host2.team-c"},
				expectedWarnings: []string{
					`warning querying tenant_id team-b: don't like them`,
					`warning querying tenant_id team-c: out of office`,
				},
				expectedMetrics: expectedThreeTenantsMetrics,
			}},
		},
		{
			mergeQueryableScenario: threeTenantsWithWarningsScenario,
			labelValuesTestCases: []labelValuesTestCase{{
				name:                "should not return warnings as the underlying queryables are not queried in requests for the __tenant_id__ label",
				labelName:           defaultTenantLabel,
				expectedLabelValues: []string{"team-a", "team-b", "team-c"},
				expectedMetrics:     expectedThreeTenantsMetrics,
			}},
		},
		{
			mergeQueryableScenario: threeTenantsWithErrorScenario,
			labelValuesTestCases: []labelValuesTestCase{{
				name:             "should return any error encountered with any tenant",
				labelName:        "instance",
				expectedQueryErr: errors.New("error querying tenant_id team-b: failure xyz"),
				expectedMetrics:  expectedThreeTenantsMetrics,
			}},
		},
		{
			mergeQueryableScenario: threeTenantsWithErrorScenario,
			labelValuesTestCases: []labelValuesTestCase{{
				name:                "should not return errors as the underlying queryables are not queried in requests for the __tenant_id__ label",
				labelName:           defaultTenantLabel,
				expectedLabelValues: []string{"team-a", "team-b", "team-c"},
				expectedMetrics:     expectedThreeTenantsMetrics,
			}},
		},
	} {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			for _, useRegexResolver := range []bool{true, false} {
				for _, tc := range scenario.labelValuesTestCases {
					t.Run(fmt.Sprintf("%s, useRegexResolver: %v", tc.name, useRegexResolver), func(t *testing.T) {
						ctx := context.Background()
						if useRegexResolver {
							reg := prometheus.NewRegistry()
							bucketClient := &bucket.ClientMock{}
							bucketClient.MockIter("", scenario.tenants, nil)
							bucketClient.MockIter("__markers__", []string{}, nil)

							for _, tenant := range scenario.tenants {
								bucketClient.MockExists(cortex_tsdb.GetGlobalDeletionMarkPath(tenant), false, nil)
								bucketClient.MockExists(cortex_tsdb.GetLocalDeletionMarkPath(tenant), false, nil)
							}

							bucketClientFactory := func(ctx context.Context) (objstore.InstrumentedBucket, error) {
								return bucketClient, nil
							}
							usersScannerConfig := cortex_tsdb.UsersScannerConfig{Strategy: cortex_tsdb.UserScanStrategyList}
							tenantFederationConfig := Config{UserSyncInterval: time.Second}
							regexResolver, err := NewRegexResolver(usersScannerConfig, tenantFederationConfig, reg, bucketClientFactory, log.NewNopLogger())
							require.NoError(t, err)

							// set a regex tenant resolver
							tenant.WithDefaultResolver(regexResolver)
							require.NoError(t, services.StartAndAwaitRunning(context.Background(), regexResolver))

							// wait update knownUsers
							test.Poll(t, time.Second*10, true, func() interface{} {
								return testutil.ToFloat64(regexResolver.lastUpdateUserRun) > 0 && testutil.ToFloat64(regexResolver.discoveredUsers) == float64(len(scenario.tenants))
							})

							ctx = user.InjectOrgID(ctx, "team-.+")
						} else {
							// Set a multi tenant resolver.
							tenant.WithDefaultResolver(tenant.NewMultiResolver())

							// inject tenants into context
							if len(scenario.tenants) > 0 {
								ctx = user.InjectOrgID(ctx, strings.Join(scenario.tenants, "|"))
							}
						}

						querier, reg, err := scenario.init()
						require.NoError(t, err)

						actLabelValues, warnings, err := querier.LabelValues(ctx, tc.labelName, nil, tc.matchers...)
						if tc.expectedQueryErr != nil {
							require.EqualError(t, err, tc.expectedQueryErr.Error())
						} else {
							require.NoError(t, err)
							assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(tc.expectedMetrics), "cortex_querier_federated_tenants_per_query"))
							assert.Equal(t, tc.expectedLabelValues, actLabelValues, fmt.Sprintf("unexpected values for label '%s'", tc.labelName))
							assertEqualWarnings(t, tc.expectedWarnings, warnings)
						}
					})
				}
			}
		})
	}
}

// assertEqualWarnings asserts that all the expected warning messages are present.
func assertEqualWarnings(t *testing.T, exp []string, act annotations.Annotations) {
	if len(exp) == 0 && len(act) == 0 {
		return
	}
	var actStrings = make([]string, len(act))
	warnings := act.AsErrors()
	for pos := range warnings {
		actStrings[pos] = warnings[pos].Error()
	}
	assert.ElementsMatch(t, exp, actStrings)
}

func TestSetLabelsRetainExisting(t *testing.T) {
	for _, tc := range []struct {
		labels           labels.Labels
		additionalLabels labels.Labels
		expected         labels.Labels
	}{
		// Test adding labels at the end.
		{
			labels:           labels.FromStrings("a", "b"),
			additionalLabels: labels.FromStrings("c", "d"),
			expected:         labels.FromStrings("a", "b", "c", "d"),
		},

		// Test adding labels at the beginning.
		{
			labels:           labels.FromStrings("c", "d"),
			additionalLabels: labels.FromStrings("a", "b"),
			expected:         labels.FromStrings("a", "b", "c", "d"),
		},

		// Test we do override existing labels and expose the original value.
		{
			labels:           labels.FromStrings("a", "b"),
			additionalLabels: labels.FromStrings("a", "c"),
			expected:         labels.FromStrings("a", "c", "original_a", "b"),
		},

		// Test we do override existing labels but don't do it recursively.
		{
			labels:           labels.FromStrings("a", "b", "original_a", "i am lost"),
			additionalLabels: labels.FromStrings("a", "d"),
			expected:         labels.FromStrings("a", "d", "original_a", "b"),
		},
	} {
		assert.Equal(t, tc.expected, setLabelsRetainExisting(tc.labels, tc.additionalLabels))
	}
}

func TestTracingMergeQueryable(t *testing.T) {
	mockTracer := mocktracer.New()
	opentracing.SetGlobalTracer(mockTracer)
	ctx := user.InjectOrgID(context.Background(), "team-a|team-b")

	// set a multi tenant resolver
	tenant.WithDefaultResolver(tenant.NewMultiResolver())
	filter := mockTenantQueryableWithFilter{}
	q := NewQueryable(&filter, defaultMaxConcurrency, false, nil)
	// retrieve querier if set
	querier, err := q.Querier(mint, maxt)
	require.NoError(t, err)

	seriesSet := querier.Select(ctx, true, &storage.SelectHints{Start: mint,
		End: maxt})

	require.NoError(t, seriesSet.Err())
	spans := mockTracer.FinishedSpans()
	assertSpanExist(t, spans, "mergeQuerier.Select", expectedTag{spanlogger.TenantIDTagName,
		[]string{"team-a", "team-b"}})
	assertSpanExist(t, spans, "mockTenantQuerier.select", expectedTag{spanlogger.TenantIDTagName,
		[]string{"team-a"}})
	assertSpanExist(t, spans, "mockTenantQuerier.select", expectedTag{spanlogger.TenantIDTagName,
		[]string{"team-b"}})
}

func assertSpanExist(t *testing.T,
	actualSpans []*mocktracer.MockSpan,
	name string,
	tag expectedTag) {
	for _, span := range actualSpans {
		if span.OperationName == name && containsTags(span, tag) {
			return
		}
	}
	require.FailNow(t, "can not find span matching params",
		"expected span with name `%v` and with "+
			"tags %v to be present but it was not. actual spans: %+v",
		name, tag, extractNameWithTags(actualSpans))
}

func extractNameWithTags(actualSpans []*mocktracer.MockSpan) []spanWithTags {
	result := make([]spanWithTags, len(actualSpans))
	for i, span := range actualSpans {
		result[i] = spanWithTags{span.OperationName, span.Tags()}
	}
	return result
}

func containsTags(span *mocktracer.MockSpan, expectedTag expectedTag) bool {
	return reflect.DeepEqual(span.Tag(expectedTag.key), expectedTag.values)
}

type spanWithTags struct {
	name string
	tags map[string]interface{}
}

type expectedTag struct {
	key    string
	values []string
}
