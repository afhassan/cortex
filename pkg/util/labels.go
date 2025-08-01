package util

import (
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
)

// LabelsToMetric converts a Labels to Metric
// Don't do this on any performance sensitive paths.
func LabelsToMetric(ls labels.Labels) model.Metric {
	m := make(model.Metric, ls.Len())
	ls.Range(func(l labels.Label) {
		m[model.LabelName(l.Name)] = model.LabelValue(l.Value)
	})
	return m
}

// LabelMatchersToString returns a string representing the input label matchers.
func LabelMatchersToString(matchers []*labels.Matcher) string {
	out := strings.Builder{}
	out.WriteRune('{')

	for idx, m := range matchers {
		if idx > 0 {
			out.WriteRune(',')
		}

		out.WriteString(m.String())
	}

	out.WriteRune('}')
	return out.String()
}
