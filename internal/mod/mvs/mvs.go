// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package mvs implements Minimal Version Selection.
// See https://research.swtch.com/vgo-mvs.
package mvs

import (
	"cmp"
	"fmt"
	"slices"
	"sync"

	"cuelang.org/go/internal/par"
)

// A Reqs is the requirement graph on which Minimal Version Selection (MVS) operates.
//
// The version strings are opaque except for the special version "none"
// (see the documentation for V). In particular, MVS does not
// assume that the version strings are semantic versions; instead, the Max method
// gives access to the comparison operation.
//
// It must be safe to call methods on a Reqs from multiple goroutines simultaneously.
// Because a Reqs may read the underlying graph from the network on demand,
// the MVS algorithms parallelize the traversal to overlap network delays.
type Reqs[V comparable] interface {
	Versions[V]

	// Required returns the module versions explicitly required by m itself.
	// The caller must not modify the returned list.
	Required(m V) ([]V, error)

	// Max returns the maximum of v1 and v2 (it returns either v1 or v2).
	//
	// For all versions v, Max(v, "none") must be v,
	// and for the target passed as the first argument to MVS functions,
	// Max(target, v) must be target.
	//
	// Note that v1 < v2 can be written Max(v1, v2) != v1
	// and similarly v1 <= v2 can be written Max(v1, v2) == v2.
	Max(v1, v2 string) string
}

// An UpgradeReqs is a Reqs that can also identify available upgrades.
type UpgradeReqs[V comparable] interface {
	Reqs[V]

	// Upgrade returns the upgraded version of m,
	// for use during an UpgradeAll operation.
	// If m should be kept as is, Upgrade returns m.
	// If m is not yet used in the build, then m.Version will be "none".
	// More typically, m.Version will be the version required
	// by some other module in the build.
	//
	// If no module version is available for the given path,
	// Upgrade returns a non-nil error.
	// TODO(rsc): Upgrade must be able to return errors,
	// but should "no latest version" just return m instead?
	Upgrade(m V) (V, error)
}

// A DowngradeReqs is a Reqs that can also identify available downgrades.
type DowngradeReqs[V comparable] interface {
	Reqs[V]

	// Previous returns the version of m.Path immediately prior to m.Version,
	// or "none" if no such version is known.
	Previous(m V) (V, error)
}

// BuildList returns the build list for the target module.
//
// target is the root vertex of a module requirement graph. For cmd/cue, this is
// typically the main module, but note that this algorithm is not intended to
// be Go-specific: module paths and versions are treated as opaque values.
//
// reqs describes the module requirement graph and provides an opaque method
// for comparing versions.
//
// BuildList traverses the graph and returns a list containing the highest
// version for each visited module. The first element of the returned list is
// target itself; reqs.Max requires target.Version to compare higher than all
// other versions, so no other version can be selected. The remaining elements
// of the list are sorted by path.
//
// See https://research.swtch.com/vgo-mvs for details.
func BuildList[V comparable](targets []V, reqs Reqs[V]) ([]V, error) {
	return buildList(targets, reqs, nil)
}

func buildList[V comparable](targets []V, reqs Reqs[V], upgrade func(V) (V, error)) ([]V, error) {
	cmp := func(v1, v2 string) int {
		if reqs.Max(v1, v2) != v1 {
			return -1
		}
		if reqs.Max(v2, v1) != v2 {
			return 1
		}
		return 0
	}

	var (
		mu       sync.Mutex
		g        = NewGraph(Versions[V](reqs), cmp, targets)
		upgrades = map[V]V{}
		errs     = map[V]error{} // (non-nil errors only)
	)

	// Explore work graph in parallel in case reqs.Required
	// does high-latency network operations.
	var work par.Work[V]
	for _, target := range targets {
		work.Add(target)
	}
	work.Do(10, func(m V) {

		var required []V
		var err error
		if reqs.Version(m) != "none" {
			required, err = reqs.Required(m)
		}

		u := m
		if upgrade != nil {
			upgradeTo, upErr := upgrade(m)
			if upErr == nil {
				u = upgradeTo
			} else if err == nil {
				err = upErr
			}
		}

		mu.Lock()
		if err != nil {
			errs[m] = err
		}
		if u != m {
			upgrades[m] = u
			required = append([]V{u}, required...)
		}
		g.Require(m, required)
		mu.Unlock()

		for _, r := range required {
			work.Add(r)
		}
	})

	// If there was an error, find the shortest path from the target to the
	// node where the error occurred so we can report a useful error message.
	if len(errs) > 0 {
		errPath := g.FindPath(func(m V) bool {
			return errs[m] != nil
		})
		if len(errPath) == 0 {
			panic("internal error: could not reconstruct path to module with error")
		}

		err := errs[errPath[len(errPath)-1]]
		isUpgrade := func(from, to V) bool {
			if u, ok := upgrades[from]; ok {
				return u == to
			}
			return false
		}
		return nil, NewBuildListError(err, errPath, g.v, isUpgrade)
	}

	// The final list is the minimum version of each module found in the graph.
	list := g.BuildList()
	if vs := list[:len(targets)]; !slices.Equal(vs, targets) {
		// target.Version will be "" for modload, the main client of MVS.
		// "" denotes the main module, which has no version. However, MVS treats
		// version strings as opaque, so "" is not a special value here.
		// See golang.org/issue/31491, golang.org/issue/29773.
		panic(fmt.Sprintf("mistake: chose versions %+v instead of targets %+v", vs, targets))
	}
	return list, nil
}

