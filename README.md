# gen-svgo-pattern

Generate Go code using the [svgo library](https://github.com/ajstarks/svgo) from a given
SVG image file. The generated code will include an
[SVG pattern](https://developer.mozilla.org/en-US/docs/Web/SVG/Tutorial/Patterns)
to reproduce the given image.

## How to run

```sh
% make && bin/gen-svgo-pattern
```

Will display usage instructions:

```
Usage: bin/gen-svgo-pattern [options]

  -h int
    	Height of pattern in pixels; leave 0 to use width from input SVG
  -in string
    	Path to an SVG image file, e.g., ~/Pictures/my-pic.svg
  -name string
    	Name of Go type (default "MyPattern")
  -out string
    	Directory where the Go code should be written, e.g., ~/my-go-project/pkg/patterns/
  -pkg string
    	Name of Go package for new type (default "patterns")
  -tab string
    	Indentation to use in generated Go (default "\t")
  -w int
    	Width of pattern in pixels; leave 0 to use width from input SVG
```

Here's an example using the Jigsaw pattern from [Hero Patterns](https://www.heropatterns.com):

```sh
% make && bin/gen-svgo-pattern -in ~/Downloads/jigsaw.svg -name Jigsaw -out ~/code/go-brocade/pkg/patterns/
go build -o bin/gen-svgo-pattern gen-svgo-pattern.go
Reading:  /Users/cheshire137/Downloads/jigsaw.svg
Generating Go type Jigsaw...
Using width 192
Using height 192
Wrote /Users/cheshire137/code/go-brocade/pkg/patterns/jigsaw.go
```

The generated Go type will adhere to this interface:

```go
type Pattern interface {
	Name() string
	DefinePattern(width int, height int, canvas *svg.SVG)
	Style(color string, offsetX int, offsetY int) string
}
```

## Transforming a pattern

The same pattern can look different across packagings without regenerating
code. Each generated type exposes a transform value type and a
`SetTileTransform` method. The transform is applied to the **tiling unit
inside the `<pattern>` element** (via an inner `<g transform>`), not to the
final canvas, so the tile grid and its dimensions stay fixed while the motif
inside every repeated unit is scaled, rotated and translated.

```go
p := NewJigsaw()

// Denser tiling via a smaller motif, 45° rotation and a shift of the motif
// inside each unit.
err := p.SetTileTransform(JigsawTransform{
	ScaleX:    0.5,
	ScaleY:    0.5,
	RotateDeg: 45,
	TranslateX: 12,
	TranslateY: -8,
})
if err != nil {
	// One or more inputs were invalid; they were corrected to safe defaults
	// and the pattern is still in a usable state.
	log.Println(err)
}
```

Validation rules:

- `ScaleX`/`ScaleY` must be finite and non-zero. Zero, `NaN` or infinite
  values are reset to `1` and reported in the returned error.
- `RotateDeg` (degrees about the unit origin) and the translations must be
  finite; `NaN`/infinite values are reset to `0`.
- The default constructor uses the identity transform, in which case no
  transform group is emitted and the rendered SVG is identical to the
  pre-transform output.

`WriteSvgCode` now returns an error instead of silently producing broken or
mis-sized code for: nil input or output, negative dimensions, dimensions that
cannot be resolved from the SVG attributes or `viewBox`, SVGs without any
paths or groups, and generated source that is not valid Go. Repeated group
IDs are automatically suffixed (`dup`, `dup-2`, `dup-3`) so the generated
document never defines the same id twice, and attribute values containing
quotes are Go-escaped.

Sample of the generated Go code:

```go
package patterns

import (
	"fmt"

	svg "github.com/ajstarks/svgo"
)

type Jigsaw struct {
	ID            string
	maskID        string
	patternWidth  int
	patternHeight int
}

func NewJigsaw() *Jigsaw {
	return &Jigsaw{
		ID:            "Jigsaw",
		maskID:        "Jigsaw-mask",
		patternWidth:  192,
		patternHeight: 192,
	}
}

func (p *Jigsaw) DefinePattern(width int, height int, canvas *svg.SVG) {
	canvas.Def()
	canvas.Pattern(p.ID, 0, 0, p.patternWidth, p.patternHeight, "user", "stroke:white;stroke-linecap:square;stroke-width:1")

	canvas.Path("M192 15v2a11 11 0 0 0-11 11c0 1.94 1.16 4.75 2.53 6.11l2.36 2.36a6.93 6.93 0 0 1 1.22 7.56l-.43.84a8.08 8.08 0 0 1-6.66 4.13H145v35.02a6.1 6.1 0 0 0 3.03 4.87l.84.43c1.58.79 4 .4 5.24-.85l2.36-2.36a12.04 12.04 0 0 1 7.51-3.11 13 13 0 1 1 .02 26 12 12 0 0 1-7.53-3.11l-2.36-2.36a4.93 4.93 0 0 0-5.24-.85l-.84.43a6.1 6.1 0 0 0-3.03 4.87V143h35.02a8.08 8.08 0 0 1 6.66 4.13l.43.84a6.91 6.91 0 0 1-1.22 7.56l-2.36 2.36A10.06 10.06 0 0 0 181 164a11 11 0 0 0 11 11v2a13 13 0 0 1-13-13 12 12 0 0 1 3.11-7.53l2.36-2.36a4.93 4.93 0 0 0 .85-5.24l-.43-.84a6.1 6.1 0 0 0-4.87-3.03H145v35.02a8.08 8.08 0 0 1-4.13 6.66l-.84.43a6.91 6.91 0 0 1-7.56-1.22l-2.36-2.36A10.06 10.06 0 0 0 124 181a11 11 0 0 0-11 11h-2a13 13 0 0 1 13-13c2.47 0 5.79 1.37 7.53 3.11l2.36 2.36a4.94 4.94 0 0 0 5.24.85l.84-.43a6.1 6.1 0 0 0 3.03-4.87V145h-35.02a8.08 8.08 0 0 1-6.66-4.13l-.43-.84a6.91 6.91 0 0 1 1.22-7.56l2.36-2.36A10.06 10.06 0 0 0 107 124a11 11 0 0 0-22 0c0 1.94 1.16 4.75 2.53 6.11l2.36 2.36a6.93 6.93 0 0 1 1.22 7.56l-.43.84a8.08 8.08 0 0 1-6.66 4.13H49v35.02a6.1 6.1 0 0 0 3.03 4.87l.84.43c1.58.79 4 .4 5.24-.85l2.36-2.36a12.04 12.04 0 0 1 7.51-3.11A13 13 0 0 1 81 192h-2a11 11 0 0 0-11-11c-1.94 0-4.75 1.16-6.11 2.53l-2.36 2.36a6.93 6.93 0 0 1-7.56 1.22l-.84-.43a8.08 8.08 0 0 1-4.13-6.66V145H11.98a6.1 6.1 0 0 0-4.87 3.03l-.43.84c-.79 1.58-.4 4 .85 5.24l2.36 2.36a12.04 12.04 0 0 1 3.11 7.51A13 13 0 0 1 0 177v-2a11 11 0 0 0 11-11c0-1.94-1.16-4.75-2.53-6.11l-2.36-2.36a6.93 6.93 0 0 1-1.22-7.56l.43-.84a8.08 8.08 0 0 1 6.66-4.13H47v-35.02a6.1 6.1 0 0 0-3.03-4.87l-.84-.43c-1.59-.8-4-.4-5.24.85l-2.36 2.36A12 12 0 0 1 28 109a13 13 0 1 1 0-26c2.47 0 5.79 1.37 7.53 3.11l2.36 2.36a4.94 4.94 0 0 0 5.24.85l.84-.43A6.1 6.1 0 0 0 47 84.02V49H11.98a8.08 8.08 0 0 1-6.66-4.13l-.43-.84a6.91 6.91 0 0 1 1.22-7.56l2.36-2.36A10.06 10.06 0 0 0 11 28 11 11 0 0 0 0 17v-2a13 13 0 0 1 13 13c0 2.47-1.37 5.79-3.11 7.53l-2.36 2.36a4.94 4.94 0 0 0-.85 5.24l.43.84A6.1 6.1 0 0 0 11.98 47H47V11.98a8.08 8.08 0 0 1 4.13-6.66l.84-.43a6.91 6.91 0 0 1 7.56 1.22l2.36 2.36A10.06 10.06 0 0 0 68 11 11 11 0 0 0 79 0h2a13 13 0 0 1-13 13 12 12 0 0 1-7.53-3.11l-2.36-2.36a4.93 4.93 0 0 0-5.24-.85l-.84.43A6.1 6.1 0 0 0 49 11.98V47h35.02a8.08 8.08 0 0 1 6.66 4.13l.43.84a6.91 6.91 0 0 1-1.22 7.56l-2.36 2.36A10.06 10.06 0 0 0 85 68a11 11 0 0 0 22 0c0-1.94-1.16-4.75-2.53-6.11l-2.36-2.36a6.93 6.93 0 0 1-1.22-7.56l.43-.84a8.08 8.08 0 0 1 6.66-4.13H143V11.98a6.1 6.1 0 0 0-3.03-4.87l-.84-.43c-1.59-.8-4-.4-5.24.85l-2.36 2.36A12 12 0 0 1 124 13a13 13 0 0 1-13-13h2a11 11 0 0 0 11 11c1.94 0 4.75-1.16 6.11-2.53l2.36-2.36a6.93 6.93 0 0 1 7.56-1.22l.84.43a8.08 8.08 0 0 1 4.13 6.66V47h35.02a6.1 6.1 0 0 0 4.87-3.03l.43-.84c.8-1.59.4-4-.85-5.24l-2.36-2.36A12 12 0 0 1 179 28a13 13 0 0 1 13-13zM84.02 143a6.1 6.1 0 0 0 4.87-3.03l.43-.84c.8-1.59.4-4-.85-5.24l-2.36-2.36A12 12 0 0 1 83 124a13 13 0 1 1 26 0c0 2.47-1.37 5.79-3.11 7.53l-2.36 2.36a4.94 4.94 0 0 0-.85 5.24l.43.84a6.1 6.1 0 0 0 4.87 3.03H143v-35.02a8.08 8.08 0 0 1 4.13-6.66l.84-.43a6.91 6.91 0 0 1 7.56 1.22l2.36 2.36A10.06 10.06 0 0 0 164 107a11 11 0 0 0 0-22c-1.94 0-4.75 1.16-6.11 2.53l-2.36 2.36a6.93 6.93 0 0 1-7.56 1.22l-.84-.43a8.08 8.08 0 0 1-4.13-6.66V49h-35.02a6.1 6.1 0 0 0-4.87 3.03l-.43.84c-.79 1.58-.4 4 .85 5.24l2.36 2.36a12.04 12.04 0 0 1 3.11 7.51A13 13 0 1 1 83 68a12 12 0 0 1 3.11-7.53l2.36-2.36a4.93 4.93 0 0 0 .85-5.24l-.43-.84A6.1 6.1 0 0 0 84.02 49H49v35.02a8.08 8.08 0 0 1-4.13 6.66l-.84.43a6.91 6.91 0 0 1-7.56-1.22l-2.36-2.36A10.06 10.06 0 0 0 28 85a11 11 0 0 0 0 22c1.94 0 4.75-1.16 6.11-2.53l2.36-2.36a6.93 6.93 0 0 1 7.56-1.22l.84.43a8.08 8.08 0 0 1 4.13 6.66V143h35.02z")
	canvas.PatternEnd()

	canvas.Mask(p.maskID, 0, 0, width, height)
	canvas.Rect(0, 0, width, height, fmt.Sprintf("fill:url(#%s)", p.ID))
	canvas.MaskEnd()

	canvas.DefEnd()
}

func (p *Jigsaw) Style(color string) string {
	return fmt.Sprintf("mask:url(#%s);fill:%s", p.maskID, color)
}
```

## Thanks

- [rustyoz/svg](https://github.com/rustyoz/svg)
