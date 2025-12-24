package server

import (
	"fmt"

	"github.com/goplus/xgolsw/xgo/xgoutil"
)

// See https://microsoft.github.io/language-server-protocol/specifications/lsp/3.18/specification/#textDocument_prepareRename
func (s *Server) textDocumentPrepareRename(params *PrepareRenameParams) (*Range, error) {
	proj := s.getProjWithFile()
	if proj == nil {
		return nil, nil
	}
	spxFile, err := s.fromDocumentURI(params.TextDocument.URI)
	if err != nil {
		return nil, fmt.Errorf("failed to get file path from document URI %q: %w", params.TextDocument.URI, err)
	}

	astFile, _ := proj.ASTFile(spxFile)
	if astFile == nil {
		return nil, nil
	}
	position := ToPosition(proj, astFile, params.Position)

	typeInfo, _ := proj.TypeInfo()
	if typeInfo == nil {
		return nil, nil
	}
	astPkg, _ := proj.ASTPackage()

	ident := xgoutil.IdentAtPosition(proj.Fset, typeInfo, astFile, position)
	if ident == nil || xgoutil.IsBlankIdent(ident) || xgoutil.IsSyntheticThisIdent(proj.Fset, typeInfo, astPkg, ident) {
		return nil, nil
	}

	obj := typeInfo.ObjectOf(ident)
	if !xgoutil.IsRenameable(obj) {
		return nil, nil
	}
	defIdent := typeInfo.ObjToDef[obj]
	if defIdent == nil || defIdent.Implicit() || xgoutil.NodeTokenFile(proj.Fset, defIdent) == nil {
		return nil, nil
	}

	return ToPtr(RangeForNode(proj, ident)), nil
}

// See https://microsoft.github.io/language-server-protocol/specifications/lsp/3.18/specification/#textDocument_rename
func (s *Server) textDocumentRename(params *RenameParams) (*WorkspaceEdit, error) {
	result, _, astFile, err := s.compileAndGetASTFileForDocumentURI(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	if astFile == nil {
		return nil, nil
	}
	position := ToPosition(result.proj, astFile, params.Position)

	typeInfo, _ := result.proj.TypeInfo()
	if typeInfo == nil {
		return nil, nil
	}
	astPkg, _ := result.proj.ASTPackage()

	ident := xgoutil.IdentAtPosition(result.proj.Fset, typeInfo, astFile, position)
	if ident == nil || xgoutil.IsBlankIdent(ident) || xgoutil.IsSyntheticThisIdent(result.proj.Fset, typeInfo, astPkg, ident) {
		return nil, nil
	}

	obj := typeInfo.ObjectOf(ident)
	if !xgoutil.IsRenameable(obj) {
		return nil, nil
	}
	defIdent := typeInfo.ObjToDef[obj]
	if defIdent == nil || xgoutil.NodeTokenFile(result.proj.Fset, defIdent) == nil {
		return nil, fmt.Errorf("failed to find definition of object %q", obj.Name())
	}

	defLoc := s.locationForNode(result.proj, defIdent)

	workspaceEdit := WorkspaceEdit{
		Changes: map[DocumentURI][]TextEdit{
			defLoc.URI: {
				{
					Range:   RangeForNode(result.proj, defIdent),
					NewText: params.NewName,
				},
			},
		},
	}
	for _, refLoc := range s.findReferenceLocations(result, obj) {
		workspaceEdit.Changes[refLoc.URI] = append(workspaceEdit.Changes[refLoc.URI], TextEdit{
			Range:   refLoc.Range,
			NewText: params.NewName,
		})
	}
	return &workspaceEdit, nil
}
