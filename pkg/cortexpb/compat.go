package cortexpb

import (
	stdjson "encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unsafe"

	jsoniter "github.com/json-iterator/go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"

	"github.com/cortexproject/cortex/pkg/util"
)

// ToWriteRequest converts matched slices of Labels, Samples, Metadata and Histograms into a WriteRequest proto.
// It gets timeseries from the pool, so ReuseSlice() should be called when done.
func ToWriteRequest(lbls []labels.Labels, samples []Sample, metadata []*MetricMetadata, histograms []Histogram, source WriteRequest_SourceEnum) *WriteRequest {
	req := &WriteRequest{
		Timeseries: PreallocTimeseriesSliceFromPool(),
		Metadata:   metadata,
		Source:     source,
	}

	i := 0
	for i < len(samples) || i < len(histograms) {
		ts := TimeseriesFromPool()
		ts.Labels = append(ts.Labels, FromLabelsToLabelAdapters(lbls[i])...)
		if i < len(samples) {
			ts.Samples = append(ts.Samples, samples[i])
		}
		if i < len(histograms) {
			ts.Histograms = append(ts.Histograms, histograms[i])
		}
		i++
		req.Timeseries = append(req.Timeseries, PreallocTimeseries{TimeSeries: ts})
	}

	return req
}

func (w *WriteRequest) AddHistogramTimeSeries(lbls []labels.Labels, histograms []Histogram) {
	for i := 0; i < len(lbls); i++ {
		ts := TimeseriesFromPool()
		ts.Labels = append(ts.Labels, FromLabelsToLabelAdapters(lbls[i])...)
		ts.Histograms = append(ts.Histograms, histograms[i])
		w.Timeseries = append(w.Timeseries, PreallocTimeseries{TimeSeries: ts})
	}
}

// FromLabelAdaptersToLabels casts []LabelAdapter to labels.Labels.
// It uses unsafe, but as LabelAdapter == labels.Label this should be safe.
// This allows us to use labels.Labels directly in protos.
//
// Note: while resulting labels.Labels is supposedly sorted, this function
// doesn't enforce that. If input is not sorted, output will be wrong.
func FromLabelAdaptersToLabels(ls []LabelAdapter) labels.Labels {
	return *(*labels.Labels)(unsafe.Pointer(&ls))
}

// FromLabelAdaptersToLabelsWithCopy converts []LabelAdapter to labels.Labels.
// Do NOT use unsafe to convert between data types because this function may
// get in input labels whose data structure is reused.
func FromLabelAdaptersToLabelsWithCopy(input []LabelAdapter) labels.Labels {
	return CopyLabels(input)
}

// Efficiently copies labels input slice. To be used in cases where input slice
// can be reused, but long-term copy is needed.
func CopyLabels(input []LabelAdapter) labels.Labels {
	builder := labels.NewBuilder(labels.EmptyLabels())

	size := 0
	for _, l := range input {
		size += len(l.Name)
		size += len(l.Value)
	}

	// Copy all strings into the buffer, and use 'yoloString' to convert buffer
	// slices to strings.
	buf := make([]byte, size)
	var name, value string

	for _, l := range input {
		name, buf = copyStringToBuffer(l.Name, buf)
		value, buf = copyStringToBuffer(l.Value, buf)
		builder.Set(name, value)
	}
	return builder.Labels()
}

// Copies string to buffer (which must be big enough), and converts buffer slice containing
// the string copy into new string.
func copyStringToBuffer(in string, buf []byte) (string, []byte) {
	l := len(in)
	c := copy(buf, in)
	if c != l {
		panic("not copied full string")
	}

	return yoloString(buf[0:l]), buf[l:]
}

// FromLabelsToLabelAdapters casts labels.Labels to []LabelAdapter.
// It uses unsafe, but as LabelAdapter == labels.Label this should be safe.
// This allows us to use labels.Labels directly in protos.
func FromLabelsToLabelAdapters(ls labels.Labels) []LabelAdapter {
	return *(*[]LabelAdapter)(unsafe.Pointer(&ls))
}

// FromLabelAdaptersToMetric converts []LabelAdapter to a model.Metric.
// Don't do this on any performance sensitive paths.
func FromLabelAdaptersToMetric(ls []LabelAdapter) model.Metric {
	return util.LabelsToMetric(FromLabelAdaptersToLabels(ls))
}

// FromLabelAdaptersToMetric converts []LabelAdapter to a model.Metric with copy.
// Don't do this on any performance sensitive paths.
func FromLabelAdaptersToMetricWithCopy(ls []LabelAdapter) model.Metric {
	return util.LabelsToMetric(FromLabelAdaptersToLabelsWithCopy(ls))
}

// FromMetricsToLabelAdapters converts model.Metric to []LabelAdapter.
// Don't do this on any performance sensitive paths.
// The result is sorted.
func FromMetricsToLabelAdapters(metric model.Metric) []LabelAdapter {
	result := make([]LabelAdapter, 0, len(metric))
	for k, v := range metric {
		result = append(result, LabelAdapter{
			Name:  string(k),
			Value: string(v),
		})
	}
	sort.Sort(byLabel(result)) // The labels should be sorted upon initialisation.
	return result
}

