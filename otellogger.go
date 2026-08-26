package bridge

import (
	"context"
	"os"
	"strings"
	"time"

	autosdk "github.com/agoda-com/opentelemetry-logs-go/autoconfigure/sdk/logs"
	"github.com/agoda-com/opentelemetry-logs-go/logs"
	sdk "github.com/agoda-com/opentelemetry-logs-go/sdk/logs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	otelSdkDisabled            = "OTEL_SDK_DISABLED"
	instrumentationLibraryName = "github.com/odigos-io/opentelemetry-zap-bridge"
)

// syncTimeout bounds the flush that Sync performs.
//
// zapcore.Core.Sync carries no context, so the bound has to live here. It exists rather than
// blocking indefinitely because the callers that need Sync most are short-lived processes — a
// CronJob under concurrencyPolicy: Forbid, where a flush that hangs against an unreachable
// collector does not merely delay the pod, it starts skipping the next tick's work.
const syncTimeout = 5 * time.Second

type OtelZapCore struct {
	zapcore.Core

	logger logs.Logger

	// provider is retained SOLELY so the records this core buffers can be flushed.
	//
	// Without it the provider is unreachable the moment NewOtelZapCore returns, and its batch
	// processor's buffer is lost whenever the process exits before the batch timer fires. That is
	// invisible in a long-running service and total in a short-lived one: measured on a Kubernetes
	// CronJob fleet, one cron in twenty delivered two log lines in twenty-four hours while every
	// metric arrived, because metrics had an explicit shutdown and logs had no handle to call one on.
	provider *sdk.LoggerProvider
}

// this function creates a new zapcore.Core that can be used with zap.New()
// this instance will translate zap logs to opentelemetry logs and export them
//
// serviceName is required and is used as a resource attribute in the reported telemetry.
// TODO: we should probably extend this to support more options like additional resource attributes
// TODO2: should we also support a way to configure other components and configuration options?
// like exporters, processors, etc.
// Currently a user can configure the SDK only via environment variables which is fair enough
// but advanced users might want more control.
func NewOtelZapCore() zapcore.Core {
	core, _ := NewOtelZapCoreWithShutdown()

	return core
}

// NewOtelZapCoreWithShutdown is NewOtelZapCore plus the handle needed to flush it.
//
// The returned function shuts the LoggerProvider down, delivering whatever the batch processor is
// still holding, and is safe to call more than once. It honours the caller's context, so a process
// that already budgets its telemetry shutdown — one deadline per signal, so a dead collector cannot
// spend another signal's budget — can give this one the same treatment.
//
// PREFER THIS OVER NewOtelZapCore IN ANY PROCESS THAT EXITS. A cron, a job, a CLI or a test binary
// finishes long before the batch timer fires, and without this its logs are simply dropped — no
// error, no warning, and the records are gone. A long-running service can keep using NewOtelZapCore,
// where the periodic flush is enough.
func NewOtelZapCoreWithShutdown() (zapcore.Core, func(context.Context) error) {
	ctx := context.Background()
	loggerProvider := autosdk.NewLoggerProvider(ctx)
	// TODO: what scope name should we use?
	// should we allow this to be conbfigurable?
	// how do we record correct scope name with zap?
	logger := loggerProvider.Logger(instrumentationLibraryName)

	core := &OtelZapCore{
		logger:   logger,
		provider: loggerProvider,
	}

	return core, core.shutdown
}

// TODO: I guess there is more idomatic way to do this in go
func AttachToZapLogger(logger *zap.Logger) *zap.Logger {
	otelSdlDisabled, defined := os.LookupEnv(otelSdkDisabled)
	// do not register the otel zap core if user set OTEL_SDK_DISABLED=true
	if defined && strings.ToLower(otelSdlDisabled) == "true" {
		return logger
	}

	return logger.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		otelZapLogger := NewOtelZapCore()
		return zapcore.NewTee(core, otelZapLogger)
	}))
}

// TODO: see how it is implemented in zapcore and consider adding it here as well
func (o *OtelZapCore) Enabled(zapcore.Level) bool {
	return true
}

// Sync flushes the records this core is holding, which is what zap's contract asks of it.
//
// It also closes a latent panic. OtelZapCore embeds zapcore.Core as an interface and never assigns
// it, so before this method existed Sync was promoted to that nil interface — any caller doing the
// ordinary `defer logger.Sync()` would have taken a nil dereference. Nothing in this repository's
// dependents called it, which is the only reason it went unnoticed.
//
// The flush is bounded by syncTimeout because Sync takes no context; a caller that needs to choose
// its own deadline should use the shutdown function from NewOtelZapCoreWithShutdown instead.
func (o *OtelZapCore) Sync() error {
	if o.provider == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	defer cancel()

	return o.provider.ForceFlush(ctx)
}

// shutdown delivers what the batch processor still holds and releases it.
//
// Shutdown rather than ForceFlush: this is the end of the process, so the processor should stop
// accepting records as well as drain, and calling it twice must not be an error for a caller whose
// cleanup runs on more than one path.
func (o *OtelZapCore) shutdown(ctx context.Context) error {
	if o.provider == nil {
		return nil
	}

	return o.provider.Shutdown(ctx)
}

// TODO: implement this and add the fields to each new log record created
func (o *OtelZapCore) With(fields []zapcore.Field) zapcore.Core {
	return o
}

func (o *OtelZapCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return ce.AddCore(ent, o)
}

func (o *OtelZapCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	otelSeverity, severityText := convertZapLevelToOtelSeverity(ent.Level)

	fields = append(fields, []zap.Field{
		zap.String("stacktrace", ent.Stack),
	}...)

	otelEncoder := newZapOtelEncoder(len(fields))
	for _, field := range fields {
		field.AddTo(otelEncoder)
	}

	lrc := logs.LogRecordConfig{
		Timestamp:      &ent.Time,
		Body:           &ent.Message,
		SeverityNumber: &otelSeverity,
		SeverityText:   &severityText,
		Attributes:     &otelEncoder.OtelAttributes,
	}
	logRecord := logs.NewLogRecord(lrc)
	o.logger.Emit(logRecord)

	return nil
}

func convertZapLevelToOtelSeverity(zapLevel zapcore.Level) (logs.SeverityNumber, string) {

	// zap levels seems to be int8 and can be negative!
	// it appears that the debug level is -1, and 0 should not be used for otel severity
	// as stated in the docs of the otel enum.
	// thus I will add 2 and hope for the best
	//
	// TODO: Otel seems to be using different levels of severities which we can map.
	// for example - zapcore.WarnLevel can be mapped to otel's `WARN SeverityNumber = 13`.
	// but - is it always true? what happens when zap is triggered by logr with something like
	// `log.V(2).Info("message")`?
	// We should check these cases and possibly use more accurate mapping.

	switch zapLevel {
	case zapcore.DebugLevel:
		return 5, "DEBUG" // SeverityNumberDebug
	case zapcore.InfoLevel:
		return 9, "INFO" // SeverityNumberInfo
	case zapcore.WarnLevel:
		return 13, "WARN" // SeverityNumberWarn
	case zapcore.ErrorLevel:
		return 17, "ERROR" // SeverityNumberError
	case zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel:
		return 21, "FATAL" // SeverityNumberFatal
	default:
		return 9, "INFO" // SeverityNumberUnspecified
	}
}
