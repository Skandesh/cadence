package main

import (
	"testing"

	"github.com/onflow/cadence/runtime/ast"
	"github.com/onflow/cadence/runtime/common"
	"github.com/onflow/cadence/tools/analysis"
	"github.com/stretchr/testify/require"
)

func TestReferenceToOptionalAnalyzer(t *testing.T) {

	t.Parallel()

	const code = `
      pub contract Test {
          pub fun test() {
              let xs: {Int: String} = {0: "zero"}
              let ref = &xs[0] as &String
          }
      }
	`

	location := common.StringLocation("test")
	locationID := location.ID()

	config := newAnalysisConfig(
		map[common.LocationID]string{
			locationID: code,
		},
		nil,
	)

	programs, err := analysis.Load(config, location)
	require.NoError(t, err)

	var diagnostics []analysis.Diagnostic

	programs[locationID].Run(
		[]*analysis.Analyzer{
			referenceToOptionalAnalyzer,
		},
		func(diagnostic analysis.Diagnostic) {
			diagnostics = append(diagnostics, diagnostic)
		},
	)

	require.Equal(
		t,
		[]analysis.Diagnostic{
			{
				Range: ast.Range{
					StartPos: ast.Position{Offset: 131, Line: 5, Column: 27},
					EndPos:   ast.Position{Offset: 133, Line: 5, Column: 29},
				},
				Location: location,
				Message:  "reference to optional",
			},
		},
		diagnostics,
	)
}

func TestDeprecatedKeyFunctionsAnalyzer(t *testing.T) {

	t.Parallel()

	const code = `
      pub contract Test {
          pub fun test(account: AuthAccount) {
              account.addPublicKey([])
              account.removePublicKey(0)
          }
      }
	`

	location := common.StringLocation("test")
	locationID := location.ID()

	config := newAnalysisConfig(
		map[common.LocationID]string{
			locationID: code,
		},
		nil,
	)

	programs, err := analysis.Load(config, location)
	require.NoError(t, err)

	var diagnostics []analysis.Diagnostic

	programs[locationID].Run(
		[]*analysis.Analyzer{
			deprecatedKeyFunctionsAnalyzer,
		},
		func(diagnostic analysis.Diagnostic) {
			diagnostics = append(diagnostics, diagnostic)
		},
	)

	require.Equal(
		t,
		[]analysis.Diagnostic{
			{
				Range: ast.Range{
					StartPos: ast.Position{Offset: 88, Line: 4, Column: 14},
					EndPos:   ast.Position{Offset: 111, Line: 4, Column: 37},
				},
				Location: location,
				Message:  "use of deprecated key management API: replace 'addPublicKey' with 'keys.add'",
			},
			{
				Range: ast.Range{
					StartPos: ast.Position{Offset: 127, Line: 5, Column: 14},
					EndPos:   ast.Position{Offset: 152, Line: 5, Column: 39},
				},
				Location: location,
				Message:  "use of deprecated key management API: replace 'removePublicKey' with 'keys.revoke'",
			},
		},
		diagnostics,
	)
}