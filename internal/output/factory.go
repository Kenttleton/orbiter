package output

import "os"

// NewRenderer returns the appropriate Renderer for the given format string.
// format must be FormatStyled or FormatJSON. Defaults to FormatStyled.
func NewRenderer(format string, verbose bool) Renderer {
	switch format {
	case FormatJSON:
		return NewJSONRenderer(os.Stdout, verbose)
	default:
		return NewStyledRenderer(os.Stdout, verbose)
	}
}
