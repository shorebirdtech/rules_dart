package dart

import (
	"testing"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

func declConfig(t *testing.T, rel string) *config.Config {
	t.Helper()
	f, err := rule.LoadData("BUILD.bazel", rel, []byte(`
dart_package_metadata(name = "pkg", package_name = "app", language_version = "3.11")
`))
	if err != nil {
		t.Fatal(err)
	}
	c := &config.Config{RepoRoot: t.TempDir(), Exts: map[string]interface{}{}}
	configurePackageDecl(c, rel, f)
	return c
}

func TestPlaceLibraryUnderDeclaredLib(t *testing.T) {
	c := declConfig(t, "")
	for _, tt := range []struct {
		rel, name, label string
	}{
		{"lib", "app", "//:pkg"},
		{"lib/src/model", "model", "//:pkg"},
	} {
		p := placeLibrary(c, tt.rel, repoDir(c, tt.rel))
		if p.Decl == nil || p.Root != "" || p.PackageName != "app" {
			t.Fatalf("%s: got %+v", tt.rel, p)
		}
		if got := splitLibraryName(tt.rel, p); got != tt.name {
			t.Errorf("%s: name %q, want %q", tt.rel, got, tt.name)
		}
		if got := declLabel(tt.rel, p.Decl); got != tt.label {
			t.Errorf("%s: label %q, want %q", tt.rel, got, tt.label)
		}
	}
	// Outside lib/ (tests, nested packages) is not part of the declaration.
	for _, rel := range []string{"", "test", "packages/sub", "library"} {
		if p := placeLibrary(c, rel, repoDir(c, rel)); p.Decl != nil {
			t.Errorf("%s: placed in the declared package: %+v", rel, p)
		}
	}
}

func TestPlaceLibraryNestedDecl(t *testing.T) {
	c := declConfig(t, "apps/app")
	p := placeLibrary(c, "apps/app/lib/src/x", "")
	if p.Decl == nil || p.Root != "apps/app" {
		t.Fatalf("got %+v", p)
	}
	if got := declLabel("apps/app/lib/src/x", p.Decl); got != "//apps/app:pkg" {
		t.Errorf("label %q", got)
	}
	if got := packageURI(p, "apps/app/lib/src/x/a.dart"); got != "package:app/src/x/a.dart" {
		t.Errorf("uri %q", got)
	}
}

func TestImportURI(t *testing.T) {
	p := placement{Root: "", PackageName: "app"}
	file := "lib/src/arcs/arc.dart"
	for _, tt := range []struct {
		uri, want string
	}{
		{"../code/code.dart", "package:app/src/code/code.dart"},
		{"step.dart", "package:app/src/arcs/step.dart"},
		{"./step.dart", "package:app/src/arcs/step.dart"},
		{"package:path/path.dart", "package:path/path.dart"},
		{"package:app/src/x.dart", "package:app/src/x.dart"},
		{"dart:io", ""},
		{"../../../tool/x.dart", ""},
	} {
		if got := importURI(p, file, classifyURI(tt.uri)); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.uri, got, tt.want)
		}
	}
}
