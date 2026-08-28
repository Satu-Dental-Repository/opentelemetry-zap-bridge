package bridge

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// TestOtelZapCoreSync is the regression for a latent nil dereference.
//
// OtelZapCore embeds zapcore.Core as an interface and never assigns it, so before Sync was
// implemented it was promoted to that nil interface — the ordinary `defer logger.Sync()` would have
// panicked. No dependent happened to call it, which is the only reason it went unnoticed, and is
// exactly why it is pinned here rather than left to the next caller to discover.
func TestOtelZapCoreSync(t *testing.T) {
	tests := []struct {
		name string
		core func() zapcore.Core
		// write says whether to emit a record before flushing. An empty buffer flushes trivially and
		// would prove nothing about the path a real caller takes, so it is true wherever it can be.
		write bool
	}{
		{
			name:  "a core built through the compatibility constructor syncs without panicking",
			core:  NewOtelZapCore,
			write: true,
		},
		{
			name: "a core built with a shutdown handle syncs without panicking",
			core: func() zapcore.Core {
				core, _ := NewOtelZapCoreWithShutdown()

				return core
			},
			write: true,
		},
		{
			// The zero value reaches Sync through any caller that constructs the exported type
			// directly, and a nil provider means nothing was ever buffered — so Sync must be inert.
			//
			// ⚠️ It is NOT written to, and that is a finding rather than a convenience: a zero-valued
			// core panics in Write on its nil logs.Logger. That is a second, pre-existing nil in this
			// type, separate from the Sync one this test pins, and it is deliberately left alone —
			// guarding Write would make a misconstructed core swallow every record silently, which is
			// worse than failing loudly. Recorded here so the next person does not rediscover it by
			// writing this same test case.
			name:  "a zero-valued core is inert on Sync rather than panicking",
			core:  func() zapcore.Core { return &OtelZapCore{} },
			write: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			core := test.core()

			if test.write {
				zap.New(core).Info("a record for the flush to carry")
			}

			// The RETURN VALUE is deliberately not asserted. With no collector reachable — which is
			// the case in CI — a flush legitimately reports "connection refused", and surfacing that
			// is correct behaviour rather than a defect. What is being pinned here is that the call
			// completes at all: before Sync existed it was promoted to a nil embedded interface and
			// took the process down.
			_ = core.Sync()
		})
	}
}

// TestNewOtelZapCoreWithShutdown covers the handle that exists so a process which EXITS can deliver
// what the batch processor is holding.
//
// The repeat-call case is not defensive tidiness: a caller whose cleanup runs on more than one path
// — a deferred shutdown plus an error path that shuts down early — must not have the second call
// turn a successful flush into a failure.
func TestNewOtelZapCoreWithShutdown(t *testing.T) {
	t.Run("shutdown completes and is safe to call twice", func(t *testing.T) {
		// A caller whose cleanup runs on more than one path — a deferred shutdown plus an early one
		// on an error path — must not have the second call take the process down.
		core, shutdown := NewOtelZapCoreWithShutdown()
		if shutdown == nil {
			t.Fatal("shutdown handle must not be nil — without it the provider is unreachable")
		}

		zap.New(core).Info("a record for shutdown to carry")

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Errors are tolerated for the same reason as in Sync: CI has no collector. Completion is
		// the assertion.
		_ = shutdown(ctx)
		_ = shutdown(ctx)
	})

	t.Run("the caller's cancelled context is honoured, not replaced", func(t *testing.T) {
		// This is the whole reason the handle takes a context. A process that budgets one deadline
		// per telemetry signal — so a dead collector cannot spend another signal's budget — has to be
		// able to bound this one too, and an internal timeout would silently ignore that.
		core, shutdown := NewOtelZapCoreWithShutdown()
		zap.New(core).Info("a record that will not get out")

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := shutdown(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("shutdown returned %v, wanted context.Canceled — the caller's deadline must win", err)
		}
	})
}

// TestNewOtelZapCoreStillSatisfiesTheOldContract pins backward compatibility explicitly.
//
// This library is a SHARED dependency — Satu-Dental-CMS-BE and Dua-Lab-BE both import it — so the
// pre-existing constructors have to keep working untouched. Adding the handle must be additive, and
// this is what says so.
func TestNewOtelZapCoreStillSatisfiesTheOldContract(t *testing.T) {
	t.Run("NewOtelZapCore returns a usable core with no arguments", func(t *testing.T) {
		core := NewOtelZapCore()
		if core == nil {
			t.Fatal("NewOtelZapCore returned nil")
		}

		zap.New(core).Info("written through the compatibility constructor")
	})

	t.Run("AttachToZapLogger tees rather than replacing", func(t *testing.T) {
		t.Setenv(otelSdkDisabled, "false")

		base := zap.NewNop()
		attached := AttachToZapLogger(base)

		if attached == base {
			t.Fatal("AttachToZapLogger must wrap the logger when the SDK is enabled")
		}

		attached.Info("written through the tee")
	})

	t.Run("AttachToZapLogger returns the logger untouched when the SDK is disabled", func(t *testing.T) {
		t.Setenv(otelSdkDisabled, "true")

		base := zap.NewNop()
		if attached := AttachToZapLogger(base); attached != base {
			t.Fatal("OTEL_SDK_DISABLED=true must leave the logger exactly as it was")
		}
	})
}
