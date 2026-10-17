// Package corpus generates synthetic CI logs with ground truth for the fold
// quality tests. Every line is
// labelled:
//
//	N  noise   - part of a repeating block a good compactor should fold
//	S  signal  - must stay visible (distinct information)
//	C  context - neutral; folding or not is acceptable
//
// Blocks record where intended repeats are so bench can compute the best
// achievable fold for each scenario.
package corpus

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
)

type Label byte

const (
	Noise   Label = 'N'
	Signal  Label = 'S'
	Context Label = 'C'
)

type Block struct {
	Start, Period, Count int
}

// Log is one generated scenario.
type Log struct {
	Name   string
	Class  string // foldable | hard | negative | trap
	Note   string
	Lines  []string
	Labels []Label
	Blocks []Block
}

type b struct {
	log   *Log
	rng   *rand.Rand
	clock time.Time
}

func newB(name, class, note string, seed uint64) *b {
	return &b{
		log:   &Log{Name: name, Class: class, Note: note},
		rng:   rand.New(rand.NewPCG(seed, 0x5eed)),
		clock: time.Date(2026, 10, 16, 9, 58, 1, 0, time.UTC),
	}
}

func (g *b) add(l Label, format string, args ...any) {
	g.log.Lines = append(g.log.Lines, fmt.Sprintf(format, args...))
	g.log.Labels = append(g.log.Labels, l)
}

// repeat emits count cycles of a block; each cycle calls fn to get fresh
// (volatile) text for each line of the block.
func (g *b) repeat(count int, fn func(cycle int) []string) {
	start := len(g.log.Lines)
	period := 0
	for c := range count {
		lines := fn(c)
		period = len(lines)
		for _, l := range lines {
			g.add(Noise, "%s", l)
		}
	}
	g.log.Blocks = append(g.log.Blocks, Block{Start: start, Period: period, Count: count})
}

func (g *b) tick(d time.Duration) time.Time { g.clock = g.clock.Add(d); return g.clock }
func (g *b) hex(n int) string {
	const digits = "0123456789abcdef"
	out := make([]byte, n)
	for i := range out {
		out[i] = digits[g.rng.IntN(16)]
	}
	return string(out)
}
func (g *b) uuid() string {
	return g.hex(8) + "-" + g.hex(4) + "-" + g.hex(4) + "-" + g.hex(4) + "-" + g.hex(12)
}
func (g *b) ip() string {
	return fmt.Sprintf("10.%d.%d.%d", g.rng.IntN(255), g.rng.IntN(255), 1+g.rng.IntN(254))
}

var words = strings.Fields(`alpha bravo cache compile config deploy driver engine fetch graph
handler index kernel ledger module network object parser queue router schema socket
storage tenant token update vector worker yaml zone build linker plugin bundle asset`)

func (g *b) word() string { return words[g.rng.IntN(len(words))] }

// prose emits n unique-ish context lines (compiler/test chatter).
func (g *b) prose(n int, l Label) {
	for range n {
		switch g.rng.IntN(4) {
		case 0:
			g.add(l, "compiling %s/%s_%s.go", g.word(), g.word(), g.word())
		case 1:
			g.add(l, "  %s: resolved %s -> %s", g.word(), g.word(), g.word())
		case 2:
			g.add(l, "Step %s: %s the %s %s", g.word(), g.word(), g.word(), g.word())
		default:
			g.add(l, "[%s] %s %s configured for %s", g.word(), g.word(), g.word(), g.word())
		}
	}
}

// GHAStamp prefixes every line with an Actions runner timestamp that
// advances by a random sub-second step, like raw job logs.
func GHAStamp(l *Log, seed uint64) *Log {
	rng := rand.New(rand.NewPCG(seed, 7))
	t := time.Date(2026, 10, 16, 9, 58, 1, 0, time.UTC)
	out := *l
	out.Name = l.Name + "+gha"
	out.Lines = make([]string, len(l.Lines))
	for i, line := range l.Lines {
		t = t.Add(time.Duration(rng.IntN(900)) * time.Millisecond)
		out.Lines[i] = t.Format("2006-01-02T15:04:05.0000000Z") + " " + line
	}
	return &out
}

// ANSI wraps level-ish words in color escapes, like a TTY-aware logger.
func ANSI(l *Log) *Log {
	r := strings.NewReplacer(
		"INFO", "\x1b[36mINFO\x1b[0m", "WARN", "\x1b[33mWARN\x1b[0m",
		"ERROR", "\x1b[31mERROR\x1b[0m", "PASS", "\x1b[32mPASS\x1b[0m",
	)
	out := *l
	out.Name = l.Name + "+ansi"
	out.Lines = make([]string, len(l.Lines))
	for i, line := range l.Lines {
		out.Lines[i] = r.Replace(line)
	}
	return &out
}
