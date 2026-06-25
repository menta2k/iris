package errlog

import (
	"context"
	"log/slog"
	"time"
)

const redactedValue = "[REDACTED]"

// Handler passes every record to a base slog.Handler (e.g. the JSON stdout
// handler) and additionally mirrors Warn/Error records into a Sink. It is safe
// for concurrent use by multiple goroutines.
type Handler struct {
	base  slog.Handler
	async *asyncSink
	opts  Options
	attrs []slog.Attr
	group string // active group prefix ("" when none); workers rarely nest groups
}

// New constructs a Handler wrapping base and an asynchronous writer to sink.
// Call Run to drain captured entries until its context is cancelled.
func New(base slog.Handler, sink Sink, opts Options) *Handler {
	if opts.BufferSize <= 0 {
		opts.BufferSize = 1024
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 64
	}
	if opts.FlushInterval <= 0 {
		opts.FlushInterval = time.Second
	}
	return &Handler{
		base:  base,
		async: &asyncSink{sink: sink, ch: make(chan Entry, opts.BufferSize), opts: opts},
		opts:  opts,
	}
}

// Run drains the capture buffer into the sink until ctx is cancelled, then
// flushes once more. Intended to be started as a background worker; it returns
// ctx.Err() on exit to fit the worker runner signature.
func (h *Handler) Run(ctx context.Context) error {
	h.async.run(ctx)
	return ctx.Err()
}

// Dropped returns the number of entries discarded because the buffer was full.
func (h *Handler) Dropped() uint64 { return h.async.dropped.Load() }

// Enabled captures Warn and above even when the base handler is set to a higher
// level, so error-log coverage never silently depends on stdout verbosity.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= slog.LevelWarn || h.base.Enabled(ctx, level)
}

// Handle forwards the record to the base handler (when it accepts the level) and
// mirrors Warn/Error records to the sink.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	var err error
	if h.base.Enabled(ctx, r.Level) {
		err = h.base.Handle(ctx, r)
	}
	if r.Level >= slog.LevelWarn {
		h.async.enqueue(h.entry(r))
	}
	return err
}

// WithAttrs returns a handler that also carries attrs. The base handler and the
// captured-entry attrs are both extended so the two outputs stay consistent.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	nh := h.clone()
	nh.base = h.base.WithAttrs(attrs)
	nh.attrs = append(append(make([]slog.Attr, 0, len(h.attrs)+len(attrs)), h.attrs...), attrs...)
	return nh
}

// WithGroup returns a handler that nests subsequent attrs under name.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	nh := h.clone()
	nh.base = h.base.WithGroup(name)
	if h.group == "" {
		nh.group = name
	} else {
		nh.group = h.group + "." + name
	}
	return nh
}

func (h *Handler) clone() *Handler {
	c := *h
	return &c
}

// entry builds the durable Entry from a record plus the handler's accumulated
// attrs. The "worker" attribute is lifted into its own column; everything else
// (redacted) goes into the detail map.
func (h *Handler) entry(r slog.Record) Entry {
	detail := make(map[string]any)
	worker := ""
	add := func(a slog.Attr) {
		if a.Key == "worker" && worker == "" && h.group == "" {
			if s := a.Value.Resolve().String(); s != "" {
				worker = s
				return
			}
		}
		detail[h.qualify(a.Key)] = h.value(a.Key, a.Value)
	}
	for _, a := range h.attrs {
		add(a)
	}
	r.Attrs(func(a slog.Attr) bool {
		add(a)
		return true
	})

	level := "error"
	if r.Level < slog.LevelError {
		level = "warn"
	}
	t := r.Time
	if t.IsZero() {
		t = time.Now()
	}
	return Entry{
		Time:    t.UTC(),
		Level:   level,
		Worker:  worker,
		Message: r.Message,
		Detail:  detail,
	}
}

func (h *Handler) qualify(key string) string {
	if h.group == "" {
		return key
	}
	return h.group + "." + key
}

// value converts a slog value to a JSON-friendly any, redacting sensitive keys
// and flattening groups and error values.
func (h *Handler) value(key string, v slog.Value) any {
	if h.opts.Redact != nil && h.opts.Redact(key) {
		return redactedValue
	}
	v = v.Resolve()
	switch v.Kind() {
	case slog.KindGroup:
		members := v.Group()
		m := make(map[string]any, len(members))
		for _, ga := range members {
			m[ga.Key] = h.value(ga.Key, ga.Value)
		}
		return m
	case slog.KindAny:
		a := v.Any()
		if err, ok := a.(error); ok {
			return err.Error()
		}
		return a
	default:
		return v.Any()
	}
}
