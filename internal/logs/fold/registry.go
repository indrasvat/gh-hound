package fold

import (
	"fmt"
	"sort"
	"strings"
)

// Registries let new strategies plug in without touching the engine or
// callers (open/closed): register a constructor, select it by name.
var (
	normalizers = map[string]func() Normalizer{
		"shape": func() Normalizer { return Shape{} },
	}
	detectors = map[string]func() Detector{
		"consecutive": func() Detector { return Consecutive{} },
		"cycle":       func() Detector { return Cycle{} },
		"set":         func() Detector { return NewSet() },
	}
	guards = map[string]func() Guard{
		"structural": func() Guard { return Structural{} },
	}
	refiners = map[string]func() Refiner{
		"outlier": func() Refiner { return NewOutlier() },
	}
)

func RegisterNormalizer(name string, f func() Normalizer) { normalizers[name] = f }
func RegisterDetector(name string, f func() Detector)     { detectors[name] = f }
func RegisterGuard(name string, f func() Guard)           { guards[name] = f }
func RegisterRefiner(name string, f func() Refiner)       { refiners[name] = f }

func NormalizerNames() []string { return keys(normalizers) }
func DetectorNames() []string   { return keys(detectors) }
func GuardNames() []string      { return append(keys(guards), keys(refiners)...) }

// Build resolves names into an Engine. guardList is a comma separated mix
// of guard and refiner names; empty or "none" means neither.
func Build(norm, det, guardList string, opts Options) (Engine, error) {
	nf, ok := normalizers[norm]
	if !ok {
		return Engine{}, fmt.Errorf("unknown normalizer %q (have %v)", norm, NormalizerNames())
	}
	df, ok := detectors[det]
	if !ok {
		return Engine{}, fmt.Errorf("unknown detector %q (have %v)", det, DetectorNames())
	}
	e := Engine{Normalizer: nf(), Detector: df(), Options: opts}
	if guardList != "" && guardList != "none" {
		for g := range strings.SplitSeq(guardList, ",") {
			if rf, ok := refiners[g]; ok {
				e.Refiners = append(e.Refiners, rf())
				continue
			}
			gf, ok := guards[g]
			if !ok {
				return Engine{}, fmt.Errorf("unknown guard %q (have %v)", g, GuardNames())
			}
			e.Guards = append(e.Guards, gf())
		}
	}
	return e, nil
}

func keys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
