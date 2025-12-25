package server

import (
	"go/types"
	"strings"

	xgoast "github.com/goplus/xgo/ast"
	xgotoken "github.com/goplus/xgo/token"
	"github.com/goplus/xgolsw/internal/pkgdata"
	"github.com/goplus/xgolsw/pkgdoc"
	"github.com/goplus/xgolsw/protocol"
	"github.com/goplus/xgolsw/xgo/xgoutil"
)

// See https://microsoft.github.io/language-server-protocol/specifications/lsp/3.18/specification/#textDocument_signatureHelp
func (s *Server) textDocumentSignatureHelp(params *SignatureHelpParams) (*SignatureHelp, error) {
	result, _, astFile, err := s.compileAndGetASTFileForDocumentURI(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	if astFile == nil {
		return nil, nil
	}

	// Get the token.Pos (integer position) from LSP position
	pos := PosAt(result.proj, astFile, params.Position)

	typeInfo, _ := result.proj.TypeInfo()
	if typeInfo == nil {
		return nil, nil
	}

	// Try to find the identifier at the current position first
	position := ToPosition(result.proj, astFile, params.Position)
	ident := xgoutil.IdentAtPosition(result.proj.Fset, typeInfo, astFile, position)

	// If no identifier at current position, try to find the enclosing call expression
	// This handles the case when cursor is at "say(" - we need to look backwards to find "say"
	if ident == nil && pos > 0 {
		// Try position - 1 (just before current position, likely the opening paren or space)
		prevPos := pos - 1
		prevPosition := result.proj.Fset.Position(prevPos)
		ident = xgoutil.IdentAtPosition(result.proj.Fset, typeInfo, astFile, prevPosition)

		// Also try position - 2, -3, etc. to handle cases like "say  " (multiple spaces)
		if ident == nil {
			for offset := 2; offset <= 5 && pos-xgotoken.Pos(offset) > 0; offset++ {
				testPos := pos - xgotoken.Pos(offset)
				testPosition := result.proj.Fset.Position(testPos)
				ident = xgoutil.IdentAtPosition(result.proj.Fset, typeInfo, astFile, testPosition)
				if ident != nil {
					break
				}
			}
		}
	}

	// If still no identifier, try scanning backwards to find the function name
	if ident == nil {
		// Look for CallExpr or command-style call in the AST path
		path, _ := xgoutil.PathEnclosingInterval(astFile, pos-1, pos)
		for i, node := range path {
			// Check if this is a CallExpr
			if callExpr, ok := node.(*xgoast.CallExpr); ok {
				// Check if cursor is within the CallExpr range
				// For signature help, we only want the innermost CallExpr that contains the cursor
				// Skip if this is not the first CallExpr we found (i.e., there's a more inner one)
				if i > 0 {
					// Check if there's a more inner CallExpr
					hasInnerCallExpr := false
					for j := 0; j < i; j++ {
						if _, ok := path[j].(*xgoast.CallExpr); ok {
							hasInnerCallExpr = true
							break
						}
					}
					if hasInnerCallExpr {
						continue
					}
				}

				// The Fun field of CallExpr is the function being called
				// It could be an Ident (simple call like "say()") or a SelectorExpr (method call like "obj.method()")
				switch fun := callExpr.Fun.(type) {
				case *xgoast.Ident:
					ident = fun
					break
				case *xgoast.SelectorExpr:
					ident = fun.Sel
					break
				}
			}

			if ident != nil {
				break
			}
		}
	}

	if ident == nil {
		return nil, nil
	}

	obj := typeInfo.ObjectOf(ident)
	if obj == nil {
		return nil, nil
	}

	// Check if the object is a function
	fun, ok := obj.(*types.Func)
	if !ok {
		return nil, nil
	}
	sig, ok := fun.Type().(*types.Signature)
	if !ok {
		return nil, nil
	}

	// Get package documentation for the function
	var pkgDoc *pkgdoc.PkgDoc
	if xgoutil.IsInMainPkg(fun) {
		pkgDoc, _ = result.proj.PkgDoc()
	} else if !xgoutil.IsInBuiltinPkg(fun) {
		pkgPath := xgoutil.PkgPath(fun.Pkg())
		pkgDoc, _ = pkgdata.GetPkgDoc(pkgPath)
	}

	// Get the definition to extract documentation
	var documentation string
	if pkgDoc != nil || xgoutil.IsInBuiltinPkg(fun) {
		def := GetSpxDefinitionForFunc(fun, "", pkgDoc)
		if def.Overview != "" || def.Detail != "" {
			// Combine overview and detail for signature documentation
			if def.Overview != "" && def.Detail != "" {
				documentation = def.Overview + "\n\n" + def.Detail
			} else if def.Overview != "" {
				documentation = def.Overview
			} else {
				documentation = def.Detail
			}
		}
	}

	var paramsInfo []ParameterInformation
	sigParams := sig.Params()
	for i := 0; i < sigParams.Len(); i++ {
		param := sigParams.At(i)
		paramsInfo = append(paramsInfo, ParameterInformation{
			Label: param.Name() + " " + GetSimplifiedTypeString(param.Type()),
		})
	}

	label := fun.Name() + "("
	if sig.Params().Len() > 0 {
		var paramLabels []string
		for _, p := range paramsInfo {
			paramLabels = append(paramLabels, p.Label)
		}
		label += strings.Join(paramLabels, ", ")
	}
	label += ")"

	if results := sig.Results(); results != nil && results.Len() > 0 {
		var returnTypes []string
		for i := 0; i < results.Len(); i++ {
			returnVar := results.At(i)
			returnTypes = append(returnTypes, GetSimplifiedTypeString(returnVar.Type()))
		}
		label += " (" + strings.Join(returnTypes, ", ") + ")"
	}

	signatureInfo := SignatureInformation{
		Label:      label,
		Parameters: paramsInfo,
	}

	// Add documentation if available
	if documentation != "" {
		signatureInfo.Documentation = &protocol.Or_SignatureInformation_documentation{
			Value: MarkupContent{
				Kind:  Markdown,
				Value: documentation,
			},
		}
	}

	return &SignatureHelp{
		Signatures: []SignatureInformation{signatureInfo},
	}, nil
}
