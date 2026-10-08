package tweaks_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/nmsbonker/internal/modscript"
	"github.com/ushineko/nmsbonker/internal/tweaks"
)

/*
A script saved with Windows line endings means what the same script means with
Unix ones (spec 023 R7.2).

Scripts downloaded from Nexus are usually CRLF, and a Windows editor saves one
that way. Every built-in is loaded both ways and has to give the same change
tables, the same header and the same parameters; and a parameter override on
the CRLF copy has to take effect as it does on the LF one.
*/
func TestEveryBuiltInMeansTheSameWithCRLFLineEndings(t *testing.T) {
	for _, name := range tweaks.Names() {
		lf, ok := tweaks.Source(name)
		require.True(t, ok, name)
		require.NotContains(t, string(lf), "\r", "%s is checked out with LF (.gitattributes)", name)
		crlf := bytes.ReplaceAll(lf, []byte("\n"), []byte("\r\n"))

		a, err := modscript.LoadSource(t.Context(), name+".lua", lf)
		require.NoError(t, err, name)
		b, err := modscript.LoadSource(t.Context(), name+".lua", crlf)
		require.NoError(t, err, name)
		require.Equal(t, string(modscript.DumpJSON(a)), string(modscript.DumpJSON(b)), name)

		require.Equal(t, modscript.ParseHeader(lf), modscript.ParseHeader(crlf), name)
		params := modscript.Parameters(lf)
		require.Equal(t, params, modscript.Parameters(crlf), name)

		if len(params) == 0 {
			continue
		}
		p := params[0]
		overLF, err := modscript.Override(lf, p.Name, p.Min, p.Kind)
		require.NoError(t, err, name)
		overCRLF, err := modscript.Override(crlf, p.Name, p.Min, p.Kind)
		require.NoError(t, err, name)
		x, err := modscript.LoadSource(t.Context(), name+".lua", overLF)
		require.NoError(t, err, name)
		y, err := modscript.LoadSource(t.Context(), name+".lua", overCRLF)
		require.NoError(t, err, name)
		require.Equal(t, string(modscript.DumpJSON(x)), string(modscript.DumpJSON(y)), "%s with %s overridden", name, p.Name)
	}
}
