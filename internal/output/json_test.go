package output_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Kenttleton/orbiter/internal/output"
	"github.com/stretchr/testify/require"
)

func TestJSONRendererEncodesValue(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewJSONRenderer(&buf, false)

	type payload struct {
		Name string `json:"name"`
		ID   int    `json:"id"`
	}
	err := r.JSON(payload{Name: "payment-api", ID: 42})
	require.NoError(t, err)

	var got payload
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Equal(t, "payment-api", got.Name)
	require.Equal(t, 42, got.ID)
}

func TestJSONRendererInfoWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewJSONRenderer(&buf, false)
	r.Info("this should not appear in JSON mode")
	require.Empty(t, buf.String())
}

func TestJSONRendererIsVerbose(t *testing.T) {
	var buf bytes.Buffer
	r := output.NewJSONRenderer(&buf, true)
	require.True(t, r.IsVerbose())

	r2 := output.NewJSONRenderer(&buf, false)
	require.False(t, r2.IsVerbose())
}