// Req returns the minimal requirement list for the target module,
// with the constraint that all module paths listed in base must
// appear in the returned list.
func Req[V comparable](mainModule V, base []string, reqs Reqs[V]) ([]V, error) {
	list, err := BuildList([]V{mainModule}, reqs)
	if err != nil {
		return nil, err
	}

	// Note: Not running in parallel because we assume
	// that list came from a previous operation that paged
	// in all the requirements, so there's no I/O to overlap now.

	max := map[string]string{}
	for _, m := range list {
		max[reqs.Path(m)] = reqs.Version(m)
	}

	// Compute postorder, cache requirements.
	var postorder []V
	reqCache := map[V][]V{}
	reqCache[mainModule] = nil

	var walk func(V) error
	walk = func(m V) error {
		_, ok := reqCache[m]
		if ok {
			return nil
		}
		required, err := reqs.Required(m)
		if err != nil {
			return err
		}
		reqCache[m] = required
		for _, m1 := range required {
			if err := walk(m1); err != nil {
				return err
			}
		}
		postorder = append(postorder, m)
		return nil
	}
	for _, m := range list {
		if err := walk(m); err != nil {
			return nil, err
		}
	}

	// Walk modules in reverse post-order, only adding those not implied already.
	have := map[V]bool{}
	walk = func(m V) error {
		if have[m] {
			return nil
		}
		have[m] = true
		for _, m1 := range reqCache[m] {
			walk(m1)
		}
		return nil
	}
	// First walk the base modules that must be listed.
	var min []V
	haveBase := map[string]bool{}
	for _, path := range base {
		if haveBase[path] {
			continue
		}
		m, err := reqs.New(path, max[path])
		if err != nil {
			// Can't happen because arguments to New above are known to be OK.
			panic(err)
		}
		min = append(min, m)
		walk(m)
		haveBase[path] = true
	}
	// Now the reverse postorder to bring in anything else.
	for _, m := range slices.Backward(postorder) {
		if max[reqs.Path(m)] != reqs.Version(m) {
			// Older version.
			continue
		}
		if !have[m] {
			min = append(min, m)
			walk(m)
		}
	}
	slices.SortFunc(min, func(a, b V) int {
		return cmp.Compare(reqs.Path(a), reqs.Path(b))
	})
	return min, nil
}

// UpgradeAll returns a build list for the target module
// in which every module is upgraded to its latest version.
func UpgradeAll[V comparable](target V, reqs UpgradeReqs[V]) ([]V, error) {
	return buildList([]V{target}, Reqs[V](reqs), func(m V) (V, error) {
		if reqs.Path(m) == reqs.Path(target) {
			return target, nil
		}

		return reqs.Upgrade(m)
	})
}

// Upgrade returns a build list for the target module
// in which the given additional modules are upgraded.
func Upgrade[V comparable](target V, reqs UpgradeReqs[V], upgrade ...V) ([]V, error) {
	list, err := reqs.Required(target)
	if err != nil {
		return nil, err
	}

	pathInList := make(map[string]bool, len(list))
	for _, m := range list {
		pathInList[reqs.Path(m)] = true
	}
	list = slices.Clone(list)

	upgradeTo := make(map[string]string, len(upgrade))
	for _, u := range upgrade {
		if !pathInList[reqs.Path(u)] {
			newv, err := reqs.New(reqs.Path(u), "none")
			if err != nil {
				// Can't happen because arguments to New above are known to be OK.
				panic(err)
			}
			list = append(list, newv)
		}
		if prev, dup := upgradeTo[reqs.Path(u)]; dup {
			upgradeTo[reqs.Path(u)] = reqs.Max(prev, reqs.Version(u))
		} else {
			upgradeTo[reqs.Path(u)] = reqs.Version(u)
		}
	}

	return buildList([]V{target}, &override[V]{target, list, reqs}, func(m V) (V, error) {
		if v, ok := upgradeTo[reqs.Path(m)]; ok {
			return reqs.New(reqs.Path(m), v)
		}
		return m, nil
	})
}

