package output

import (
	"io"
	"os"
)

// NewRenderer returns the appropriate Renderer for the given format string.
// format must be FormatStyled or FormatJSON. Defaults to FormatStyled.
func NewRenderer(format string, trace bool) Renderer {
	return NewRendererTo(format, trace, os.Stdout)
}

// NewRendererTo creates a Renderer writing to out instead of os.Stdout.
func NewRendererTo(format string, trace bool, out io.Writer) Renderer {
	switch format {
	case FormatJSON:
		return NewJSONRenderer(out, trace)
	default:
		return NewStyledRenderer(out, trace)
	}
}
