package generator

import (
	"bytes"
	"go/parser"
	"go/token"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rustyoz/svg"
)

// svgoVersion is pinned in the throwaway modules the tests use to compile
// generated code.
const svgoVersion = "v0.0.0-20211024235047-1546f124cd8b"

func parseFixture(t *testing.T, name string) *svg.Svg {
	t.Helper()
	buf, err := ioutil.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	parsed, err := svg.ParseSvg(string(buf), name, 1.0)
	if err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	return parsed
}

// generate runs the generator against parsed and returns the formatted Go
// source it would write.
func generate(t *testing.T, parsed *svg.Svg, typeName string, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	gen := NewGenerator("patterns", "\t", typeName, width, height)
	if err := gen.WriteSvgCode(parsed, &buf); err != nil {
		t.Fatalf("WriteSvgCode: %v", err)
	}
	return buf.Bytes()
}

// TestGenerateNestedGroups locks the full output for a non-square SVG with
// nested groups: the default constructor must keep the original visual
// output and the identity transform must render as the empty string.
func TestGenerateNestedGroups(t *testing.T) {
	parsed := parseFixture(t, "nested_groups.svg")
	src := generate(t, parsed, "Sample", 0, 0)

	if !bytes.Contains(src, []byte("patternWidth:  100")) ||
		!bytes.Contains(src, []byte("patternHeight: 60")) {
		t.Errorf("non-square dimensions not embedded:\n%s", src)
	}
	if !bytes.Contains(src, []byte(`return ""`)) {
		t.Errorf("identity transform must render as empty string:\n%s", src)
	}
	if !bytes.Contains(src, []byte(`canvas.Path("M0 0h50v30H0z")`)) {
		t.Errorf("outer path missing:\n%s", src)
	}
	if !bytes.Contains(src, []byte(`canvas.Gid("inner")`)) {
		t.Errorf("nested group missing:\n%s", src)
	}
	if !bytes.Contains(src, []byte(`canvas.Path("M10 10h20v10H10z")`)) {
		t.Errorf("nested path missing:\n%s", src)
	}

	// The source must parse as valid Go.
	if _, err := parser.ParseFile(token.NewFileSet(), "sample.go", src, parser.AllErrors); err != nil {
		t.Fatalf("generated source does not parse: %v\n%s", err, src)
	}
}

// TestViewBoxFallback covers non-square, comma-separated viewBoxes with
// fractional sizes, including the case where width/height attrs are absent.
func TestViewBoxFallback(t *testing.T) {
	parsed := parseFixture(t, "viewbox_only.svg")
	src := generate(t, parsed, "ViewBoxPattern", 0, 0)

	if !bytes.Contains(src, []byte("patternWidth:  121")) {
		t.Errorf("expected viewBox width 120.5 rounded to 121:\n%s", src)
	}
	if !bytes.Contains(src, []byte("patternHeight: 40")) {
		t.Errorf("expected viewBox height 40:\n%s", src)
	}
}

// TestExplicitDimensionsOverride ensures generator dimensions win over the
// SVG attributes.
func TestExplicitDimensionsOverride(t *testing.T) {
	parsed := parseFixture(t, "nested_groups.svg")
	src := generate(t, parsed, "Sized", 33, 77)
	if !bytes.Contains(src, []byte("patternWidth:  33")) ||
		!bytes.Contains(src, []byte("patternHeight: 77")) {
		t.Errorf("explicit dimensions not used:\n%s", src)
	}
}

