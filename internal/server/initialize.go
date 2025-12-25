package server

import (
	"strings"

	"golang.org/x/text/language"

	"github.com/goplus/xgolsw/i18n"
	"github.com/goplus/xgolsw/protocol"
)

// initialize handles the initialize request and sets up the server language preference
func (s *Server) initialize(params *InitializeParams) (*InitializeResult, error) {
	// Set language based on client locale
	s.setLanguageFromLocale(params.Locale)

	// Set workspace root URI from initialize params
	if params.RootURI != "" {
		s.workspaceRootURI = params.RootURI
		// Ensure it ends with /
		if !strings.HasSuffix(string(s.workspaceRootURI), "/") {
			s.workspaceRootURI = DocumentURI(string(s.workspaceRootURI) + "/")
		}
	} else if params.RootPath != "" {
		// Fallback to rootPath if rootURI is not available
		s.workspaceRootURI = DocumentURI("file://" + params.RootPath + "/")
	}

	// Create server capabilities with all supported features
	capabilities := ServerCapabilities{
		TextDocumentSync: float64(1), // Full document sync
		HoverProvider: &protocol.Or_ServerCapabilities_hoverProvider{
			Value: true,
		},
		CompletionProvider: &protocol.CompletionOptions{
			// Trigger on common characters for better auto-completion experience
			// Letters trigger completion for identifiers, "." for member access, "(" for function calls
			TriggerCharacters: []string{".", "(", "\"", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z", "A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z"},
		},
		SignatureHelpProvider: &protocol.SignatureHelpOptions{
			TriggerCharacters: []string{"(", ","},
		},
		DeclarationProvider: &protocol.Or_ServerCapabilities_declarationProvider{
			Value: true,
		},
		DefinitionProvider: &protocol.Or_ServerCapabilities_definitionProvider{
			Value: true,
		},
		TypeDefinitionProvider: &protocol.Or_ServerCapabilities_typeDefinitionProvider{
			Value: true,
		},
		ImplementationProvider: &protocol.Or_ServerCapabilities_implementationProvider{
			Value: true,
		},
		ReferencesProvider: &protocol.Or_ServerCapabilities_referencesProvider{
			Value: true,
		},
		DocumentHighlightProvider: &protocol.Or_ServerCapabilities_documentHighlightProvider{
			Value: true,
		},
		// DocumentLinkProvider is disabled to avoid underline styling on identifiers.
		// VSCode renders all document links with underlines, which affects readability.
		// DocumentLinkProvider: &protocol.DocumentLinkOptions{},
		DocumentFormattingProvider: &protocol.Or_ServerCapabilities_documentFormattingProvider{
			Value: true,
		},
		RenameProvider: &protocol.RenameOptions{
			PrepareProvider: true,
		},
		// SemanticTokensProvider is disabled to avoid underline styling issues
		// in VSCode dark themes. TextMate grammar provides sufficient highlighting.
		// SemanticTokensProvider: &protocol.SemanticTokensOptions{
		// 	Legend: protocol.SemanticTokensLegend{
		// 		TokenTypes: []string{
		// 			"namespace", "type", "interface", "struct", "enum", "enumMember",
		// 			"variable", "parameter", "function", "method", "property",
		// 			"keyword", "comment", "string", "number", "operator", "label",
		// 		},
		// 		TokenModifiers: []string{
		// 			"declaration", "readonly", "static", "definition", "defaultLibrary",
		// 		},
		// 	},
		// 	Full: &protocol.Or_SemanticTokensOptions_full{
		// 		Value: true,
		// 	},
		// },
		InlayHintProvider: &protocol.InlayHintOptions{},
		DiagnosticProvider: &protocol.Or_ServerCapabilities_diagnosticProvider{
			Value: protocol.DiagnosticOptions{
				Identifier:            "xgolsw",
				InterFileDependencies: true,
				WorkspaceDiagnostics:  true,
			},
		},
		ExecuteCommandProvider: &protocol.ExecuteCommandOptions{
			Commands: []string{
				"xgo.renameResource",
			},
		},
	}

	return &InitializeResult{
		Capabilities: capabilities,
		ServerInfo: &ServerInfo{
			Name:    "XGo Language Server",
			Version: "0.1.0",
		},
	}, nil
}

// setLanguageFromLocale sets the server language based on the client locale
func (s *Server) setLanguageFromLocale(locale string) {
	// Default to English
	s.language = i18n.LanguageEN

	// Parse the locale using golang.org/x/text/language
	tag, err := language.Parse(locale)
	if err != nil {
		// If parsing fails, keep the default language
		return
	}

	// Check if the base language is Chinese
	base, _ := tag.Base()
	chineseBase, _ := language.Chinese.Base()
	if base == chineseBase {
		s.language = i18n.LanguageCN
	}
}

// translate translates a diagnostic message based on the server's current language
func (s *Server) translate(message string) string {
	return i18n.Translate(message, s.language)
}
