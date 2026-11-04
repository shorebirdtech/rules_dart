package dart

import (
	"flag"
	"testing"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

// splitRepo indexes package app declared at the repo root and split across
// lib/, lib/src/path (a directory named like the pub package path) and
// lib/src/view, and returns the config and index Resolve reads.
func splitRepo(t *testing.T, directives string) (*config.Config, *resolve.RuleIndex, *rule.Rule) {
	t.Helper()
	c := declConfig(t, "")
	c.Exts["dart_pub_deps_repo"] = "pub_deps"
	rc := &resolve.Configurer{}
	rc.RegisterFlags(flag.NewFlagSet("test", flag.ContinueOnError), "update", c)
	if err := rc.CheckFlags(nil, c); err != nil {
		t.Fatal(err)
	}
	root, err := rule.LoadData("BUILD.bazel", "", []byte(directives))
	if err != nil {
		t.Fatal(err)
	}
	rc.Configure(c, "", root)

	lang := NewLanguage()
	ix := resolve.NewRuleIndex(func(*rule.Rule, string) resolve.Resolver { return lang })
	var view *rule.Rule
	for _, lib := range []struct{ rel, name, src string }{
		{"lib", "app", "app.dart"},
		{"lib/src/path", "path", "p.dart"},
		{"lib/src/view", "view", "v.dart"},
	} {
		f := rule.EmptyFile(lib.rel+"/BUILD.bazel", lib.rel)
		r := rule.NewRule("dart_library", lib.name)
		r.SetAttr("srcs", []string{lib.src})
		r.SetAttr("package", "//:pkg")
		r.Insert(f)
		ix.AddRule(c, r, f)
		if lib.name == "view" {
			view = r
		}
	}
	ix.Finish()
	return c, ix, view
}

func viewImports(uris map[string][]string) *importSet {
	is := &importSet{packages: map[string]bool{}, uris: map[string]map[string]bool{}, self: "app", split: true}
	for pkg, us := range uris {
		is.packages[pkg] = true
		is.uris[pkg] = map[string]bool{}
		for _, u := range us {
			is.uris[pkg][u] = true
		}
	}
	return is
}

func resolveView(t *testing.T, directives string, is *importSet) []string {
	t.Helper()
	c, ix, view := splitRepo(t, directives)
	NewLanguage().(*dartLang).Resolve(c, ix, nil, view, is, label.New("", "lib/src/view", "view"))
	return view.AttrStrings("deps")
}

func TestResolveSplitDirNamedLikePubPackage(t *testing.T) {
	got := resolveView(t, "", viewImports(map[string][]string{
		"app":  {"package:app/src/path/p.dart"},
		"path": {"package:path/path.dart"},
	}))
	want := []string{"//lib/src/path", "@pub_deps//:path"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("deps %v, want %v", got, want)
	}
}

func TestResolveUnindexedSelfFileAddsNoPackageDep(t *testing.T) {
	got := resolveView(t, "", viewImports(map[string][]string{
		"app": {"package:app/src/path/p.dart", "package:app/src/gen/model.g.dart"},
	}))
	if len(got) != 1 || got[0] != "//lib/src/path" {
		t.Fatalf("deps %v, want [//lib/src/path] (no //lib:app, which would be a cycle)", got)
	}
}

func TestResolvePackageOverrideWinsOverFiles(t *testing.T) {
	got := resolveView(t, "# gazelle:resolve dart path //third_party:path\n", viewImports(map[string][]string{
		"path": {"package:path/path.dart"},
	}))
	if len(got) != 1 || got[0] != "//third_party:path" {
		t.Fatalf("deps %v, want [//third_party:path]", got)
	}
}

func TestImportsSplitDirIndexesFilesNotItsName(t *testing.T) {
	c := declConfig(t, "")
	for _, tt := range []struct {
		rel, name string
		want      []string
	}{
		{"lib/src/path", "path", []string{"package:app/src/path/p.dart"}},
		{"lib", "app", []string{"app", "package:app/p.dart"}},
	} {
		f := rule.EmptyFile(tt.rel+"/BUILD.bazel", tt.rel)
		r := rule.NewRule("dart_library", tt.name)
		r.SetAttr("srcs", []string{"p.dart"})
		var got []string
		for _, s := range NewLanguage().(*dartLang).Imports(c, r, f) {
			got = append(got, s.Imp)
		}
		if len(got) != len(tt.want) || got[0] != tt.want[0] || got[len(got)-1] != tt.want[len(tt.want)-1] {
			t.Errorf("%s: imports %v, want %v", tt.rel, got, tt.want)
		}
	}
}

func TestPlaceLibraryBesideDeclarationUsesDeclaredName(t *testing.T) {
	c := declConfig(t, "pkgdir")
	p := placeLibrary(c, "pkgdir", repoDir(c, "pkgdir"))
	if p.PackageName != "app" || p.Decl != nil || p.Root != "pkgdir" {
		t.Fatalf("got %+v", p)
	}
}
