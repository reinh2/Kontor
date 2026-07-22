// Package metrics is a small, dependency-free metrics registry that renders the
// Prometheus text exposition format (version 0.0.4). It deliberately avoids the
// upstream client_golang dependency: Kontor keeps its runtime dependencies
// minimal (see go.mod), and the handful of counters, gauges, and histograms the
// service needs are cheaper to own than to import.
//
// The registry is safe for concurrent use. Metrics are registered once at
// startup and then updated from request-handling goroutines; scrapes read a
// consistent snapshot per metric via atomic loads or a per-histogram lock.
package metrics

import (
	"math"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DefBuckets are the default latency histogram upper bounds in seconds. They
// mirror the widely used Prometheus client defaults so existing dashboards and
// alert rules work without retuning.
var DefBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// labelSeparator joins label values into a child key. 0xff never appears in the
// low-cardinality label values Kontor emits (HTTP methods, status codes,
// versions), so it cannot collide.
const labelSeparator = "\xff"

// collector renders one metric family (HELP, TYPE, and its samples).
type collector interface {
	render(b *strings.Builder)
}

// Registry holds registered metric families and renders them for scraping.
type Registry struct {
	mu         sync.Mutex
	collectors []collector
	names      map[string]struct{}
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{names: make(map[string]struct{})}
}

func (r *Registry) register(name string, c collector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.names[name]; exists {
		panic("metrics: duplicate registration of " + name)
	}
	r.names[name] = struct{}{}
	r.collectors = append(r.collectors, c)
}

// Gather renders every registered metric family in registration order.
func (r *Registry) Gather() string {
	var b strings.Builder
	r.mu.Lock()
	collectors := r.collectors
	r.mu.Unlock()
	for _, c := range collectors {
		c.render(&b)
	}
	return b.String()
}

// Handler serves the exposition over HTTP. It never caches and does not set
// permissive CORS: scraping is an internal, network-restricted concern.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(r.Gather()))
	})
}

// NewCounter registers and returns an unlabeled monotonic counter.
func (r *Registry) NewCounter(name, help string) *Counter {
	c := &Counter{}
	r.register(name, &counterFamily{name: name, help: help, single: c})
	return c
}

// NewCounterVec registers and returns a counter partitioned by labelNames.
func (r *Registry) NewCounterVec(name, help string, labelNames ...string) *CounterVec {
	v := &CounterVec{name: name, help: help, labelNames: labelNames, children: map[string]*counterChild{}}
	r.register(name, v)
	return v
}

// NewGauge registers and returns an unlabeled gauge.
func (r *Registry) NewGauge(name, help string) *Gauge {
	g := &Gauge{}
	r.register(name, &gaugeFamily{name: name, help: help, single: g})
	return g
}

// NewGaugeVec registers and returns a gauge partitioned by labelNames.
func (r *Registry) NewGaugeVec(name, help string, labelNames ...string) *GaugeVec {
	v := &GaugeVec{name: name, help: help, labelNames: labelNames, children: map[string]*gaugeChild{}}
	r.register(name, v)
	return v
}

// NewHistogramVec registers and returns a histogram partitioned by labelNames.
// A nil or empty buckets slice uses DefBuckets. Buckets must be sorted ascending.
func (r *Registry) NewHistogramVec(name, help string, buckets []float64, labelNames ...string) *HistogramVec {
	if len(buckets) == 0 {
		buckets = DefBuckets
	}
	upper := append([]float64(nil), buckets...)
	sort.Float64s(upper)
	v := &HistogramVec{name: name, help: help, labelNames: labelNames, upper: upper, children: map[string]*Histogram{}}
	r.register(name, v)
	return v
}

// RegisterProcessInfo adds kontor_build_info and kontor_process_start_time_seconds.
// version is the build version string ("dev" when unset via -ldflags).
func RegisterProcessInfo(r *Registry, version string) {
	if version == "" {
		version = "dev"
	}
	info := r.NewGaugeVec("kontor_build_info", "Build and runtime information; always 1.", "version", "go_version")
	info.With(version, runtime.Version()).Set(1)
	start := r.NewGauge("kontor_process_start_time_seconds", "Start time of the process since the Unix epoch, in seconds.")
	start.Set(float64(time.Now().Unix()))
}

