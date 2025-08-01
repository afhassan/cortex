package queryrange

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/weaveworks/common/httpgrpc"
	"github.com/weaveworks/common/user"

	"github.com/cortexproject/cortex/pkg/parser"
	"github.com/cortexproject/cortex/pkg/querier/tripperware"
	"github.com/cortexproject/cortex/pkg/util"
	"github.com/cortexproject/cortex/pkg/util/validation"
)

func TestLimitsMiddleware_MaxQueryLookback(t *testing.T) {
	t.Parallel()
	const (
		thirtyDays = 30 * 24 * time.Hour
	)

	now := time.Now()

	tests := map[string]struct {
		maxQueryLookback  time.Duration
		reqStartTime      time.Time
		reqEndTime        time.Time
		expectedSkipped   bool
		expectedStartTime time.Time
		expectedEndTime   time.Time
	}{
		"should not manipulate time range if max lookback is disabled": {
			maxQueryLookback:  0,
			reqStartTime:      time.Unix(0, 0),
			reqEndTime:        now,
			expectedStartTime: time.Unix(0, 0),
			expectedEndTime:   now,
		},
		"should not manipulate time range for a query on short time range": {
			maxQueryLookback:  thirtyDays,
			reqStartTime:      now.Add(-time.Hour),
			reqEndTime:        now,
			expectedStartTime: now.Add(-time.Hour),
			expectedEndTime:   now,
		},
		"should not manipulate a query on large time range close to the limit": {
			maxQueryLookback:  thirtyDays,
			reqStartTime:      now.Add(-thirtyDays).Add(time.Hour),
			reqEndTime:        now,
			expectedStartTime: now.Add(-thirtyDays).Add(time.Hour),
			expectedEndTime:   now,
		},
		"should manipulate a query on large time range over the limit": {
			maxQueryLookback:  thirtyDays,
			reqStartTime:      now.Add(-thirtyDays).Add(-100 * time.Hour),
			reqEndTime:        now,
			expectedStartTime: now.Add(-thirtyDays),
			expectedEndTime:   now,
		},
		"should skip executing a query outside the allowed time range": {
			maxQueryLookback: thirtyDays,
			reqStartTime:     now.Add(-thirtyDays).Add(-100 * time.Hour),
			reqEndTime:       now.Add(-thirtyDays).Add(-90 * time.Hour),
			expectedSkipped:  true,
		},
	}

	for testName, testData := range tests {
		testData := testData
		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			req := &tripperware.PrometheusRequest{
				Start: util.TimeToMillis(testData.reqStartTime),
				End:   util.TimeToMillis(testData.reqEndTime),
			}

			limits := mockLimits{maxQueryLookback: testData.maxQueryLookback}
			middleware := NewLimitsMiddleware(limits, 5*time.Minute)

			innerRes := tripperware.NewEmptyPrometheusResponse(false)
			inner := &mockHandler{}
			inner.On("Do", mock.Anything, mock.Anything).Return(innerRes, nil)

			ctx := user.InjectOrgID(context.Background(), "test")
			outer := middleware.Wrap(inner)
			res, err := outer.Do(ctx, req)
			require.NoError(t, err)

			if testData.expectedSkipped {
				// We expect an empty response, but not the one returned by the inner handler
				// which we expect has been skipped.
				assert.NotSame(t, innerRes, res)
				assert.Len(t, inner.Calls, 0)
			} else {
				// We expect the response returned by the inner handler.
				assert.Same(t, innerRes, res)

				// Assert on the time range of the request passed to the inner handler (5s delta).
				delta := float64(5000)
				require.Len(t, inner.Calls, 1)
				assert.InDelta(t, util.TimeToMillis(testData.expectedStartTime), inner.Calls[0].Arguments.Get(1).(tripperware.Request).GetStart(), delta)
				assert.InDelta(t, util.TimeToMillis(testData.expectedEndTime), inner.Calls[0].Arguments.Get(1).(tripperware.Request).GetEnd(), delta)
			}
		})
	}
}

