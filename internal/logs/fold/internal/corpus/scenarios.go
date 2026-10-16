package corpus

import (
	"fmt"
	"strings"
	"time"
)

// Scenario builds one labelled log from a seed.
type Scenario struct {
	Name  string
	Class string
	Note  string
	Build func(g *b)
}

// All returns every scenario plus decorated variants (runner timestamps,
// ANSI color) of the ones where decoration changes the problem.
func All(seed uint64) []*Log {
	var out []*Log
	for i, s := range scenarios {
		g := newB(s.Name, s.Class, s.Note, seed+uint64(i)*1000)
		s.Build(g)
		out = append(out, g.log)
		out = append(out, GHAStamp(g.log, seed+uint64(i)))
		if strings.Contains(s.Note, "+ansi") {
			out = append(out, ANSI(g.log))
		}
	}
	return out
}

var scenarios = []Scenario{
	{"exact-spam", "foldable", "identical line x500 (the uniq case)", func(g *b) {
		g.prose(20, Context)
		g.repeat(500, func(int) []string { return []string{"Waiting for server to start..."} })
		g.add(Signal, "Server started on :8080")
		g.prose(20, Context)
	}},
	{"ts-poll", "foldable", "one line, fresh timestamp each time +ansi", func(g *b) {
		g.prose(30, Context)
		g.repeat(800, func(int) []string {
			return []string{fmt.Sprintf("INFO [%s] waiting for deployment to be ready", g.tick(5*time.Second).Format("2006-01-02 15:04:05"))}
		})
		g.add(Signal, "ERROR [%s] deployment did not become ready within 1h", g.clock.Format("2006-01-02 15:04:05"))
	}},
	{"poll-pair-pointers", "foldable", "2-line cycle, ts + 3 hex pointers vary per line +ansi", func(g *b) {
		g.prose(40, Context)
		g.repeat(700, func(int) []string {
			t1 := g.tick(9 * time.Second).Format("2006-01-02 15:04:05")
			t2 := g.tick(1 * time.Second).Format("2006-01-02 15:04:05")
			return []string{
				fmt.Sprintf("INFO[%s] checking if service is running             name=svc-lifecycle scenario=lifecycle", t1),
				fmt.Sprintf("INFO[%s] state :: &{  0x%s 0x%s 0x%s}  name=svc-lifecycle scenario=lifecycle", t2, g.hex(12), g.hex(12), g.hex(12)),
			}
		})
		g.add(Signal, "FAIL: scenario lifecycle: timed out after 2h waiting for service")
		g.prose(15, Context)
	}},
	{"retry-block", "foldable", "5-line retry block with counter, IP, status, backoff", func(g *b) {
		g.prose(25, Context)
		ip := g.ip()
		g.repeat(60, func(c int) []string {
			return []string{
				fmt.Sprintf("attempt %d/60: GET http://%s:8080/healthz", c+1, ip),
				"  -> HTTP 503 Service Unavailable",
				fmt.Sprintf("  response time: %dms", 5+g.rng.IntN(300)),
				fmt.Sprintf("  request-id: %s", g.uuid()),
				fmt.Sprintf("  sleeping %ds before retry", 1+c%5),
			}
		})
		g.add(Signal, "giving up after 60 attempts")
	}},
	{"k8s-rollout", "foldable", "same line, counter moves 1->2->3 of 3", func(g *b) {
		g.prose(20, Context)
		for _, n := range []int{0, 1, 2} {
			g.repeat(120+g.rng.IntN(80), func(int) []string {
				return []string{fmt.Sprintf(`Waiting for deployment "api" rollout to finish: %d of 3 updated replicas are available...`, n)}
			})
		}
		g.add(Signal, `deployment "api" successfully rolled out`)
	}},
	{"stacktrace-repeat", "foldable", "12-line exception repeated (period > default MaxPeriod)", func(g *b) {
		g.prose(30, Context)
		g.repeat(40, func(int) []string {
			lines := []string{fmt.Sprintf("%s WARN [pool-%d] Connection refused, retrying", g.tick(3*time.Second).Format(time.RFC3339), 1+g.rng.IntN(9))}
			lines = append(lines, "java.net.ConnectException: Connection refused")
			for _, f := range []string{"PlainSocketImpl.socketConnect", "AbstractPlainSocketImpl.doConnect", "SocksSocketImpl.connect", "Socket.connect", "HttpClient.openServer", "HttpClient.<init>", "HttpURLConnection.connect", "HealthProbe.check", "Retrier.run"} {
				lines = append(lines, fmt.Sprintf("\tat %s(%s.java:%d)", f, strings.Split(f, ".")[0], 100+len(f)*7))
			}
			lines = append(lines, "\t... 12 more")
			return lines
		})
		g.add(Signal, "BUILD FAILED in 2m 1s")
	}},
	{"progress-per-file", "foldable", "percent progress per artifact; names differ, so runs are per file", func(g *b) {
		for range 6 {
			name := fmt.Sprintf("%s-%s.tar.gz", g.word(), g.word())
			g.add(Signal, "Fetching %s", name)
			total := 20 + g.rng.IntN(200)
			g.repeat(100, func(c int) []string {
				return []string{fmt.Sprintf("  %s: %d%% (%.1fMB/%dMB)", name, c, float64(total*c)/100, total)}
			})
			g.add(Signal, "  %s: done", name)
		}
	}},
	{"json-heartbeat", "foldable", "structured JSON logs, seq/uptime/ts vary", func(g *b) {
		g.prose(10, Context)
		g.repeat(600, func(c int) []string {
			return []string{fmt.Sprintf(`{"ts":"%s","level":"info","msg":"heartbeat","seq":%d,"uptime_ms":%d,"worker":"w-%s"}`,
				g.tick(time.Second).Format(time.RFC3339Nano), c, c*1000+g.rng.IntN(999), g.hex(6))}
		})
		g.add(Signal, `{"ts":"%s","level":"error","msg":"lost lease","worker":"w-0"}`, g.clock.Format(time.RFC3339Nano))
	}},
	{"spinner-frames", "foldable", "spinner glyph cycles with period 10 (literal, not masked)", func(g *b) {
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		g.prose(10, Context)
		g.repeat(30, func(int) []string {
			out := make([]string, len(frames))
			for i, f := range frames {
				out[i] = f + " Building bundle..."
			}
			return out
		})
		g.add(Signal, "✔ Bundle built")
	}},
	{"nested-repeat", "foldable", "[header, inner x5, footer] x30 (period 7)", func(g *b) {
		g.prose(10, Context)
		g.repeat(30, func(c int) []string {
			out := []string{fmt.Sprintf("--- shard %d: begin", c%3)}
			for range 5 {
				out = append(out, fmt.Sprintf("  probe ok in %dms", g.rng.IntN(50)))
			}
			return append(out, "--- shard: end")
		})
	}},
	{"blank-spam", "foldable", "runs of blank/whitespace lines", func(g *b) {
		for range 5 {
			g.prose(10, Context)
			g.repeat(40, func(c int) []string { return []string{strings.Repeat(" ", c%3)} })
		}
	}},
	{"jitter-cycle", "foldable", "A,B cycle broken every ~25 lines by a unique signal line", func(g *b) {
		for k := range 20 {
			g.repeat(10+g.rng.IntN(5), func(int) []string {
				return []string{
					fmt.Sprintf("[%s] reconcile: listing objects", g.tick(time.Second).Format("15:04:05")),
					fmt.Sprintf("[%s] reconcile: %d objects unchanged", g.tick(time.Second).Format("15:04:05"), 40+g.rng.IntN(3)),
				}
			})
			g.add(Signal, "[%s] reconcile: object %s-%d drifted, patching", g.clock.Format("15:04:05"), g.word(), k)
		}
	}},
	{"hidden-word-failure", "trap", "poll status=Pending; one status=Failed mid-run (word change)", func(g *b) {
		g.repeat(300, func(int) []string {
			return []string{fmt.Sprintf("%s pod/api-0 status=Pending", g.tick(2*time.Second).Format("15:04:05"))}
		})
		g.add(Signal, "%s pod/api-0 status=Failed reason=ImagePullBackOff", g.tick(2*time.Second).Format("15:04:05"))
		g.repeat(300, func(int) []string {
			return []string{fmt.Sprintf("%s pod/api-0 status=Pending", g.tick(2*time.Second).Format("15:04:05"))}
		})
	}},
	{"hidden-number-failure", "trap", "probe exit code 0 x200; one exit code 137 mid-run (number change)", func(g *b) {
		g.repeat(200, func(int) []string { return []string{"probe finished with exit code 0"} })
		g.add(Signal, "probe finished with exit code 137")
		g.repeat(200, func(int) []string { return []string{"probe finished with exit code 0"} })
	}},
	{"numbered-names", "negative", "distinct packages/specs that differ only by a number suffix", func(g *b) {
		for i := range 120 {
			g.add(Signal, "ok   github.com/acme/svc/shard-%d   %d.%03ds", i, g.rng.IntN(3), g.rng.IntN(999))
		}
	}},
	{"go-test-unique", "negative", "distinct test names; nothing should fold", func(g *b) {
		for i := range 300 {
			name := fmt.Sprintf("Test%s%s_%s", title(g.word()), title(g.word()), g.word())
			g.add(Context, "=== RUN   %s", name)
			g.add(Signal, "--- PASS: %s (0.%02ds)", name, i%100)
		}
	}},
	{"maven-downloads", "hard", "high-cardinality noise: distinct URLs, same template", func(g *b) {
		for range 400 {
			a, v := g.word()+"-"+g.word(), fmt.Sprintf("%d.%d.%d", g.rng.IntN(5), g.rng.IntN(20), g.rng.IntN(9))
			g.add(Context, "Downloading from central: https://repo.maven.apache.org/maven2/org/%s/%s/%s/%s-%s.pom", g.word(), a, v, a, v)
		}
		g.add(Signal, "[ERROR] Failed to execute goal: dependency convergence")
	}},
	{"docker-layers-interleaved", "hard", "parallel layer pulls interleaved randomly (not periodic)", func(g *b) {
		layers := make([]string, 12)
		for i := range layers {
			layers[i] = g.hex(12)
		}
		stage := make([]int, len(layers))
		msgs := []string{"Pulling fs layer", "Waiting", "Downloading", "Verifying Checksum", "Download complete", "Extracting", "Pull complete"}
		for {
			active := []int{}
			for i, s := range stage {
				if s < len(msgs) {
					active = append(active, i)
				}
			}
			if len(active) == 0 {
				break
			}
			i := active[g.rng.IntN(len(active))]
			if msgs[stage[i]] == "Downloading" && g.rng.IntN(6) != 0 {
				g.add(Noise, "%s: Downloading  %d.%dMB/%dMB", layers[i], g.rng.IntN(90), g.rng.IntN(9), 90)
				continue
			}
			g.add(Context, "%s: %s", layers[i], msgs[stage[i]])
			stage[i]++
		}
		g.add(Signal, "Status: Downloaded newer image for acme/api:latest")
	}},
	{"interleaved-pollers", "hard", "two workers polling with random interleave", func(g *b) {
		for range 800 {
			if g.rng.IntN(2) == 0 {
				g.add(Noise, "[worker-a] %s queue empty, sleeping", g.tick(time.Second).Format("15:04:05"))
			} else {
				g.add(Noise, "[worker-b] %s lease held by %s", g.tick(time.Second).Format("15:04:05"), g.uuid())
			}
		}
		g.add(Signal, "[worker-a] job 42 picked up")
	}},
	{"unique-prose", "negative", "5k unique lines: must fold ~nothing", func(g *b) {
		g.prose(5000, Signal)
	}},
	{"gha-groups-repeat", "foldable", "##[group] markers between repeated lines must stay", func(g *b) {
		for k := range 10 {
			g.add(Signal, "##[group]Run integration shard %d", k)
			g.repeat(50, func(int) []string {
				return []string{fmt.Sprintf("waiting for db (%s)", g.tick(time.Second).Format("15:04:05"))}
			})
			g.add(Signal, "##[endgroup]")
		}
	}},
}

func title(w string) string { return strings.ToUpper(w[:1]) + w[1:] }
