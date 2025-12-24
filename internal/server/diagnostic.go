package server

// See https://microsoft.github.io/language-server-protocol/specifications/lsp/3.18/specification#textDocument_diagnostic
func (s *Server) textDocumentDiagnostic(params *DocumentDiagnosticParams) (*DocumentDiagnosticReport, error) {
	result, err := s.compile()
	if err != nil {
		return nil, err
	}

	items := result.diagnostics[params.TextDocument.URI]
	if items == nil {
		items = []Diagnostic{} // Return empty array instead of nil
	}

	return &DocumentDiagnosticReport{Value: RelatedFullDocumentDiagnosticReport{
		FullDocumentDiagnosticReport: FullDocumentDiagnosticReport{
			Kind:  string(DiagnosticFull),
			Items: items,
		},
	}}, nil
}

// See https://microsoft.github.io/language-server-protocol/specifications/lsp/3.18/specification#workspace_diagnostic
func (s *Server) workspaceDiagnostic(params *WorkspaceDiagnosticParams) (*WorkspaceDiagnosticReport, error) {
	result, err := s.compile()
	if err != nil {
		return nil, err
	}

	items := make([]WorkspaceDocumentDiagnosticReport, 0, len(result.diagnostics))
	for file, fileDiags := range result.diagnostics {
		if fileDiags == nil {
			fileDiags = []Diagnostic{} // Return empty array instead of nil
		}
		items = append(items, WorkspaceDocumentDiagnosticReport{
			Value: WorkspaceFullDocumentDiagnosticReport{
				URI: DocumentURI(file),
				FullDocumentDiagnosticReport: FullDocumentDiagnosticReport{
					Kind:  string(DiagnosticFull),
					Items: fileDiags,
				},
			},
		})
	}
	return &WorkspaceDiagnosticReport{Items: items}, nil
}