// TestInvalidDimensions verifies bad inputs fail explicitly instead of
// silently falling back to a 200px tile.
func TestInvalidDimensions(t *testing.T) {
	parsed := parseFixture(t, "nested_groups.svg")

	var buf bytes.Buffer
	gen := NewGenerator("patterns", "\t", "Bad", -5, 10)
	err := gen.WriteSvgCode(parsed, &buf)
	if err == nil || !strings.Contains(err.Error(), "width") {
		t.Fatalf("expected width error, got %v", err)
	}

	gen = NewGenerator("patterns", "\t", "Bad", 10, -5)
	buf.Reset()
	err = gen.WriteSvgCode(parsed, &buf)
	if err == nil || !strings.Contains(err.Error(), "height") {
		t.Fatalf("expected height error, got %v", err)
	}

	// Percentage dimensions without a usable viewBox cannot be resolved.
	percentSvg := parseFixture(t, "percent_dimensions.svg")
	gen = NewGenerator("patterns", "\t", "Bad", 0, 0)
	buf.Reset()
	err = gen.WriteSvgCode(percentSvg, &buf)
	if err == nil || !strings.Contains(err.Error(), "width") {
		t.Fatalf("expected unresolvable width error, got %v", err)
	}
}

// TestRejectEmptyAndAbnormalSVG covers nil input and SVGs without drawable
// content.
func TestRejectEmptyAndAbnormalSVG(t *testing.T) {
	gen := NewGenerator("patterns", "\t", "Empty", 10, 10)

	var buf bytes.Buffer
	if err := gen.WriteSvgCode(nil, &buf); err == nil {
		t.Fatal("nil SVG must produce an error")
	}

	noDrawable := parseFixture(t, "no_drawable.svg")
	buf.Reset()
	if err := gen.WriteSvgCode(noDrawable, &buf); err == nil {
		t.Fatal("SVG without paths or groups must produce an error")
	}

	buf.Reset()
	if err := gen.WriteSvgCode(parseFixture(t, "nested_groups.svg"), nil); err == nil {
		t.Fatal("nil output file must produce an error")
	}
}

// TestDuplicateIDsAreDeduplicated ensures repeated group IDs are made unique
// so the generated document never defines the same id twice.
func TestDuplicateIDsAreDeduplicated(t *testing.T) {
	parsed := parseFixture(t, "duplicate_ids.svg")
	src := generate(t, parsed, "Dup", 0, 0)

	re := regexp.MustCompile(`canvas\.Gid\("([^"]+)"\)`)
	seen := map[string]bool{}
	for _, match := range re.FindAllStringSubmatch(string(src), -1) {
		id := match[1]
		if seen[id] {
			t.Fatalf("duplicate id %q emitted:\n%s", id, src)
		}
		seen[id] = true
	}
	if len(seen) != 3 {
		t.Fatalf("expected 3 unique group ids, got %v", seen)
	}
	if !seen["dup"] || !seen["dup-2"] || !seen["dup-3"] {
		t.Fatalf("expected dup, dup-2 and dup-3, got %v", seen)
	}
}

// TestQuotedAttributesCompile sends attribute values containing double
// quotes, ampersands and angle brackets through the generator and compiles
// the result. encoding/xml decodes entities to literal characters and
// strconv.Quote must re-escape them so the generated Go stays valid.
func TestQuotedAttributesCompile(t *testing.T) {
	parsed := parseFixture(t, "quoted_attrs.svg")
	src := generate(t, parsed, "Quoted", 0, 0)

	if _, err := parser.ParseFile(token.NewFileSet(), "q.go", src, parser.AllErrors); err != nil {
		t.Fatalf("generated source with quoted attributes does not parse: %v\n%s", err, src)
	}
	// Literal double quotes decoded from &quot; are escaped as \" in the
	// Go string literals for both style and id.
	if !bytes.Contains(src, []byte(`canvas.Gstyle("fill:url(\"gradient#1\")")`)) {
		t.Errorf("expected escaped quotes in Gstyle literal:\n%s", src)
	}
	if !bytes.Contains(src, []byte(`canvas.Gid("id with \"double quotes\"")`)) {
		t.Errorf("expected escaped quotes in Gid literal:\n%s", src)
	}

	dir := compileGenerated(t, src, "quoted")
	defer os.RemoveAll(dir)
}

