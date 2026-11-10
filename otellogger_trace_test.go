package bridge

import (
	"testing"
	"time"

	"github.com/agoda-com/opentelemetry-logs-go/logs"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// captureLogger records what the core emits, so a test can read the record it built.
type captureLogger struct{ records []logs.LogRecord }

func (c *captureLogger) Emit(r logs.LogRecord) { c.records = append(c.records, r) }

// A line that carries `trace_id` and `span_id` is emitted with them as the record's own trace
// context, which is what a backend joins on to list a trace's logs. Anything else leaves the
// trace context empty rather than inventing one.
func TestWriteRecordTraceContext(t *testing.T) {
	const traceHex = "4bf92f3577b34da6a3ce929d0e0e4736"
	const spanHex = "00f067aa0ba902b7"
	wantTrace, _ := trace.TraceIDFromHex(traceHex)
	wantSpan, _ := trace.SpanIDFromHex(spanHex)

	tests := []struct {
		name      string
		fields    []zapcore.Field
		wantTrace *trace.TraceID
		wantSpan  *trace.SpanID
	}{
		{
			name:      "trace and span ids become the record's trace context",
			fields:    []zapcore.Field{zap.String("trace_id", traceHex), zap.String("span_id", spanHex)},
			wantTrace: &wantTrace,
			wantSpan:  &wantSpan,
		},
		{
			name:      "a trace id alone is kept without a span",
			fields:    []zapcore.Field{zap.String("trace_id", traceHex)},
			wantTrace: &wantTrace,
		},
		{
			name:   "a span id without a trace id identifies nothing and is dropped",
			fields: []zapcore.Field{zap.String("span_id", spanHex)},
		},
		{
			name:   "a line without the fields has no trace context",
			fields: []zapcore.Field{zap.String("user_id", "u-1")},
		},
		{
			name:   "a malformed trace id is ignored",
			fields: []zapcore.Field{zap.String("trace_id", "not-hex"), zap.String("span_id", spanHex)},
		},
		{
			name:   "an all-zero trace id is invalid and ignored",
			fields: []zapcore.Field{zap.String("trace_id", "00000000000000000000000000000000")},
		},
		{
			name:   "a non-string trace_id field is ignored",
			fields: []zapcore.Field{zap.Int("trace_id", 42)},
		},
		{
			name:      "a malformed span id keeps the trace and drops the span",
			fields:    []zapcore.Field{zap.String("trace_id", traceHex), zap.String("span_id", "xyz")},
			wantTrace: &wantTrace,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capture := &captureLogger{}
			core := &OtelZapCore{logger: capture}

			err := core.Write(zapcore.Entry{Level: zapcore.InfoLevel, Time: time.Now(), Message: "m"}, test.fields)
			if err != nil {
				t.Fatalf("Write returned %v", err)
			}
			if len(capture.records) != 1 {
				t.Fatalf("emitted %d records, want 1", len(capture.records))
			}
			r := capture.records[0]

			if got := r.TraceId(); !equalPtr(got, test.wantTrace) {
				t.Errorf("TraceId = %v, want %v", got, test.wantTrace)
			}
			if got := r.SpanId(); !equalPtr(got, test.wantSpan) {
				t.Errorf("SpanId = %v, want %v", got, test.wantSpan)
			}
			// The fields stay attributes as well, so queries on them keep working.
			if got, want := len(*r.Attributes()), len(test.fields)+1; got != want { // +1: stacktrace
				t.Errorf("attributes = %d, want %d", got, want)
			}
		})
	}
}

func equalPtr[T comparable](got, want *T) bool {
	if got == nil || want == nil {
		return got == want
	}
	return *got == *want
}
