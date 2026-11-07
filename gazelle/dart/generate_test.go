package dart

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bazelbuild/bazel-gazelle/config"
)

func TestStageTargetName(t *testing.T) {
	jsonInfo := &builderInfo{
		Annotation: "JsonSerializable",
		Produces:   []string{".json_serializable.g.part"},
	}
	freezedInfo := &builderInfo{
		Annotation: "Freezed",
		Produces:   []string{".freezed.dart"},
	}
	routerInfo := &builderInfo{
		Annotation: "StackedApp",
		Produces:   []string{".router.dart"},
	}
	locatorInfo := &builderInfo{
		Annotation: "StackedApp",
		Produces:   []string{".locator.dart"},
	}
	tests := []struct {
		name   string
		src    string
		stage  pipelineStage
		fanOut bool
		want   string
	}{
		{
			name:  "JsonSerializable (no fan-out)",
			src:   "lib/user.dart",
			stage: pipelineStage{Builder: jsonInfo, ID: nodeKey(jsonInfo)},
			want:  "_user_json_serializable_gen",
		},
		{
			name:  "freezed (no fan-out)",
			src:   "lib/event.dart",
			stage: pipelineStage{Builder: freezedInfo, ID: nodeKey(freezedInfo)},
			want:  "_event_freezed_gen",
		},
		{
			name:  "combining stage",
			src:   "lib/user.dart",
			stage: pipelineStage{Combining: true},
			want:  "_user_combined",
		},
		{
			name: "stacked router fan-out gets distinct name from locator",
			src:  "lib/app.dart",
			stage: pipelineStage{
				Builder: routerInfo,
				ID:      nodeKey(routerInfo),
			},
			fanOut: true,
			want:   "_app_stacked_app_router_gen",
		},
		{
			name: "stacked locator fan-out",
			src:  "lib/app.dart",
			stage: pipelineStage{
				Builder: locatorInfo,
				ID:      nodeKey(locatorInfo),
			},
			fanOut: true,
			want:   "_app_stacked_app_locator_gen",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stageTargetName(tt.src, tt.stage, tt.fanOut)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFilterOutGeneratedFiles(t *testing.T) {
	registry := registryWith(&builderInfo{
		Annotation: "X",
		Produces:   []string{".x.g.part"},
	})
	in := []DartFileInfo{
		{Path: "lib/user.dart"},
		{Path: "lib/user.g.dart"},
		{Path: "lib/user.x.g.part"},
		{Path: "lib/user.freezed.dart"},
		{Path: "lib/user.mocks.dart"},
	}
	out := filterOutGeneratedFiles(in, registry)
	if len(out) != 1 || out[0].Path != "lib/user.dart" {
		t.Fatalf("filter dropped wrong files: %#v", out)
	}
}

func TestDefaultBuildersIncludesShipped(t *testing.T) {
	r := &builderRegistry{byAnnotation: map[string][]*builderInfo{}}
	for _, b := range defaultBuilders() {
		r.byAnnotation[b.Annotation] = append(r.byAnnotation[b.Annotation], b)
	}
	for _, want := range []string{
		"JsonSerializable", "freezed", "Freezed", "CopyWith",
		"GenerateMocks", "TypedGoRoute",
		"DriftDatabase", "StackedApp", "FormView", "SerializersFor",
	} {
		if r.lookup(want) == nil {
			t.Errorf("default registry missing %s", want)
		}
	}
	// @SerializersFor is the annotation-driven entry point to built_value
	// (e.g. serializers.dart in the e2e exemplar); it must map onto the
	// same built_value builder as the synthetic BuiltValue trigger.
	sfor := r.lookup("SerializersFor")
	bval := r.lookup("BuiltValue")
	if len(sfor) != 1 || len(bval) != 1 {
		t.Fatalf("SerializersFor/BuiltValue should each register exactly one builder, got %d/%d", len(sfor), len(bval))
	}
	if sfor[0].ShimLabel != bval[0].ShimLabel || sfor[0].Macro != bval[0].Macro {
		t.Errorf("SerializersFor should share built_value's shim/macro, got %+v", sfor[0])
	}
	// @StackedApp must fan out to exactly five builders (router, locator,
	// logger, dialog, bottomsheet — form uses @FormView separately).
	got := len(r.lookup("StackedApp"))
	if got != 5 {
		t.Errorf("@StackedApp should register 5 sub-builders, got %d", got)
	}
}

// TestAllRegisteredProducedExtensionsSorted verifies the returned slice
// is lexicographically sorted so downstream consumers (Gazelle's
// generated-file filter) don't depend on Go's randomised map iteration.
func TestAllRegisteredProducedExtensionsSorted(t *testing.T) {
	r := registryWith(
		&builderInfo{Annotation: "Z", Produces: []string{".zzz.g.part"}},
		&builderInfo{Annotation: "A", Produces: []string{".aaa.g.part"}},
		&builderInfo{Annotation: "M", Produces: []string{".mmm.g.part", ".more.g.part"}},
	)
	got := r.allRegisteredProducedExtensions()
	for i := 1; i < len(got); i++ {
		if got[i] < got[i-1] {
			t.Fatalf("extensions not sorted: %v", got)
		}
	}
}

func TestPackageRelPath(t *testing.T) {
	writePubspec := func(t *testing.T, root, rel string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, rel, "pubspec.yaml"), []byte("name: p\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	nested := t.TempDir()
	for _, d := range []string{"a/pkg/bin/sub", "a/pkg/example/bin", "nameless/bin", "nopkg/bin", "decl/bin"} {
		if err := os.MkdirAll(filepath.Join(nested, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writePubspec(t, nested, "a/pkg")
	writePubspec(t, nested, "a/pkg/example")
	if err := os.WriteFile(filepath.Join(nested, "nameless/pubspec.yaml"), []byte("description: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	atRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(atRoot, "tools/bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	writePubspec(t, atRoot, "")

	declCfg := &config.Config{Exts: map[string]interface{}{
		extKeyPackageDecl: &packageDecl{Rel: "decl", Name: "decl_pkg", PackageName: "decl"},
	}}
	plain := &config.Config{Exts: map[string]interface{}{}}
	tests := []struct {
		root, rel, want string
		c               *config.Config
	}{
		{nested, "a/pkg", "", plain},
		{nested, "a/pkg/bin", "bin", plain},
		{nested, "a/pkg/bin/sub", "bin/sub", plain},
		{nested, "a/pkg/example/bin", "bin", plain},
		{nested, "nameless/bin", "nameless/bin", plain},
		{nested, "nopkg/bin", "nopkg/bin", plain},
		{nested, "decl/bin", "bin", declCfg},
		{atRoot, "bin", "bin", plain},
		{atRoot, "tools/bin", "tools/bin", plain},
	}
	for _, tt := range tests {
		if got := packageRelPath(tt.c, filepath.Join(tt.root, tt.rel), tt.rel); got != tt.want {
			t.Errorf("packageRelPath(%q) = %q, want %q", tt.rel, got, tt.want)
		}
	}
}

// A package below the workspace root gets dart_binary for bin/.
func TestNestedPackageBin(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "apps", "server")
	if err := os.MkdirAll(filepath.Join(pkg, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "pubspec.yaml"), []byte("name: server\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "bin", "main.dart"), []byte("void main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := generate(t, &dartLang{}, root, "apps/server/bin", []string{"main.dart"}, "")
	if r, _ := findRule(res, "main"); r == nil || r.Kind() != "dart_binary" {
		t.Errorf("got %v, want dart_binary %q", res.Gen, "main")
	}
}
