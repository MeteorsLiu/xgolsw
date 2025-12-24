package server

import (
	"fmt"
	"go/doc"
	"strings"

	"github.com/goplus/xgolsw/xgo/xgoutil"
)

// See https://microsoft.github.io/language-server-protocol/specifications/lsp/3.18/specification#textDocument_hover
func (s *Server) textDocumentHover(params *HoverParams) (*Hover, error) {
	fmt.Printf("[Hover DEBUG] Request for URI: %s at line %d, char %d\n",
		params.TextDocument.URI, params.Position.Line, params.Position.Character)

	result, _, astFile, err := s.compileAndGetASTFileForDocumentURI(params.TextDocument.URI)
	if err != nil {
		fmt.Printf("[Hover DEBUG] Compile error: %v\n", err)
		return nil, err
	}
	if astFile == nil {
		fmt.Printf("[Hover DEBUG] astFile is nil\n")
		return nil, nil
	}
	if !astFile.Pos().IsValid() {
		fmt.Printf("[Hover DEBUG] astFile.Pos() is invalid\n")
		return nil, nil
	}
	position := ToPosition(result.proj, astFile, params.Position)
	fmt.Printf("[Hover DEBUG] position: %v\n", position)

	typeInfo, _ := result.proj.TypeInfo()
	if typeInfo == nil {
		fmt.Printf("[Hover DEBUG] typeInfo is nil\n")
		return nil, nil
	}

	ident := xgoutil.IdentAtPosition(result.proj.Fset, typeInfo, astFile, position)
	fmt.Printf("[Hover DEBUG] ident: %v\n", ident)
	if ident == nil {
		// Check if the position is within an import declaration.
		// If so, return the package documentation.
		rpkg := result.spxImportsAtASTFilePosition(astFile, position)
		if rpkg != nil {
			fmt.Printf("[Hover DEBUG] Found import package\n")
			return &Hover{
				Contents: MarkupContent{
					Kind:  Markdown,
					Value: doc.Synopsis(rpkg.Pkg.Doc),
				},
				Range: RangeForNode(result.proj, rpkg.Node),
			}, nil
		}
		fmt.Printf("[Hover DEBUG] No ident or import found\n")
		return nil, nil
	}

	spxDefs := result.spxDefinitionsForIdent(ident)
	fmt.Printf("[Hover DEBUG] spxDefs count: %d\n", len(spxDefs))
	if spxDefs == nil {
		fmt.Printf("[Hover DEBUG] spxDefs is nil\n")
		return nil, nil
	}

	var hoverContent strings.Builder
	for i, spxDef := range spxDefs {
		// Use Overview and Detail for Markdown content instead of HTML
		if spxDef.Overview != "" {
			hoverContent.WriteString("**")
			hoverContent.WriteString(spxDef.Overview)
			hoverContent.WriteString("**")
			hoverContent.WriteString("\n\n")
		}
		if spxDef.Detail != "" {
			hoverContent.WriteString(spxDef.Detail)
			if i < len(spxDefs)-1 {
				hoverContent.WriteString("\n\n---\n\n")
			}
		}
		fmt.Printf("[Hover DEBUG] spxDef[%d] Overview: %s, Detail length: %d\n", i, spxDef.Overview, len(spxDef.Detail))
	}

	result_hover := &Hover{
		Contents: MarkupContent{
			Kind:  Markdown,
			Value: hoverContent.String(),
		},
		Range: RangeForNode(result.proj, ident),
	}
	fmt.Printf("[Hover DEBUG] Returning hover with content: %s\n", hoverContent.String())
	return result_hover, nil
}