// Downgrade returns a build list for the target module
// in which the given additional modules are downgraded,
// potentially overriding the requirements of the target.
//
// The versions to be downgraded may be unreachable from reqs.Latest and
// reqs.Previous, but the methods of reqs must otherwise handle such versions
// correctly.
func Downgrade[V comparable](target V, reqs DowngradeReqs[V], downgrade ...V) ([]V, error) {
	// Per https://research.swtch.com/vgo-mvs#algorithm_4:
	// “To avoid an unnecessary downgrade to E 1.1, we must also add a new
	// requirement on E 1.2. We can apply Algorithm R to find the minimal set of
	// new requirements to write to go.mod.”
	//
	// In order to generate those new requirements, we need to identify versions
	// for every module in the build list — not just reqs.Required(target).
	list, err := BuildList([]V{target}, reqs)
	if err != nil {
		return nil, err
	}
	list = list[1:] // remove target

	max := make(map[string]string)
	for _, r := range list {
		max[reqs.Path(r)] = reqs.Version(r)
	}
	for _, d := range downgrade {
		if v, ok := max[reqs.Path(d)]; !ok || reqs.Max(v, reqs.Version(d)) != reqs.Version(d) {
			max[reqs.Path(d)] = reqs.Version(d)
		}
	}

	var (
		added    = make(map[V]bool)
		rdeps    = make(map[V][]V)
		excluded = make(map[V]bool)
	)
	var exclude func(V)
	exclude = func(m V) {
		if excluded[m] {
			return
		}
		excluded[m] = true
		for _, p := range rdeps[m] {
			exclude(p)
		}
	}
	var add func(V)
	add = func(m V) {
		if added[m] {
			return
		}
		added[m] = true
		if v, ok := max[reqs.Path(m)]; ok && reqs.Max(reqs.Version(m), v) != v {
			// m would upgrade an existing dependency — it is not a strict downgrade,
			// and because it was already present as a dependency, it could affect the
			// behavior of other relevant packages.
			exclude(m)
			return
		}
		list, err := reqs.Required(m)
		if err != nil {
			// If we can't load the requirements, we couldn't load the go.mod file.
			// There are a number of reasons this can happen, but this usually
			// means an older version of the module had a missing or invalid
			// go.mod file. For example, if example.com/mod released v2.0.0 before
			// migrating to modules (v2.0.0+incompatible), then added a valid go.mod
			// in v2.0.1, downgrading from v2.0.1 would cause this error.
			//
			// TODO(golang.org/issue/31730, golang.org/issue/30134): if the error
			// is transient (we couldn't download go.mod), return the error from
			// Downgrade. Currently, we can't tell what kind of error it is.
			exclude(m)
			return
		}
		for _, r := range list {
			add(r)
			if excluded[r] {
				exclude(m)
				return
			}
			rdeps[r] = append(rdeps[r], m)
		}
	}

	downgraded := make([]V, 0, len(list)+1)
	downgraded = append(downgraded, target)
List:
	for _, r := range list {
		add(r)
		for excluded[r] {
			p, err := reqs.Previous(r)
			if err != nil {
				// This is likely a transient error reaching the repository,
				// rather than a permanent error with the retrieved version.
				//
				// TODO(golang.org/issue/31730, golang.org/issue/30134):
				// decode what to do based on the actual error.
				return nil, err
			}
			// If the target version is a pseudo-version, it may not be
			// included when iterating over prior versions using reqs.Previous.
			// Insert it into the right place in the iteration.
			// If v is excluded, p should be returned again by reqs.Previous on the next iteration.
			if v := max[reqs.Path(r)]; reqs.Max(v, reqs.Version(r)) != v && reqs.Max(reqs.Version(p), v) != reqs.Version(p) {
				p0, err := reqs.New(reqs.Path(p), v)
				if err != nil {
					// Can't happen because arguments to New above are known to be OK.
					panic(err)
				}
				p = p0
			}
			if reqs.Version(p) == "none" {
				continue List
			}
			add(p)
			r = p
		}
		downgraded = append(downgraded, r)
	}

	// The downgrades we computed above only downgrade to versions enumerated by
	// reqs.Previous. However, reqs.Previous omits some versions — such as
	// pseudo-versions and retracted versions — that may be selected as transitive
	// requirements of other modules.
	//
	// If one of those requirements pulls the version back up above the version
	// identified by reqs.Previous, then the transitive dependencies of that
	// initially-downgraded version should no longer matter — in particular, we
	// should not add new dependencies on module paths that nothing else in the
	// updated module graph even requires.
	//
	// In order to eliminate those spurious dependencies, we recompute the build
	// list with the actual versions of the downgraded modules as selected by MVS,
	// instead of our initial downgrades.
	// (See the downhiddenartifact and downhiddencross test cases).
	actual, err := BuildList([]V{target}, &override[V]{
		target: target,
		list:   downgraded,
		Reqs:   reqs,
	})
	if err != nil {
		return nil, err
	}
	actualVersion := make(map[string]string, len(actual))
	for _, m := range actual {
		actualVersion[reqs.Path(m)] = reqs.Version(m)
	}

	downgraded = downgraded[:0]
	for _, m := range list {
		if v, ok := actualVersion[reqs.Path(m)]; ok {
			m1, err := reqs.New(reqs.Path(m), v)
			if err != nil {
				// Can't happen because arguments to New above are known to be OK.
				panic(err)
			}
			downgraded = append(downgraded, m1)
		}
	}

	return BuildList([]V{target}, &override[V]{
		target: target,
		list:   downgraded,
		Reqs:   reqs,
	})
}

type override[V comparable] struct {
	target V
	list   []V
	Reqs[V]
}

func (r *override[V]) Required(m V) ([]V, error) {
	if m == r.target {
		return r.list, nil
	}
	return r.Reqs.Required(m)
}
