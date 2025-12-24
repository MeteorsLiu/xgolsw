package server

// See https://microsoft.github.io/language-server-protocol/specifications/lsp/3.18/specification#textDocument_diagnostic
func (s *Server) textDocumentDiagnostic(params *DocumentDiagnosticParams) (*DocumentDiagnosticReport, error) {
	result, err := s.compile()
	if err != nil {
		// Return empty diagnostics on compile error instead of nil
		return &DocumentDiagnosticReport{Value: RelatedFullDocumentDiagnosticReport{
			FullDocumentDiagnosticReport: FullDocumentDiagnosticReport{
				Kind:  string(DiagnosticFull),
				Items: []Diagnostic{},
			},
		}}, nil
	}

	// Get diagnostics for the file, or empty array if none
	items := result.diagnostics[params.TextDocument.URI]
	if items == nil {
		items = []Diagnostic{}
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
		// Return empty diagnostics on compile error instead of nil
		return &WorkspaceDiagnosticReport{Items: []WorkspaceDocumentDiagnosticReport{}}, nil
	}

	items := make([]WorkspaceDocumentDiagnosticReport, 0, len(result.diagnostics))
	for file, fileDiags := range result.diagnostics {
		// Ensure we always have an array, not nil
		if fileDiags == nil {
			fileDiags = []Diagnostic{}
		}

		items = append(items, WorkspaceDocumentDiagnosticReport{
			Value: WorkspaceFullDocumentDiagnosticReport{
				URI:     DocumentURI(file),
				Version: 0, // Default version since we don't track document versions
				FullDocumentDiagnosticReport: FullDocumentDiagnosticReport{
					Kind:  string(DiagnosticFull),
					Items: fileDiags,
				},
			},
		})
	}
	return &WorkspaceDiagnosticReport{Items: items}, nil
}
