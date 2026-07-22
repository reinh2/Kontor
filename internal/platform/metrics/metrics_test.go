package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestCounterVecExposition(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounterVec("reqs_total", "Total requests.", "method", "code")
	c.With("GET", "200").Inc()
	c.With("GET", "200").Inc()
	c.With("POST", "500").Add(3)

	got := r.Gather()
	for _, want := range []string{
		"# HELP reqs_total Total requests.",
		"# TYPE reqs_total counter",
		`reqs_total{method="GET",code="200"} 2`,
		`reqs_total{method="POST",code="500"} 3`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("exposition missing %q in:\n%s", want, got)
		}
	}
	// Children render in sorted-key order: GET before POST.
	if strings.Index(got, `method="GET"`) > strings.Index(got, `method="POST"`) {
		t.Fatalf("children not in deterministic order:\n%s", got)
	}
}

func TestGaugeSetIncDec(t *testing.T) {
	r := NewRegistry()
	g := r.NewGauge("in_flight", "In-flight requests.")
	g.Set(5)
	g.Inc()
	g.Dec()
	g.Dec()
	if !strings.Contains(r.Gather(), "in_flight 4") {
		t.Fatalf("gauge value wrong:\n%s", r.Gather())
	}
}

func TestHistogramBucketsSumCount(t *testing.T) {
	r := NewRegistry()
	h := r.NewHistogramVec("lat", "Latency.", []float64{0.1, 0.5, 1}, "method")
	obs := h.With("GET")
	obs.Observe(0.05) // le=0.1
	obs.Observe(0.2)  // le=0.5
	obs.Observe(2.0)  // +Inf only

	got := r.Gather()
	for _, want := range []string{
		"# TYPE lat histogram",
		`lat_bucket{method="GET",le="0.1"} 1`,
		`lat_bucket{method="GET",le="0.5"} 2`,
		`lat_bucket{method="GET",le="1"} 2`,
		`lat_bucket{method="GET",le="+Inf"} 3`,
		`lat_sum{method="GET"} 2.25`,
		`lat_count{method="GET"} 3`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("histogram exposition missing %q in:\n%s", want, got)
		}
	}
}

func TestHistogramDefaultBucketsWhenNil(t *testing.T) {
	r := NewRegistry()
	h := r.NewHistogramVec("d", "Default buckets.", nil, "method")
	h.With("GET").Observe(0.003)
	got := r.Gather()
	// The smallest default bucket is 0.005; 0.003 lands in it.
	if !strings.Contains(got, `d_bucket{method="GET",le="0.005"} 1`) {
		t.Fatalf("default buckets not applied:\n%s", got)
	}
}

func TestLabelValueEscaping(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounterVec("escaped", "Escaping.", "path")
	c.With(`a"b\c` + "\n" + "d").Inc()
	got := r.Gather()
	if !strings.Contains(got, `escaped{path="a\"b\\c\nd"} 1`) {
		t.Fatalf("label value not escaped:\n%s", got)
	}
}

func TestHelpTextEscaping(t *testing.T) {
	r := NewRegistry()
	r.NewCounter("h", "line one\nline\\two")
	got := r.Gather()
	if !strings.Contains(got, `# HELP h line one\nline\\two`) {
		t.Fatalf("help text not escaped:\n%s", got)
	}
}

func TestHandlerContentTypeAndNoStore(t *testing.T) {
	r := NewRegistry()
	r.NewCounter("hits", "Hits.").Inc()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain; version=0.0.4") {
		t.Fatalf("content-type = %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("cache-control = %q", cc)
	}
	if !strings.Contains(rec.Body.String(), "hits 1") {
		t.Fatalf("body missing sample:\n%s", rec.Body.String())
	}
}

func TestDuplicateRegistrationPanics(t *testing.T) {
	r := NewRegistry()
	r.NewCounter("dup", "First.")
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	r.NewGauge("dup", "Second.")
}

func TestLabelCardinalityMismatchPanics(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounterVec("card", "Cardinality.", "a", "b")
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on cardinality mismatch")
		}
	}()
	c.With("only-one")
}

func TestRegisterProcessInfo(t *testing.T) {
	r := NewRegistry()
	RegisterProcessInfo(r, "1.2.3")
	got := r.Gather()
	if !strings.Contains(got, `kontor_build_info{version="1.2.3",go_version=`) {
		t.Fatalf("build info missing:\n%s", got)
	}
	if !strings.Contains(got, "kontor_process_start_time_seconds ") {
		t.Fatalf("start time missing:\n%s", got)
	}
}

func TestConcurrentCounterAndHistogram(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounterVec("c", "Concurrent counter.", "k")
	h := r.NewHistogramVec("hh", "Concurrent histogram.", []float64{1, 2}, "k")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.With("x").Inc()
				h.With("x").Observe(1.5)
			}
		}()
	}
	wg.Wait()

	got := r.Gather()
	if !strings.Contains(got, `c{k="x"} 5000`) {
		t.Fatalf("concurrent counter lost updates:\n%s", got)
	}
	if !strings.Contains(got, `hh_count{k="x"} 5000`) {
		t.Fatalf("concurrent histogram lost updates:\n%s", got)
	}
}