func FromExemplarsToExemplarProtos(es []exemplar.Exemplar) []Exemplar {
	result := make([]Exemplar, 0, len(es))
	for _, e := range es {
		result = append(result, Exemplar{
			Labels:      FromLabelsToLabelAdapters(e.Labels),
			Value:       e.Value,
			TimestampMs: e.Ts,
		})
	}
	return result
}

func FromExemplarProtosToExemplars(es []Exemplar) []exemplar.Exemplar {
	result := make([]exemplar.Exemplar, 0, len(es))
	for _, e := range es {
		result = append(result, exemplar.Exemplar{
			Labels: FromLabelAdaptersToLabels(e.Labels),
			Value:  e.Value,
			Ts:     e.TimestampMs,
		})
	}
	return result
}

type byLabel []LabelAdapter

func (s byLabel) Len() int           { return len(s) }
func (s byLabel) Less(i, j int) bool { return strings.Compare(s[i].Name, s[j].Name) < 0 }
func (s byLabel) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// MetricMetadataMetricTypeToMetricType converts a metric type from our internal client
// to a Prometheus one.
func MetricMetadataMetricTypeToMetricType(mt MetricMetadata_MetricType) model.MetricType {
	switch mt {
	case UNKNOWN:
		return model.MetricTypeUnknown
	case COUNTER:
		return model.MetricTypeCounter
	case GAUGE:
		return model.MetricTypeGauge
	case HISTOGRAM:
		return model.MetricTypeHistogram
	case GAUGEHISTOGRAM:
		return model.MetricTypeGaugeHistogram
	case SUMMARY:
		return model.MetricTypeSummary
	case INFO:
		return model.MetricTypeInfo
	case STATESET:
		return model.MetricTypeStateset
	default:
		return model.MetricTypeUnknown
	}
}

// isTesting is only set from tests to get special behaviour to verify that custom sample encode and decode is used,
// both when using jsonitor or standard json package.
var isTesting = false

// MarshalJSON implements json.Marshaler.
func (s Sample) MarshalJSON() ([]byte, error) {
	if isTesting && math.IsNaN(s.Value) {
		return nil, fmt.Errorf("test sample")
	}

	t, err := jsoniter.ConfigCompatibleWithStandardLibrary.Marshal(model.Time(s.TimestampMs))
	if err != nil {
		return nil, err
	}
	v, err := jsoniter.ConfigCompatibleWithStandardLibrary.Marshal(model.SampleValue(s.Value))
	if err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf("[%s,%s]", t, v)), nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (s *Sample) UnmarshalJSON(b []byte) error {
	var t model.Time
	var v model.SampleValue
	vs := [...]stdjson.Unmarshaler{&t, &v}
	if err := jsoniter.ConfigCompatibleWithStandardLibrary.Unmarshal(b, &vs); err != nil {
		return err
	}
	s.TimestampMs = int64(t)
	s.Value = float64(v)

	if isTesting && math.IsNaN(float64(v)) {
		return fmt.Errorf("test sample")
	}
	return nil
}

func SampleJsoniterEncode(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	sample := (*Sample)(ptr)

	if isTesting && math.IsNaN(sample.Value) {
		stream.Error = fmt.Errorf("test sample")
		return
	}

	stream.WriteArrayStart()
	stream.WriteFloat64(float64(sample.TimestampMs) / float64(time.Second/time.Millisecond))
	stream.WriteMore()
	stream.WriteString(model.SampleValue(sample.Value).String())
	stream.WriteArrayEnd()
}

func SampleJsoniterDecode(ptr unsafe.Pointer, iter *jsoniter.Iterator) {
	if !iter.ReadArray() {
		iter.ReportError("cortexpb.Sample", "expected [")
		return
	}

	t := model.Time(iter.ReadFloat64() * float64(time.Second/time.Millisecond))

	if !iter.ReadArray() {
		iter.ReportError("cortexpb.Sample", "expected ,")
		return
	}

	bs := iter.ReadStringAsSlice()
	ss := *(*string)(unsafe.Pointer(&bs))
	v, err := strconv.ParseFloat(ss, 64)
	if err != nil {
		iter.ReportError("cortexpb.Sample", err.Error())
		return
	}

	if isTesting && math.IsNaN(v) {
		iter.Error = fmt.Errorf("test sample")
		return
	}

	if iter.ReadArray() {
		iter.ReportError("cortexpb.Sample", "expected ]")
	}

	*(*Sample)(ptr) = Sample{
		TimestampMs: int64(t),
		Value:       v,
	}
}

func init() {
	jsoniter.RegisterTypeEncoderFunc("cortexpb.Sample", SampleJsoniterEncode, func(unsafe.Pointer) bool { return false })
	jsoniter.RegisterTypeDecoderFunc("cortexpb.Sample", SampleJsoniterDecode)
}
