package queryrange

import (
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/stretchr/testify/require"

	"github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/cortexproject/cortex/pkg/querier/tripperware"
)

func TestFromValue(t *testing.T) {
	var testExpr = []struct {
		input    *promql.Result
		err      bool
		expected []tripperware.SampleStream
	}{
		// string (errors)
		{
			input: &promql.Result{Value: promql.String{T: 1, V: "hi"}},
			err:   true,
		},
		{
			input: &promql.Result{Err: errors.New("foo")},
			err:   true,
		},
		// Scalar
		{
			input: &promql.Result{Value: promql.Scalar{T: 1, V: 1}},
			err:   false,
			expected: []tripperware.SampleStream{
				{
					Samples: []cortexpb.Sample{
						{
							Value:       1,
							TimestampMs: 1,
						},
					},
				},
			},
		},
		// Vector
		{
			input: &promql.Result{
				Value: promql.Vector{
					promql.Sample{
						T:      1,
						F:      1,
						Metric: labels.FromStrings("a", "a1", "b", "b1"),
					},
					promql.Sample{
						T:      2,
						F:      2,
						Metric: labels.FromStrings("a", "a2", "b", "b2"),
					},
				},
			},
			err: false,
			expected: []tripperware.SampleStream{
				{
					Labels: []cortexpb.LabelAdapter{
						{Name: "a", Value: "a1"},
						{Name: "b", Value: "b1"},
					},
					Samples: []cortexpb.Sample{
						{
							Value:       1,
							TimestampMs: 1,
						},
					},
				},
				{
					Labels: []cortexpb.LabelAdapter{
						{Name: "a", Value: "a2"},
						{Name: "b", Value: "b2"},
					},
					Samples: []cortexpb.Sample{
						{
							Value:       2,
							TimestampMs: 2,
						},
					},
				},
			},
		},
		// Matrix
		{
			input: &promql.Result{
				Value: promql.Matrix{
					{
						Metric: labels.FromStrings("a", "a1", "b", "b1"),
						Floats: []promql.FPoint{
							{T: 1, F: 1},
							{T: 2, F: 2},
						},
					},
					{
						Metric: labels.FromStrings("a", "a2", "b", "b2"),
						Floats: []promql.FPoint{
							{T: 1, F: 8},
							{T: 2, F: 9},
						},
					},
				},
			},
			err: false,
			expected: []tripperware.SampleStream{
				{
					Labels: []cortexpb.LabelAdapter{
						{Name: "a", Value: "a1"},
						{Name: "b", Value: "b1"},
					},
					Samples: []cortexpb.Sample{
						{
							Value:       1,
							TimestampMs: 1,
						},
						{
							Value:       2,
							TimestampMs: 2,
						},
					},
				},
				{
					Labels: []cortexpb.LabelAdapter{
						{Name: "a", Value: "a2"},
						{Name: "b", Value: "b2"},
					},
					Samples: []cortexpb.Sample{
						{
							Value:       8,
							TimestampMs: 1,
						},
						{
							Value:       9,
							TimestampMs: 2,
						},
					},
				},
			},
		},
	}

	for i, c := range testExpr {
		c := c
		t.Run(fmt.Sprintf("[%d]", i), func(t *testing.T) {
			t.Parallel()
			result, err := FromResult(c.input)
			if c.err {
				require.NotNil(t, err)
			} else {
				require.Nil(t, err)
				require.Equal(t, c.expected, result)
			}
		})
	}
}