// Counter is a monotonically increasing float64 value.
type Counter struct {
	bits atomic.Uint64
}

// Inc adds one.
func (c *Counter) Inc() { c.Add(1) }

// Add increases the counter by delta, which must not be negative.
func (c *Counter) Add(delta float64) {
	if delta < 0 {
		panic("metrics: counter cannot decrease")
	}
	for {
		old := c.bits.Load()
		next := math.Float64frombits(old) + delta
		if c.bits.CompareAndSwap(old, math.Float64bits(next)) {
			return
		}
	}
}

func (c *Counter) value() float64 { return math.Float64frombits(c.bits.Load()) }

// Gauge is a float64 value that can go up or down.
type Gauge struct {
	bits atomic.Uint64
}

// Set replaces the gauge value.
func (g *Gauge) Set(v float64) { g.bits.Store(math.Float64bits(v)) }

// Inc adds one.
func (g *Gauge) Inc() { g.Add(1) }

// Dec subtracts one.
func (g *Gauge) Dec() { g.Add(-1) }

// Add adds delta (which may be negative).
func (g *Gauge) Add(delta float64) {
	for {
		old := g.bits.Load()
		next := math.Float64frombits(old) + delta
		if g.bits.CompareAndSwap(old, math.Float64bits(next)) {
			return
		}
	}
}

func (g *Gauge) value() float64 { return math.Float64frombits(g.bits.Load()) }

// Histogram counts observations into cumulative buckets and tracks sum and count.
type Histogram struct {
	upper       []float64
	labelValues []string

	mu     sync.Mutex
	counts []uint64 // per-bucket (non-cumulative) counts for finite bounds
	sum    float64
	count  uint64
}

// Observe records one value (typically a duration in seconds).
func (h *Histogram) Observe(v float64) {
	i := sort.SearchFloat64s(h.upper, v)
	h.mu.Lock()
	if i < len(h.counts) {
		h.counts[i]++
	}
	h.sum += v
	h.count++
	h.mu.Unlock()
}

// CounterVec is a set of counters sharing a name and label schema.
type CounterVec struct {
	name, help string
	labelNames []string

	mu       sync.RWMutex
	children map[string]*counterChild
}

type counterChild struct {
	labelValues []string
	counter     Counter
}

