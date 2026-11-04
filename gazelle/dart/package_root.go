package dart

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

// extKeyPackageDecl holds the nearest `dart_package_metadata` Gazelle has seen
// at or above the current directory.
const extKeyPackageDecl = "dart_package_decl"

// packageDecl is a `dart_package_metadata` target: where it is declared and
// the Dart package it names. A `dart_library` under `<Rel>/lib/` that points
// at it has `<Rel>` as its package root (see `library_lib_root` in
// dart_library.bzl), so one package's sources can live in one BUILD file per
// directory.
type packageDecl struct {
	Rel         string // the declaring BUILD file's directory
	Name        string // the target's name
	PackageName string // its `package_name`
}

// configurePackageDecl records a directory's `dart_package_metadata`, if it
// has one, for the directories below it.
func configurePackageDecl(c *config.Config, rel string, f *rule.File) {
	for _, r := range f.Rules {
		if r.Kind() != "dart_package_metadata" {
			continue
		}
		name := r.AttrString("package_name")
		if name == "" {
			continue
		}
		if c.Exts == nil {
			c.Exts = make(map[string]interface{})
		}
		c.Exts[extKeyPackageDecl] = &packageDecl{Rel: rel, Name: r.Name(), PackageName: name}
		return
	}
}

func libDirOf(root string) string {
	if root == "" {
		return "lib"
	}
	return root + "/lib"
}

// placement is where a directory's library sits in its Dart package.
type placement struct {
	Root        string       // the package root (parent of lib/), repo-relative
	PackageName string       // the Dart package name its files are imported by
	Decl        *packageDecl // set when the library is one directory of a declared package
}

// placeLibrary returns the package a library in directory rel belongs to:
// the declared package when rel lies under its lib/, otherwise a package
// rooted at rel itself (less a trailing lib), as the rule reads it.
func placeLibrary(c *config.Config, rel, dir string) placement {
	if d, ok := c.Exts[extKeyPackageDecl].(*packageDecl); ok && d != nil {
		if rel == d.Rel {
			// Beside its declaration: the declaration names the package.
			return placement{Root: d.Rel, PackageName: d.PackageName}
		}
		if isInDir(rel, libDirOf(d.Rel)) {
			return placement{Root: d.Rel, PackageName: d.PackageName, Decl: d}
		}
	}
	root := rel
	if root == "lib" {
		root = ""
	} else {
		root = strings.TrimSuffix(root, "/lib")
	}
	return placement{Root: root, PackageName: libraryName(rel, dir, c)}
}

// splitLibraryName is the target name of one directory's library within a
// declared package: the directory's name, or the package's name for lib/.
func splitLibraryName(rel string, p placement) string {
	if rel == libDirOf(p.Root) {
		return p.PackageName
	}
	return path.Base(rel)
}

// declLabel is the `package` attribute pointing a library in rel at its
// declaration.
func declLabel(rel string, d *packageDecl) string {
	return label.New("", d.Rel, d.Name).Rel("", rel).String()
}

// packageURI is the `package:` URI of the file at repo-relative path file,
// or "" when it lies outside the package's lib/.
func packageURI(p placement, file string) string {
	lib := libDirOf(p.Root) + "/"
	if p.PackageName == "" || !strings.HasPrefix(file, lib) {
		return ""
	}
	return "package:" + p.PackageName + "/" + strings.TrimPrefix(file, lib)
}

// importURI is the `package:` URI an import in the file at repo-relative path
// file names: a package import as written, a relative one resolved against the
// file's directory. "" for `dart:` imports and relative ones leaving lib/.
func importURI(p placement, file string, imp DartImport) string {
	switch {
	case imp.IsPackage:
		if imp.Path == "" {
			return ""
		}
		return imp.URI
	case imp.IsRelative:
		if strings.Contains(imp.URI, ":") {
			return ""
		}
		return packageURI(p, path.Clean(path.Join(path.Dir(file), imp.URI)))
	}
	return ""
}

// repoDir is the absolute directory of repo-relative rel.
func repoDir(c *config.Config, rel string) string {
	return filepath.Join(c.RepoRoot, filepath.FromSlash(rel))
}