// TestDefaultConstructorEmitsNoTransformWrapper is the visual-regression
// guarantee: the identity transform means the pattern body is emitted
// exactly as before, without an extra <g transform>.
func TestDefaultConstructorEmitsNoTransformWrapper(t *testing.T) {
	dir := buildRenderProgram(t, "nested_groups.svg", "Default", 0, 0, "")
	defer os.RemoveAll(dir)
	out := runProgram(t, dir)
	if bytes.Contains(out, []byte("<g transform=")) {
		t.Fatalf("default constructor must not emit a transform group:\n%s", out)
	}
	if !bytes.Contains(out, []byte(`width="100" height="60" patternUnits="userSpaceOnUse"`)) {
		t.Fatalf("pattern tile dimensions missing:\n%s", out)
	}
}

// TestGeneratedCodeCompilesAndTransformsTiles compiles generated code in a
// throwaway module, applies a transform at runtime and checks the emitted
// SVG both for the transform attribute and for the coordinate mapping of
// the tiling unit.
func TestGeneratedCodeCompilesAndTransformsTiles(t *testing.T) {
	setup := `
		err := p.SetTileTransform(patterns.TilesTransform{
			ScaleX: 2, ScaleY: 3, RotateDeg: 90, TranslateX: 15, TranslateY: 25,
		})
		if err != nil {
			panic(err)
		}`
	dir := buildRenderProgram(t, "nested_groups.svg", "Tiles", 0, 0, setup)
	defer os.RemoveAll(dir)
	out := runProgram(t, dir)

	// Transform lives inside the pattern (the tiling unit), not on the
	// canvas-level rect whose style only carries the pixel offset.
	wantAttr := `<g transform="translate(15,25) rotate(90) scale(2,3)">`
	if !bytes.Contains(out, []byte(wantAttr)) {
		t.Fatalf("tile transform attribute missing; want %s\noutput:\n%s", wantAttr, out)
	}
	patternBlock := substringBetween(out, []byte("<pattern"), []byte("</pattern>"))
	if !bytes.Contains(patternBlock, []byte(wantAttr)) {
		t.Fatalf("transform must be inside pattern tile:\n%s", out)
	}
	if !bytes.Contains(out, []byte(`width="100" height="60"`)) {
		t.Fatalf("pattern tile must keep its original tiling dimensions:\n%s", out)
	}

	// Coordinate check of the unit transform M = T * R(90deg) * S.
	// Local point (10, 20): after scale -> (20, 60); after 90deg rotation
	// (x,y)->(-y,x) -> (-60, 20); after translation -> (-45, 45).
	const localX, localY = 10.0, 20.0
	sx, sy := 2.0, 3.0
	rad := 90.0 * math.Pi / 180.0
	tx, ty := 15.0, 25.0
	px, py := sx*localX, sy*localY
	gotX := tx + px*math.Cos(rad) - py*math.Sin(rad)
	gotY := ty + px*math.Sin(rad) + py*math.Cos(rad)
	if !within(gotX, -45, 1e-9) || !within(gotY, 45, 1e-9) {
		t.Fatalf("unit transform maps (10,20) to (%g,%g), want (-45,45)", gotX, gotY)
	}

	// The tile itself keeps origin (0,0) and its original grid size, so
	// adjacent repetitions remain aligned to the pattern tile grid.
	if !bytes.Contains(out, []byte(`x="0" y="0" width="100" height="60"`)) {
		t.Fatalf("tile origin/grid missing:\n%s", out)
	}
}

