package dart

import (
	"log"
	"path"
	"sort"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/repo"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

// indexedKinds is the set of rule kinds that provide a Dart package for
// import resolution: the dart_library primitive plus every convenience
// macro (each wraps a dart_library under the hood).
var indexedKinds = func() map[string]bool {
	m := map[string]bool{"dart_library": true}
	for _, k := range macroKinds {
		m[k] = true
	}
	return m
}()

// Imports returns the import specifications for a rule.
// These are used to build the import index for dependency resolution.
func (d *dartLang) Imports(c *config.Config, r *rule.Rule, f *rule.File) []resolve.ImportSpec {
	// A dart_library (or macro wrapping one) provides its rule name as an
	// import, plus its `package_name` attr when that differs — imports use
	// the Dart package name (`package:<package_name>/...`), which need not
	// match the Bazel target name.
	if !indexedKinds[r.Kind()] {
		return nil
	}
	p := placeLibrary(c, f.Pkg, repoDir(c, f.Pkg))
	pkgName := r.AttrString("package_name")
	var specs []resolve.ImportSpec
	if p.Decl == nil {
		specs = append(specs, resolve.ImportSpec{Lang: dartName, Imp: r.Name()})
		if pkgName != "" && pkgName != r.Name() {
			specs = append(specs, resolve.ImportSpec{Lang: dartName, Imp: pkgName})
		}
	} else if f.Pkg == libDirOf(p.Root) && r.Name() == p.PackageName {
		// One directory of a package declared above is named for its
		// directory, which is no package name (`lib/src/path` is not
		// package:path). Only the package's lib/ library stands for it.
		specs = append(specs, resolve.ImportSpec{Lang: dartName, Imp: p.PackageName})
	}
	// Each source is also indexed by its `package:` URI, so an import of one
	// file resolves to the target holding it, wherever in the package that is.
	if pkgName != "" {
		p.PackageName = pkgName
	}
	for _, src := range r.AttrStrings("srcs") {
		if strings.HasPrefix(src, ":") || strings.HasPrefix(src, "/") || strings.HasPrefix(src, "@") {
			continue
		}
		if uri := packageURI(p, path.Join(f.Pkg, src)); uri != "" {
			specs = append(specs, resolve.ImportSpec{Lang: dartName, Imp: uri})
		}
	}
	return specs
}

// Embeds returns rules that this rule embeds (not used for Dart).
func (d *dartLang) Embeds(r *rule.Rule, from label.Label) []label.Label {
	return nil
}

// Resolve resolves import dependencies for a rule.
func (d *dartLang) Resolve(c *config.Config, ix *resolve.RuleIndex, rc *repo.RemoteCache, r *rule.Rule, rawImports interface{}, from label.Label) {
	is, ok := rawImports.(*importSet)
	if !ok || is == nil {
		return
	}

	pubDepsRepo := ""
	if c.Exts != nil {
		if repo, ok := c.Exts["dart_pub_deps_repo"].(string); ok {
			pubDepsRepo = repo
		}
	}

	var deps []string
	for _, pkg := range is.SortedPackages() {
		// A library named for its package doesn't depend on that package.
		if !is.split && pkg == r.Name() {
			continue
		}

		// A gazelle:resolve override for the package wins.
		spec := resolve.ImportSpec{Lang: dartName, Imp: pkg}
		self := pkg == is.self
		if !(self && is.split) {
			if override, ok := resolve.FindRuleWithOverride(c, spec, dartName); ok {
				deps = append(deps, override.Rel(from.Repo, from.Pkg).String())
				continue
			}
		}

		// A file imported by URI resolves to the target that holds it.
		// Only when some file is in no indexed target does the package's
		// name decide, and never for the rule's own package when it is one
		// directory of it: the package's lib/ library likely exports this
		// one, so depending on it would be a cycle. An unindexed file of
		// its own package (generated, excluded) is left to the user.
		if resolveFiles(c, ix, is.SortedURIs(pkg), from, &deps) || (self && is.split) {
			continue
		}

		// Try to find in the rule index (first-party deps). Skip exact
		// self-matches (a rule whose package_name is indexed can match its
		// own import set) but keep legitimate intra-package cross-target
		// deps — those are filtered above only when pkg == r.Name().
		matches := ix.FindRulesByImportWithConfig(c, spec, dartName)
		matched := false
		for _, m := range matches {
			if m.IsSelfImport(from) {
				continue
			}
			deps = append(deps, m.Label.Rel(from.Repo, from.Pkg).String())
			matched = true
			break
		}
		if matched {
			continue
		}

		// The rule's own package is never a pub package.
		if pkg == is.self {
			continue
		}

		// External repository (pub package)
		var lbl label.Label
		if pubDepsRepo != "" {
			lbl = label.New(pubDepsRepo, "", pkg)
		} else {
			lbl = label.New(pkg, "", pkg)
		}
		deps = append(deps, lbl.String())
	}

	if len(deps) > 0 {
		deps = dedupe(deps)
		r.SetAttr("deps", deps)
	}
}

// resolveFiles adds a dep on the target holding each file URI and reports
// whether every one was found (a file in the importing rule itself counts).
// No URIs is not found: the import names a package, not a file.
func resolveFiles(c *config.Config, ix *resolve.RuleIndex, uris []string, from label.Label, deps *[]string) bool {
	if len(uris) == 0 {
		return false
	}
	all := true
	for _, uri := range uris {
		spec := resolve.ImportSpec{Lang: dartName, Imp: uri}
		if override, ok := resolve.FindRuleWithOverride(c, spec, dartName); ok {
			*deps = append(*deps, override.Rel(from.Repo, from.Pkg).String())
			continue
		}
		matches := ix.FindRulesByImportWithConfig(c, spec, dartName)
		if len(matches) == 0 {
			all = false
			continue
		}
		if hasSelf(matches, from) {
			continue
		}
		if len(matches) > 1 {
			// As Gazelle's Go extension does: no guess, and say so.
			var labels []string
			for _, m := range matches {
				labels = append(labels, m.Label.String())
			}
			log.Printf("%s: %s is in more than one rule (%s); add a # gazelle:resolve dart directive to choose", from, uri, strings.Join(labels, ", "))
			continue
		}
		*deps = append(*deps, matches[0].Label.Rel(from.Repo, from.Pkg).String())
	}
	return all
}

func hasSelf(matches []resolve.FindResult, from label.Label) bool {
	for _, m := range matches {
		if m.IsSelfImport(from) {
			return true
		}
	}
	return false
}

func dedupe(deps []string) []string {
	sort.Strings(deps)
	out := deps[:0]
	for i, d := range deps {
		if i == 0 || d != deps[i-1] {
			out = append(out, d)
		}
	}
	return out
}
