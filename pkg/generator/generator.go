package generator

import (
	"bytes"
	"fmt"
	"go/format"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/rustyoz/svg"
)

type Generator struct {
	packageName string
	tab         string
	typeName    string
	width       int
	height      int

	// usedIDs tracks every identifier emitted into the generated code so
	// duplicate IDs can be made unique. It is populated per WriteSvgCode run.
	usedIDs map[string]bool
}

func NewGenerator(packageName string, tab string, typeName string, width int, height int) *Generator {
	return &Generator{
		packageName: packageName,
		tab:         tab,
		typeName:    typeName,
		width:       width,
		height:      height,
	}
}

// WriteSvgCode generates Go source that reproduces svgFile as an SVG pattern.
// It returns an error instead of emitting broken code for malformed input.
func (g *Generator) WriteSvgCode(svgFile *svg.Svg, outFile io.Writer) error {
	if outFile == nil {
		return fmt.Errorf("output writer is nil")
	}
	if svgFile == nil {
		return fmt.Errorf("SVG input is nil")
	}

	patternWidth, err := g.getWidth(svgFile)
	if err != nil {
		return err
	}
	fmt.Printf("Using width %d\n", patternWidth)

	patternHeight, err := g.getHeight(svgFile)
	if err != nil {
		return err
	}
	fmt.Printf("Using height %d\n", patternHeight)

	if countDrawableNodes(svgFile) == 0 {
		return fmt.Errorf("SVG %q contains no paths or groups to generate", svgFile.Name)
	}

	g.usedIDs = map[string]bool{}
	// The pattern name and mask ID share the document's ID namespace.
	g.uniqueID(g.typeName)
	g.uniqueID(g.typeName + "-mask")

	var buf bytes.Buffer
	g.writeFileHeader(&buf)
	g.writeConstructor(patternWidth, patternHeight, &buf)
	g.writeNameFunction(&buf)
	g.writeTransformType(&buf)
	g.writeSetTransformFunction(&buf)
	g.writeTransformStringFunction(&buf)
	g.writeDefinePatternFunction(svgFile, &buf)
	g.writeStyleFunction(&buf)

	// Formatting the whole document both normalizes indentation and acts as
	// a syntax check: malformed quoting would fail here instead of producing
	// Go code that does not compile.
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("generated code is not valid Go: %w", err)
	}
	if _, err := outFile.Write(formatted); err != nil {
		return fmt.Errorf("could not write generated code: %w", err)
	}
	return nil
}

func (g *Generator) writeFileHeader(outFile io.Writer) {
	outFile.Write([]byte(fmt.Sprintf("package %s\n\n", g.packageName)))
	g.writeImports(outFile)
	g.writeTypeDefinition(outFile)
}

func (g *Generator) writeImports(outFile io.Writer) {
	outFile.Write([]byte("import (\n"))
	outFile.Write([]byte(fmt.Sprintf("%s\"fmt\"\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s\"math\"\n\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%ssvg \"github.com/ajstarks/svgo\"\n", g.tab)))
	outFile.Write([]byte(")\n\n"))
}

func (g *Generator) writeTypeDefinition(outFile io.Writer) {
	outFile.Write([]byte(fmt.Sprintf("type %s struct {\n", g.typeName)))
	outFile.Write([]byte(fmt.Sprintf("%sname          string\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%smaskID        string\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%spatternWidth  int\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%spatternHeight int\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%sscaleX        float64\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%sscaleY        float64\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%srotateDeg     float64\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%stranslateX    float64\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%stranslateY    float64\n", g.tab)))
	outFile.Write([]byte("}\n\n"))
}

func (g *Generator) writeConstructor(patternWidth int, patternHeight int, outFile io.Writer) {
	outFile.Write([]byte(fmt.Sprintf("func New%s() *%s {\n", g.typeName, g.typeName)))
	outFile.Write([]byte(fmt.Sprintf("%sreturn &%s{\n", g.tab, g.typeName)))
	outFile.Write([]byte(fmt.Sprintf("%s%sname:          %s,\n", g.tab, g.tab, strconv.Quote(g.typeName))))
	outFile.Write([]byte(fmt.Sprintf("%s%smaskID:        %s,\n", g.tab, g.tab, strconv.Quote(g.typeName+"-mask"))))
	outFile.Write([]byte(fmt.Sprintf("%s%spatternWidth:  %d,\n", g.tab, g.tab, patternWidth)))
	outFile.Write([]byte(fmt.Sprintf("%s%spatternHeight: %d,\n", g.tab, g.tab, patternHeight)))
	outFile.Write([]byte(fmt.Sprintf("%s%sscaleX:        1,\n", g.tab, g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s%sscaleY:        1,\n", g.tab, g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s}\n", g.tab)))
	outFile.Write([]byte("}\n\n"))
}

