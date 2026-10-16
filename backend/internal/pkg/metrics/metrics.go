// Package metrics is PLT-19's own "dashboard สุขภาพระบบ" data source: cheap, Redis-backed per-minute
// request counters (no Prometheus/Grafana/Loki in this environment — neither is reachable to build or
// verify a real stack against, so this gives the literal acceptance criterion — a health dashboard and
// a threshold alert — without them). Every bucket key carries a short TTL so the dataset never grows
// unbounded; this is deliberately coarse (per-minute sums, not histograms) since it only needs to answer
// "is the error rate or latency over its threshold right now", not render a percentile curve.
package metrics

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const bucketTTL = 2 * time.Hour

func bucketKey(service string, t time.Time) string {
	return "metrics:" + service + ":" + t.UTC().Format("200601021504")
}

// Record adds one HTTP response's outcome for service (e.g. "api") to the current minute's bucket.
// slow is whatever duration this caller considers slow enough to count separately (the health package
// decides the actual threshold; Record just counts against whatever it's given).
func Record(ctx context.Context, rdb *redis.Client, service string, status int, dur time.Duration, slow time.Duration) {
	if rdb == nil {
		return
	}
	key := bucketKey(service, time.Now())
	pipe := rdb.TxPipeline()
	pipe.HIncrBy(ctx, key, "count", 1)
	if status >= 500 {
		pipe.HIncrBy(ctx, key, "errors", 1)
	}
	pipe.HIncrBy(ctx, key, "duration_ms", dur.Milliseconds())
	if dur >= slow {
		pipe.HIncrBy(ctx, key, "slow", 1)
	}
	pipe.Expire(ctx, key, bucketTTL)
	_, _ = pipe.Exec(ctx) // best-effort: a lost metrics write never fails the real request
}

// Bucket is one minute's raw counters.
type Bucket struct {
	Count      int64
	Errors     int64
	DurationMs int64
	Slow       int64
}

// Window is Count/Errors/DurationMs/Slow already summed over several minutes, with the derived rates
// a dashboard actually wants.
type Window struct {
	Service      string
	Minutes      int
	Count        int64
	Errors       int64
	ErrorRate    float64 // 0 when Count is 0 — nothing to be unhealthy about yet
	AvgLatencyMs float64
	Slow         int64
	SlowRatio    float64
}

// Snapshot sums the last `minutes` whole-minute buckets for service, ending at the current minute, and
// derives the dashboard's own rates from the sums.
func Snapshot(ctx context.Context, rdb *redis.Client, service string, minutes int) (Window, error) {
	w := Window{Service: service, Minutes: minutes}
	if rdb == nil || minutes <= 0 {
		return w, nil
	}
	var totalDurationMs int64
	now := time.Now().UTC()
	for i := 0; i < minutes; i++ {
		key := bucketKey(service, now.Add(-time.Duration(i)*time.Minute))
		vals, err := rdb.HGetAll(ctx, key).Result()
		if err != nil {
			return Window{}, err
		}
		w.Count += atoi64(vals["count"])
		w.Errors += atoi64(vals["errors"])
		totalDurationMs += atoi64(vals["duration_ms"])
		w.Slow += atoi64(vals["slow"])
	}
	if w.Count > 0 {
		w.ErrorRate = float64(w.Errors) / float64(w.Count)
		w.AvgLatencyMs = float64(totalDurationMs) / float64(w.Count)
		w.SlowRatio = float64(w.Slow) / float64(w.Count)
	}
	return w, nil
}

func atoi64(s string) int64 {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int64(c-'0')
	}
	return n
}
