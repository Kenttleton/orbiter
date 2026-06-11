package output

import (
	"io"
	"os"
)

// NewRenderer returns the appropriate Renderer for the given format string.
// format must be FormatStyled or FormatJSON. Defaults to FormatStyled.
func NewRenderer(format string, verbose bool) Renderer {
	return NewRendererTo(format, verbose, os.Stdout)
}

// NewRendererTo creates a Renderer writing to out instead of os.Stdout.
func NewRendererTo(format string, verbose bool, out io.Writer) Renderer {
	switch format {
	case FormatJSON:
		return NewJSONRenderer(out, verbose)
	default:
		return NewStyledRenderer(out, verbose)
	}
}