// With returns the counter for the given label values, creating it on first use.
// The number of values must equal the number of label names.
func (v *CounterVec) With(values ...string) *Counter {
	if len(values) != len(v.labelNames) {
		panic("metrics: label cardinality mismatch for " + v.name)
	}
	key := strings.Join(values, labelSeparator)
	v.mu.RLock()
	child, ok := v.children[key]
	v.mu.RUnlock()
	if ok {
		return &child.counter
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if child, ok = v.children[key]; ok {
		return &child.counter
	}
	child = &counterChild{labelValues: append([]string(nil), values...)}
	v.children[key] = child
	return &child.counter
}

func (v *CounterVec) render(b *strings.Builder) {
	writeHelpType(b, v.name, v.help, "counter")
	v.mu.RLock()
	defer v.mu.RUnlock()
	for _, key := range sortedKeys(v.children) {
		child := v.children[key]
		writeSample(b, v.name, v.labelNames, child.labelValues, child.counter.value())
	}
}

// GaugeVec is a set of gauges sharing a name and label schema.
type GaugeVec struct {
	name, help string
	labelNames []string

	mu       sync.RWMutex
	children map[string]*gaugeChild
}

type gaugeChild struct {
	labelValues []string
	gauge       Gauge
}

// With returns the gauge for the given label values, creating it on first use.
func (v *GaugeVec) With(values ...string) *Gauge {
	if len(values) != len(v.labelNames) {
		panic("metrics: label cardinality mismatch for " + v.name)
	}
	key := strings.Join(values, labelSeparator)
	v.mu.RLock()
	child, ok := v.children[key]
	v.mu.RUnlock()
	if ok {
		return &child.gauge
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if child, ok = v.children[key]; ok {
		return &child.gauge
	}
	child = &gaugeChild{labelValues: append([]string(nil), values...)}
	v.children[key] = child
	return &child.gauge
}

func (v *GaugeVec) render(b *strings.Builder) {
	writeHelpType(b, v.name, v.help, "gauge")
	v.mu.RLock()
	defer v.mu.RUnlock()
	for _, key := range sortedKeys(v.children) {
		child := v.children[key]
		writeSample(b, v.name, v.labelNames, child.labelValues, child.gauge.value())
	}
}

// HistogramVec is a set of histograms sharing a name, bucket layout, and schema.
type HistogramVec struct {
	name, help string
	labelNames []string
	upper      []float64

	mu       sync.RWMutex
	children map[string]*Histogram
}

// With returns the histogram for the given label values, creating it on first use.
func (v *HistogramVec) With(values ...string) *Histogram {
	if len(values) != len(v.labelNames) {
		panic("metrics: label cardinality mismatch for " + v.name)
	}
	key := strings.Join(values, labelSeparator)
	v.mu.RLock()
	child, ok := v.children[key]
	v.mu.RUnlock()
	if ok {
		return child
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if child, ok = v.children[key]; ok {
		return child
	}
	child = &Histogram{
		upper:       v.upper,
		labelValues: append([]string(nil), values...),
		counts:      make([]uint64, len(v.upper)),
	}
	v.children[key] = child
	return child
}

func (v *HistogramVec) render(b *strings.Builder) {
	writeHelpType(b, v.name, v.help, "histogram")
	v.mu.RLock()
	defer v.mu.RUnlock()
	for _, key := range sortedKeys(v.children) {
		v.children[key].render(b, v.name, v.labelNames)
	}
}

func (h *Histogram) render(b *strings.Builder, name string, labelNames []string) {
	h.mu.Lock()
	counts := append([]uint64(nil), h.counts...)
	sum := h.sum
	total := h.count
	h.mu.Unlock()

	bucketNames := append(append([]string(nil), labelNames...), "le")
	cumulative := uint64(0)
	for i, bound := range h.upper {
		cumulative += counts[i]
		values := append(append([]string(nil), h.labelValues...), formatFloat(bound))
		writeSample(b, name+"_bucket", bucketNames, values, float64(cumulative))
	}
	infValues := append(append([]string(nil), h.labelValues...), "+Inf")
	writeSample(b, name+"_bucket", bucketNames, infValues, float64(total))
	writeSample(b, name+"_sum", labelNames, h.labelValues, sum)
	writeSample(b, name+"_count", labelNames, h.labelValues, float64(total))
}

// counterFamily and gaugeFamily render a single unlabeled metric.
type counterFamily struct {
	name, help string
	single     *Counter
}

func (f *counterFamily) render(b *strings.Builder) {
	writeHelpType(b, f.name, f.help, "counter")
	writeSample(b, f.name, nil, nil, f.single.value())
}

type gaugeFamily struct {
	name, help string
	single     *Gauge
}

func (f *gaugeFamily) render(b *strings.Builder) {
	writeHelpType(b, f.name, f.help, "gauge")
	writeSample(b, f.name, nil, nil, f.single.value())
}

func writeHelpType(b *strings.Builder, name, help, typ string) {
	b.WriteString("# HELP ")
	b.WriteString(name)
	b.WriteByte(' ')
	writeEscapedHelp(b, help)
	b.WriteByte('\n')
	b.WriteString("# TYPE ")
	b.WriteString(name)
	b.WriteByte(' ')
	b.WriteString(typ)
	b.WriteByte('\n')
}

func writeSample(b *strings.Builder, name string, labelNames, labelValues []string, value float64) {
	b.WriteString(name)
	if len(labelNames) > 0 {
		b.WriteByte('{')
		for i := range labelNames {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(labelNames[i])
			b.WriteString(`="`)
			writeEscapedLabelValue(b, labelValues[i])
			b.WriteByte('"')
		}
		b.WriteByte('}')
	}
	b.WriteByte(' ')
	b.WriteString(formatFloat(value))
	b.WriteByte('\n')
}

func writeEscapedHelp(b *strings.Builder, s string) {
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
}

func writeEscapedLabelValue(b *strings.Builder, s string) {
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
