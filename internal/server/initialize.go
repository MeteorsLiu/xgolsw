package server

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/goplus/mod/xgomod"
	"golang.org/x/text/language"

	"github.com/goplus/xgolsw/i18n"
	"github.com/goplus/xgolsw/protocol"
)

// InitializationOptions represents the initialization options that can be
// provided by the client.
type InitializationOptions struct {
	// Classfile configuration when gop.mod is not available
	Classfile *ClassfileInitOptions `json:"classfile,omitempty"`
}

// ClassfileInitOptions represents manual classfile configuration provided
// during initialization.
type ClassfileInitOptions struct {
	// ProjectClass is the main class name (e.g., "Game" for SPX)
	ProjectClass string `json:"projectClass,omitempty"`
	// ProjectExt is the file extension for project files (e.g., ".spx")
	ProjectExt string `json:"projectExt,omitempty"`
	// WorkClasses maps file extensions to class names (e.g., ".spx" -> "SpriteImpl")
	WorkClasses map[string]string `json:"workClasses,omitempty"`
	// PkgPaths are the package paths that define the classes (e.g., ["github.com/goplus/spx/v2"])
	PkgPaths []string `json:"pkgPaths,omitempty"`
}

// loadClassfileConfigFromFile attempts to load classfile configuration from
// .xgolsw.json file in the workspace root.
func loadClassfileConfigFromFile(workspaceRoot string) *ClassfileInitOptions {
	if workspaceRoot == "" {
		return nil
	}

	configPath := filepath.Join(workspaceRoot, ".xgolsw.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		// File doesn't exist or can't be read, return nil
		return nil
	}

	var config struct {
		Classfile *ClassfileInitOptions `json:"classfile,omitempty"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		// Invalid JSON, return nil
		return nil
	}

	return config.Classfile
}

// initialize handles the initialize request and sets up the server language preference
func (s *Server) initialize(params *InitializeParams) (*InitializeResult, error) {
	// Set language based on client locale
	s.setLanguageFromLocale(params.Locale)

	// Set workspace root URI first
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

	// Get workspace root path for loading config file
	var workspaceRoot string
	if s.workspaceRootURI != "" {
		if u, err := url.Parse(string(s.workspaceRootURI)); err == nil {
			workspaceRoot = u.Path
		}
	}

	// Try to load classfile config from .xgolsw.json file first
	fileConfig := loadClassfileConfigFromFile(workspaceRoot)

	// Parse initialization options from LSP client
	var initOptsConfig *ClassfileInitOptions
	if params.InitializationOptions != nil {
		var opts InitializationOptions
		// Convert interface{} to JSON and back to struct
		if data, err := json.Marshal(params.InitializationOptions); err == nil {
			if err := json.Unmarshal(data, &opts); err == nil {
				initOptsConfig = opts.Classfile
			}
		}
	}

	// Priority: initializationOptions > .xgolsw.json > gop.mod
	// Use initializationOptions if provided, otherwise use file config
	var manualConfig *ClassfileInitOptions
	if initOptsConfig != nil {
		manualConfig = initOptsConfig
	} else if fileConfig != nil {
		manualConfig = fileConfig
	}

	// Store manual configuration and reload classfile config if provided
	if manualConfig != nil {
		s.manualClassfileConfig = manualConfig
		var mod *xgomod.Module
		if s.workspaceRootFS != nil {
			mod = s.workspaceRootFS.Mod
		}
		s.classfileConfig = LoadClassfileConfig(mod, manualConfig)
	}

	// Create server capabilities with all supported features
	capabilities := ServerCapabilities{
		TextDocumentSync: float64(1), // Full document sync
		HoverProvider: &protocol.Or_ServerCapabilities_hoverProvider{
			Value: true,
		},
		CompletionProvider: &CompletionOptions{
			TriggerCharacters: []string{".", "("},
		},
		SignatureHelpProvider: &SignatureHelpOptions{
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
		DocumentLinkProvider: &DocumentLinkOptions{},
		DocumentFormattingProvider: &protocol.Or_ServerCapabilities_documentFormattingProvider{
			Value: true,
		},
		RenameProvider: &RenameOptions{
			PrepareProvider: true,
		},
		SemanticTokensProvider: &SemanticTokensOptions{
			Legend: SemanticTokensLegend{
				TokenTypes: []string{
					"namespace", "type", "interface", "struct", "enum", "enumMember",
					"variable", "parameter", "function", "method", "property",
					"keyword", "comment", "string", "number", "operator", "label",
				},
				TokenModifiers: []string{
					"declaration", "readonly", "static", "definition", "defaultLibrary",
				},
			},
			Full: &protocol.Or_SemanticTokensOptions_full{
				Value: true,
			},
		},
		InlayHintProvider: &InlayHintOptions{},
		DiagnosticProvider: &protocol.Or_ServerCapabilities_diagnosticProvider{
			Value: DiagnosticOptions{
				Identifier:            "xgolsw",
				InterFileDependencies: true,
				WorkspaceDiagnostics:  true,
			},
		},
		ExecuteCommandProvider: &ExecuteCommandOptions{
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