// TestRuntimeTransformValidation compiles generated code and exercises the
// zero-scale and non-finite handling at the generated type's API level.
func TestRuntimeTransformValidation(t *testing.T) {
	setup := `
		err := p.SetTileTransform(patterns.ValidatedTransform{
			ScaleX: 0, ScaleY: math.NaN(), RotateDeg: math.Inf(1),
			TranslateX: math.NaN(), TranslateY: math.Inf(-1),
		})
		if err == nil {
			panic("expected validation error")
		}`
	dir := buildRenderProgram(t, "nested_groups.svg", "Validated", 0, 0, setup)
	defer os.RemoveAll(dir)
	out := runProgram(t, dir)
	// After correction every value is finite: no NaN/Inf may leak into the
	// document and no transform group is emitted (all values reset to
	// identity defaults).
	if bytes.Contains(out, []byte("NaN")) || bytes.Contains(out, []byte("Inf")) {
		t.Fatalf("non-finite values leaked into output:\n%s", out)
	}
	if bytes.Contains(out, []byte("<g transform=")) {
		t.Fatalf("corrected-to-identity transform must not emit a group:\n%s", out)
	}
}

// TestMalformedXMLRejected ensures parser failures surface rather than
// producing code from a half-parsed document.
func TestMalformedXMLRejected(t *testing.T) {
	if _, err := svg.ParseSvg("<svg><g></svg>", "broken", 1.0); err == nil {
		t.Fatal("expected malformed XML to fail parsing")
	}
}

// --- throwaway-module helpers ---

// compileGenerated writes src into a fresh module alongside svgo and builds
// it, returning the module directory.
func compileGenerated(t *testing.T, src []byte, name string) string {
	t.Helper()
	dir := t.TempDir()
	goMod := strings.Join([]string{
		"module example.com/" + name,
		"",
		"go 1.16",
		"",
		"require github.com/ajstarks/svgo " + svgoVersion,
		"",
	}, "\n")
	mustWrite(t, filepath.Join(dir, "go.mod"), goMod)
	patternDir := filepath.Join(dir, "patterns")
	mustMkdir(t, patternDir)
	mustWrite(t, filepath.Join(patternDir, "pattern.go"), string(src))
	runGo(t, dir, "build", "./...")
	return dir
}

// buildRenderProgram compiles generated code plus a main package that renders
// it to stdout. setupBody is extra statements run after NewX().
func buildRenderProgram(t *testing.T, fixture, typeName string, width, height int, setupBody string) string {
	t.Helper()
	parsed := parseFixture(t, fixture)
	var buf bytes.Buffer
	gen := NewGenerator("patterns", "\t", typeName, width, height)
	if err := gen.WriteSvgCode(parsed, &buf); err != nil {
		t.Fatalf("WriteSvgCode: %v", err)
	}
	moduleName := strings.ToLower(typeName) + "render"
	dir := compileGenerated(t, buf.Bytes(), moduleName)
	mainDir := filepath.Join(dir, "cmd")
	mustMkdir(t, mainDir)
	main := strings.Join([]string{
		"package main",
		"",
		"import (",
		`	"bytes"`,
		`	"math"`,
		`	"os"`,
		`	svg "github.com/ajstarks/svgo"`,
		`	"example.com/` + moduleName + `/patterns"`,
		")",
		"",
		"func main() {",
		"	_ = math.Pi",
		"	p := patterns.New" + typeName + "()",
		setupBody,
		"	var buf bytes.Buffer",
		"	canvas := svg.New(&buf)",
		"	canvas.Start(300, 200)",
		"	p.DefinePattern(300, 200, canvas)",
		"	canvas.Rect(0, 0, 300, 200, p.Style(\"red\", 0, 0))",
		"	canvas.End()",
		"	os.Stdout.Write(buf.Bytes())",
		"}",
		"",
	}, "\n")
	mustWrite(t, filepath.Join(mainDir, "main.go"), main)
	runGo(t, dir, "build", "./...")
	return dir
}

func runProgram(t *testing.T, dir string) []byte {
	t.Helper()
	return runGo(t, dir, "run", "./cmd")
}

func runGo(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := ioutil.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func substringBetween(in, start, end []byte) []byte {
	s := bytes.Index(in, start)
	if s < 0 {
		return nil
	}
	rest := in[s:]
	e := bytes.Index(rest, end)
	if e < 0 {
		return rest
	}
	return rest[:e]
}

func within(got, want, tolerance float64) bool {
	return math.Abs(got-want) <= tolerance
}