func TestLimitsMiddleware_MaxQueryLength(t *testing.T) {
	t.Parallel()
	const (
		thirtyDays = 30 * 24 * time.Hour
	)

	now := time.Now()

	wrongQuery := `up[`
	_, parserErr := parser.ParseExpr(wrongQuery)

	tests := map[string]struct {
		maxQueryLength time.Duration
		query          string
		reqStartTime   time.Time
		reqEndTime     time.Time
		expectedErr    string
	}{
		"should skip validation if max length is disabled": {
			maxQueryLength: 0,
			reqStartTime:   time.Unix(0, 0),
			reqEndTime:     now,
		},
		"even though failed to parse expression, should return no error since request will pass to next middleware": {
			query:          `up[`,
			reqStartTime:   now.Add(-time.Hour),
			reqEndTime:     now,
			maxQueryLength: thirtyDays,
			expectedErr:    httpgrpc.Errorf(http.StatusBadRequest, "%s", parserErr.Error()).Error(),
		},
		"should succeed on a query on short time range, ending now": {
			maxQueryLength: thirtyDays,
			reqStartTime:   now.Add(-time.Hour),
			reqEndTime:     now,
		},
		"should fail on query with time window > max query length": {
			query:          "up[31d]",
			maxQueryLength: thirtyDays,
			reqStartTime:   now.Add(-time.Hour),
			reqEndTime:     now,
			expectedErr:    "the query time range exceeds the limit",
		},
		"should fail on query with time window > max query length, considering multiple selects": {
			query:          "rate(up[20d]) + rate(up[20d] offset 20d)",
			maxQueryLength: thirtyDays,
			reqStartTime:   now.Add(-time.Hour),
			reqEndTime:     now,
			expectedErr:    "the query time range exceeds the limit",
		},
		"should succeed on a query on short time range, ending in the past": {
			maxQueryLength: thirtyDays,
			reqStartTime:   now.Add(-2 * thirtyDays).Add(-time.Hour),
			reqEndTime:     now.Add(-2 * thirtyDays),
		},
		"should succeed on a query on large time range close to the limit, ending now": {
			maxQueryLength: thirtyDays,
			reqStartTime:   now.Add(-thirtyDays).Add(time.Hour),
			reqEndTime:     now,
		},
		"should fail on a query on large time range over the limit, ending now": {
			maxQueryLength: thirtyDays,
			reqStartTime:   now.Add(-thirtyDays).Add(-100 * time.Hour),
			reqEndTime:     now,
			expectedErr:    "the query time range exceeds the limit",
		},
		"should fail on a query on large time range over the limit, ending in the past": {
			maxQueryLength: thirtyDays,
			reqStartTime:   now.Add(-4 * thirtyDays),
			reqEndTime:     now.Add(-2 * thirtyDays),
			expectedErr:    "the query time range exceeds the limit",
		},
		"shouldn't exceed time range when having multiple selects with offset": {
			query:          `rate(up[5m]) + rate(up[5m] offset 40d) + rate(up[5m] offset 80d)`,
			maxQueryLength: thirtyDays,
			reqStartTime:   now.Add(-time.Hour),
			reqEndTime:     now,
		},
	}

	for testName, testData := range tests {
		testData := testData
		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			req := &tripperware.PrometheusRequest{
				Query: testData.query,
				Start: util.TimeToMillis(testData.reqStartTime),
				End:   util.TimeToMillis(testData.reqEndTime),
			}
			if req.Query == "" {
				req.Query = "up"
			}

			limits := mockLimits{maxQueryLength: testData.maxQueryLength}
			middleware := NewLimitsMiddleware(limits, 5*time.Minute)

			innerRes := tripperware.NewEmptyPrometheusResponse(false)
			inner := &mockHandler{}
			inner.On("Do", mock.Anything, mock.Anything).Return(innerRes, nil)

			ctx := user.InjectOrgID(context.Background(), "test")
			outer := middleware.Wrap(inner)
			res, err := outer.Do(ctx, req)

			if testData.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), testData.expectedErr)
				assert.Nil(t, res)
				assert.Len(t, inner.Calls, 0)
			} else {
				// We expect the response returned by the inner handler.
				require.NoError(t, err)
				assert.Same(t, innerRes, res)

				// The time range of the request passed to the inner handler should have not been manipulated.
				require.Len(t, inner.Calls, 1)
				assert.Equal(t, util.TimeToMillis(testData.reqStartTime), inner.Calls[0].Arguments.Get(1).(tripperware.Request).GetStart())
				assert.Equal(t, util.TimeToMillis(testData.reqEndTime), inner.Calls[0].Arguments.Get(1).(tripperware.Request).GetEnd())
			}
		})
	}
}

type mockLimits struct {
	maxQueryLookback       time.Duration
	maxQueryLength         time.Duration
	maxCacheFreshness      time.Duration
	maxQueryResponseSize   int64
	queryVerticalShardSize int
}

func (m mockLimits) MaxQueryLookback(string) time.Duration {
	return m.maxQueryLookback
}

func (m mockLimits) MaxQueryLength(string) time.Duration {
	return m.maxQueryLength
}

func (mockLimits) MaxQueryParallelism(string) int {
	return 14 // Flag default.
}

func (m mockLimits) MaxCacheFreshness(string) time.Duration {
	return m.maxCacheFreshness
}

func (m mockLimits) MaxQueryResponseSize(string) int64 {
	return m.maxQueryResponseSize
}

func (m mockLimits) QueryVerticalShardSize(userID string) int {
	return m.queryVerticalShardSize
}

func (m mockLimits) QueryPriority(userID string) validation.QueryPriority {
	return validation.QueryPriority{}
}

func (m mockLimits) QueryRejection(userID string) validation.QueryRejection {
	return validation.QueryRejection{}
}

type mockHandler struct {
	mock.Mock
}

func (m *mockHandler) Do(ctx context.Context, req tripperware.Request) (tripperware.Response, error) {
	args := m.Called(ctx, req)
	return args.Get(0).(tripperware.Response), args.Error(1)
}