func (g *Generator) writeNameFunction(outFile io.Writer) {
	outFile.Write([]byte(fmt.Sprintf("func (p *%s) Name() string {\n", g.typeName)))
	outFile.Write([]byte(fmt.Sprintf("%sreturn p.name\n", g.tab)))
	outFile.Write([]byte("}\n\n"))
}

// writeTransformType emits the per-pattern transform value type. Giving each
// generated type its own struct avoids identifier collisions when several
// generated patterns live in one package.
func (g *Generator) writeTransformType(outFile io.Writer) {
	outFile.Write([]byte(fmt.Sprintf("// %sTransform configures the affine transform applied to every\n", g.typeName)))
	outFile.Write([]byte("// tiling unit inside the pattern, before the pattern is tiled across the\n"))
	outFile.Write([]byte("// canvas. ScaleX/ScaleY default to 1, rotation is in degrees about the\n"))
	outFile.Write([]byte("// unit origin, and translation is in user units.\n"))
	outFile.Write([]byte(fmt.Sprintf("type %sTransform struct {\n", g.typeName)))
	outFile.Write([]byte(fmt.Sprintf("%sScaleX    float64\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%sScaleY    float64\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%sRotateDeg  float64\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%sTranslateX float64\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%sTranslateY float64\n", g.tab)))
	outFile.Write([]byte("}\n\n"))
}

// writeSetTransformFunction emits validation for the transform inputs.
// Non-finite values and zero scales are replaced with safe defaults and the
// first problem encountered is returned as an error, so the pattern always
// renders with a finite, invertible unit transform.
func (g *Generator) writeSetTransformFunction(outFile io.Writer) {
	t := g.typeName
	outFile.Write([]byte("// SetTileTransform applies t to every tiling unit. Invalid values are\n"))
	outFile.Write([]byte("// corrected to safe defaults: non-finite scales and zero scales become 1,\n"))
	outFile.Write([]byte("// non-finite rotation becomes 0 and non-finite translations become 0.\n"))
	outFile.Write([]byte("// The first validation problem is returned; the stored transform is\n"))
	outFile.Write([]byte("// always left in a usable state.\n"))
	outFile.Write([]byte(fmt.Sprintf("func (p *%s) SetTileTransform(t %sTransform) error {\n", t, t)))
	outFile.Write([]byte(fmt.Sprintf("%svar problem error\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%sif math.IsNaN(t.ScaleX) || math.IsInf(t.ScaleX, 0) || t.ScaleX == 0 {\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s%sproblem = fmt.Errorf(\"invalid ScaleX %%v: must be a finite, non-zero number\", t.ScaleX)\n", g.tab, g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s%st.ScaleX = 1\n", g.tab, g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s}\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%sif math.IsNaN(t.ScaleY) || math.IsInf(t.ScaleY, 0) || t.ScaleY == 0 {\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s%sproblem = fmt.Errorf(\"invalid ScaleY %%v: must be a finite, non-zero number\", t.ScaleY)\n", g.tab, g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s%st.ScaleY = 1\n", g.tab, g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s}\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%sif math.IsNaN(t.RotateDeg) || math.IsInf(t.RotateDeg, 0) {\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s%sproblem = fmt.Errorf(\"invalid RotateDeg %%v: must be a finite number\", t.RotateDeg)\n", g.tab, g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s%st.RotateDeg = 0\n", g.tab, g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s}\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%sif math.IsNaN(t.TranslateX) || math.IsInf(t.TranslateX, 0) {\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s%sproblem = fmt.Errorf(\"invalid TranslateX %%v: must be a finite number\", t.TranslateX)\n", g.tab, g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s%st.TranslateX = 0\n", g.tab, g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s}\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%sif math.IsNaN(t.TranslateY) || math.IsInf(t.TranslateY, 0) {\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s%sproblem = fmt.Errorf(\"invalid TranslateY %%v: must be a finite number\", t.TranslateY)\n", g.tab, g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s%st.TranslateY = 0\n", g.tab, g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s}\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%sp.scaleX, p.scaleY = t.ScaleX, t.ScaleY\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%sp.rotateDeg = t.RotateDeg\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%sp.translateX, p.translateY = t.TranslateX, t.TranslateY\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%sreturn problem\n", g.tab)))
	outFile.Write([]byte("}\n\n"))
}

// writeTransformStringFunction emits the helper that renders the stored
// transform as an SVG transform attribute. The identity transform renders as
// the empty string so the default constructor keeps its original output.
func (g *Generator) writeTransformStringFunction(outFile io.Writer) {
	t := g.typeName
	outFile.Write([]byte(fmt.Sprintf("func (p *%s) tileTransformString() string {\n", t)))
	outFile.Write([]byte(fmt.Sprintf("%sif p.scaleX == 1 && p.scaleY == 1 && p.rotateDeg == 0 && p.translateX == 0 && p.translateY == 0 {\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s%sreturn \"\"\n", g.tab, g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s}\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%sreturn fmt.Sprintf(\"translate(%%g,%%g) rotate(%%g) scale(%%g,%%g)\",\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s%sp.translateX, p.translateY, p.rotateDeg, p.scaleX, p.scaleY)\n", g.tab, g.tab)))
	outFile.Write([]byte("}\n\n"))
}

func (g *Generator) writeDefinePatternFunction(svgFile *svg.Svg, outFile io.Writer) {
	t := g.typeName
	outFile.Write([]byte(fmt.Sprintf("func (p *%s) DefinePattern(width int, height int, canvas *svg.SVG) {\n", t)))
	outFile.Write([]byte(fmt.Sprintf("%scanvas.Def()\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%scanvas.Pattern(p.name, 0, 0, p.patternWidth, p.patternHeight, \"user\", \"stroke:white;stroke-linecap:square;stroke-width:1\")\n\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%stransform := p.tileTransformString()\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%sif transform != \"\" {\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s%scanvas.Gtransform(transform)\n", g.tab, g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s}\n", g.tab)))
	if len(svgFile.Elements) > 0 {
		g.writeSvgElements(svgFile, outFile)
	} else if len(svgFile.Groups) > 0 {
		g.writeSvgGroups(svgFile, outFile)
	}
	outFile.Write([]byte(fmt.Sprintf("%sif transform != \"\" {\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s%scanvas.Gend()\n", g.tab, g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%s}\n\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%scanvas.PatternEnd()\n\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%scanvas.Mask(p.maskID, 0, 0, width, height)\n", g.tab)))
	fillStr := "fill:url(#%s)"
	outFile.Write([]byte(fmt.Sprintf("%scanvas.Rect(0, 0, width, height, fmt.Sprintf(%s, p.name))\n", g.tab, strconv.Quote(fillStr))))
	outFile.Write([]byte(fmt.Sprintf("%scanvas.MaskEnd()\n\n", g.tab)))
	outFile.Write([]byte(fmt.Sprintf("%scanvas.DefEnd()\n", g.tab)))
	outFile.Write([]byte("}\n\n"))
}

func (g *Generator) writeStyleFunction(outFile io.Writer) {
	outFile.Write([]byte(fmt.Sprintf("func (p *%s) Style(color string, offsetX int, offsetY int) string {\n", g.typeName)))
	styleStr := "mask:url(#%s);fill:%s;transform:translate(%dpx, %dpx)"
	outFile.Write([]byte(fmt.Sprintf("%sreturn fmt.Sprintf(%s, p.maskID, color, offsetX, offsetY)\n", g.tab, strconv.Quote(styleStr))))
	outFile.Write([]byte("}\n"))
}

func (g *Generator) getWidth(svgFile *svg.Svg) (int, error) {
	if g.width != 0 {
		if g.width < 0 {
			return 0, fmt.Errorf("invalid pattern width %d: must be greater than zero", g.width)
		}
		return g.width, nil
	}
	if w, ok := parseDimension(svgFile.Width); ok {
		return w, nil
	}
	if w, _, ok := parseViewBoxDimensions(svgFile.ViewBox); ok {
		return w, nil
	}
	return 0, fmt.Errorf("could not determine pattern width: SVG width %q is not a valid length and no usable viewBox was found", svgFile.Width)
}

func (g *Generator) getHeight(svgFile *svg.Svg) (int, error) {
	if g.height != 0 {
		if g.height < 0 {
			return 0, fmt.Errorf("invalid pattern height %d: must be greater than zero", g.height)
		}
		return g.height, nil
	}
	if h, ok := parseDimension(svgFile.Height); ok {
		return h, nil
	}
	if _, h, ok := parseViewBoxDimensions(svgFile.ViewBox); ok {
		return h, nil
	}
	return 0, fmt.Errorf("could not determine pattern height: SVG height %q is not a valid length and no usable viewBox was found", svgFile.Height)
}

// parseDimension parses plain-number and "px" SVG length attributes such as
// "192" or "48px". Percentages and other units cannot describe a user-space
// tile size, so they fail.
func parseDimension(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	raw = strings.TrimSuffix(raw, "px")
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 0, false
	}
	return int(math.Round(value)), true
}

// parseViewBoxDimensions extracts the width and height of a viewBox
// attribute, tolerating comma-separated values ("0 0 100 50" or
// "0,0,100,50"). Non-square viewBoxes are supported.
func parseViewBoxDimensions(viewBox string) (int, int, bool) {
	fields := strings.Fields(strings.ReplaceAll(viewBox, ",", " "))
	if len(fields) != 4 {
		return 0, 0, false
	}
	values := make([]float64, 4)
	for i, field := range fields {
		value, err := strconv.ParseFloat(field, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, 0, false
		}
		values[i] = value
	}
	width := int(math.Round(values[2]))
	height := int(math.Round(values[3]))
	if width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

// uniqueID registers id and, if it was already used, returns a
// collision-free variant with a numeric suffix.
func (g *Generator) uniqueID(id string) string {
	if !g.usedIDs[id] {
		g.usedIDs[id] = true
		return id
	}
	for counter := 2; ; counter++ {
		candidate := fmt.Sprintf("%s-%d", id, counter)
		if !g.usedIDs[candidate] {
			g.usedIDs[candidate] = true
			return candidate
		}
	}
}

// countDrawableNodes counts the paths and groups the generator is able to
// emit, so an SVG without any drawable content can be rejected.
func countDrawableNodes(svgFile *svg.Svg) int {
	count := len(svgFile.Groups)
	for _, el := range svgFile.Elements {
		switch el.(type) {
		case *svg.Group, *svg.Path:
			count++
		}
	}
	return count
}

func (g *Generator) writeSvgGroups(svgFile *svg.Svg, outFile io.Writer) {
	for i := range svgFile.Groups {
		g.writeSvgGroup(&svgFile.Groups[i], outFile)
	}
}

func (g *Generator) writeSvgElements(svgFile *svg.Svg, outFile io.Writer) {
	for _, el := range svgFile.Elements {
		group, ok := el.(*svg.Group)
		if ok {
			g.writeSvgGroup(group, outFile)
		} else {
			path, ok := el.(*svg.Path)
			if ok {
				g.writeSvgPath(path, outFile)
			}
		}
	}
}

func (g *Generator) writeSvgGroup(group *svg.Group, outFile io.Writer) {
	if len(group.Fill) > 0 || len(group.Stroke) > 0 || len(group.FillRule) > 0 {
		g.writeSvgGroupStyle(group, outFile)
	} else if len(group.ID) > 0 {
		outFile.Write([]byte(fmt.Sprintf("%scanvas.Gid(%s)\n", g.tab, strconv.Quote(g.uniqueID(group.ID)))))
	} else {
		outFile.Write([]byte(fmt.Sprintf("%scanvas.Group(\"\")\n", g.tab)))
	}

	for _, groupEl := range group.Elements {
		path, ok := groupEl.(*svg.Path)
		if ok {
			g.writeSvgPath(path, outFile)
		} else {
			nestedGroup, ok := groupEl.(*svg.Group)
			if ok {
				g.writeSvgGroup(nestedGroup, outFile)
			}
		}
	}

	outFile.Write([]byte(fmt.Sprintf("%scanvas.Gend()\n\n", g.tab)))
}

func (g *Generator) writeSvgGroupStyle(group *svg.Group, outFile io.Writer) {
	var style string
	if len(group.Fill) > 0 && len(group.Stroke) > 0 && len(group.FillRule) > 0 {
		style = fmt.Sprintf("fill:%s;stroke:%s;fill-rule:%s", group.Fill, group.Stroke, group.FillRule)
	} else if len(group.Fill) > 0 && len(group.Stroke) > 0 {
		style = fmt.Sprintf("fill:%s;stroke:%s", group.Fill, group.Stroke)
	} else if len(group.FillRule) > 0 && len(group.Stroke) > 0 {
		style = fmt.Sprintf("fill-rule:%s;stroke:%s", group.FillRule, group.Stroke)
	} else if len(group.FillRule) > 0 && len(group.Fill) > 0 {
		style = fmt.Sprintf("fill-rule:%s;fill:%s", group.FillRule, group.Fill)
	} else if len(group.Fill) > 0 {
		style = fmt.Sprintf("fill:%s", group.Fill)
	} else if len(group.FillRule) > 0 {
		style = fmt.Sprintf("fill-rule:%s", group.FillRule)
	} else {
		style = fmt.Sprintf("stroke:%s", group.Stroke)
	}
	outFile.Write([]byte(fmt.Sprintf("%scanvas.Gstyle(%s)\n", g.tab, strconv.Quote(style))))
}

func (g *Generator) writeSvgPath(path *svg.Path, outFile io.Writer) {
	outFile.Write([]byte(fmt.Sprintf("%scanvas.Path(%s)\n", g.tab, strconv.Quote(path.D))))
}
