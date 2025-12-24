package server

import (
	"fmt"
	"go/types"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/goplus/gogen"
	xgoast "github.com/goplus/xgo/ast"
	xgoscanner "github.com/goplus/xgo/scanner"
	xgotoken "github.com/goplus/xgo/token"
	"github.com/goplus/xgo/x/typesutil"
	"github.com/goplus/xgolsw/internal/analysis/ast/inspector"
	"github.com/goplus/xgolsw/internal/analysis/passes/inspect"
	"github.com/goplus/xgolsw/internal/analysis/protocol"
	"github.com/goplus/xgolsw/internal/pkgdata"
	"github.com/goplus/xgolsw/pkgdoc"
	"github.com/goplus/xgolsw/xgo"
	"github.com/goplus/xgolsw/xgo/xgoutil"
	"github.com/qiniu/x/errors"
)

// errNoMainSpxFile is the error returned when no valid main classfile is found
// in the main package while compiling (e.g., main.spx, main.gmx, main.gox).
var errNoMainSpxFile = errors.New("no valid main classfile found in main package")

// compileResult contains the compile results and additional information from
// the compile process.
type compileResult struct {
	proj *xgo.Project

	// classfileConfig is the configuration for the classfile project.
	classfileConfig *ClassfileConfig

	// mainSpxFile is the main.spx file path.
	mainSpxFile string

	// diagnostics stores diagnostic messages for each document.
	diagnostics map[DocumentURI][]Diagnostic

	// seenDiagnostics stores already reported diagnostics to avoid duplicates.
	seenDiagnostics map[DocumentURI]map[string]struct{}

	// hasErrorSeverityDiagnostic is true if the compile result has any
	// diagnostics with error severity.
	hasErrorSeverityDiagnostic bool
}

// newCompileResult creates a new [compileResult].
func newCompileResult(proj *xgo.Project, classfileConfig *ClassfileConfig) *compileResult {
	return &compileResult{
		proj:            proj,
		classfileConfig: classfileConfig,
		diagnostics:     make(map[DocumentURI][]Diagnostic),
	}
}

// spxDefinitionsFor returns all spx definitions for the given object. It
// returns multiple definitions only if the object is an XGo overloadable
// function.
func (r *compileResult) spxDefinitionsFor(obj types.Object, selectorTypeName string) []SpxDefinition {
	if obj == nil {
		return nil
	}
	if xgoutil.IsInBuiltinPkg(obj) {
		return []SpxDefinition{GetSpxDefinitionForBuiltinObj(obj)}
	}

	var pkgDoc *pkgdoc.PkgDoc
	if xgoutil.IsInMainPkg(obj) {
		pkgDoc, _ = r.proj.PkgDoc()
	} else {
		pkgPath := xgoutil.PkgPath(obj.Pkg())
		pkgDoc, _ = pkgdata.GetPkgDoc(pkgPath)
	}

	typeInfo, _ := r.proj.TypeInfo()
	switch obj := obj.(type) {
	case *types.Var:
		astPkg, _ := r.proj.ASTPackage()
		forceVar := xgoutil.IsDefinedInClassFieldsDecl(r.proj.Fset, typeInfo, astPkg, obj)
		return []SpxDefinition{GetSpxDefinitionForVar(obj, selectorTypeName, forceVar, pkgDoc, r.classfileConfig)}
	case *types.Const:
		return []SpxDefinition{GetSpxDefinitionForConst(obj, pkgDoc)}
	case *types.TypeName:
		return []SpxDefinition{GetSpxDefinitionForType(obj, pkgDoc)}
	case *types.Func:
		if typeInfo != nil {
			if defIdent := typeInfo.ObjToDef[obj]; defIdent != nil && defIdent.Implicit() {
				return nil
			}
		}
		if xgoutil.IsUnexpandableXGoOverloadableFunc(obj) {
			return nil
		}
		if funcOverloads := xgoutil.ExpandXGoOverloadableFunc(obj); funcOverloads != nil {
			defs := make([]SpxDefinition, 0, len(funcOverloads))
			for _, funcOverload := range funcOverloads {
				defs = append(defs, GetSpxDefinitionForFunc(funcOverload, selectorTypeName, pkgDoc, r.classfileConfig))
			}
			return defs
		}
		return []SpxDefinition{GetSpxDefinitionForFunc(obj, selectorTypeName, pkgDoc, r.classfileConfig)}
	case *types.PkgName:
		return []SpxDefinition{GetSpxDefinitionForPkg(obj, pkgDoc)}
	}
	return nil
}

// spxDefinitionsForIdent returns all spx definitions for the given identifier.
// It returns multiple definitions only if the identifier is an XGo
// overloadable function.
func (r *compileResult) spxDefinitionsForIdent(ident *xgoast.Ident) []SpxDefinition {
	if ident.Name == "_" {
		return nil
	}
	typeInfo, _ := r.proj.TypeInfo()
	if typeInfo == nil {
		return nil
	}
	return r.spxDefinitionsFor(typeInfo.ObjectOf(ident), SelectorTypeNameForIdent(r.proj, ident, r.classfileConfig))
}

// spxDefinitionsForNamedStruct returns all spx definitions for the given named
// struct type.
func (r *compileResult) spxDefinitionsForNamedStruct(named *types.Named) []SpxDefinition {
	var defs []SpxDefinition
	xgoutil.WalkStruct(named, func(member types.Object, selector *types.Named) bool {
		defs = append(defs, r.spxDefinitionsFor(member, selector.Obj().Name())...)
		return true
	})
	return defs
}

// spxDefinitionForField returns the spx definition for the given field and
// optional selector type name.
func (r *compileResult) spxDefinitionForField(field *types.Var, selectorTypeName string) SpxDefinition {
	var (
		forceVar bool
		pkgDoc   *pkgdoc.PkgDoc
	)
	if typeInfo, _ := r.proj.TypeInfo(); typeInfo != nil {
		if defIdent := typeInfo.ObjToDef[field]; defIdent != nil {
			if selectorTypeName == "" {
				selectorTypeName = SelectorTypeNameForIdent(r.proj, defIdent, r.classfileConfig)
			}
			astPkg, _ := r.proj.ASTPackage()
			forceVar = xgoutil.IsDefinedInClassFieldsDecl(r.proj.Fset, typeInfo, astPkg, field)
			pkgDoc, _ = r.proj.PkgDoc()
		}
	} else {
		pkg := field.Pkg()
		pkgPath := xgoutil.PkgPath(pkg)
		pkgDoc, _ = pkgdata.GetPkgDoc(pkgPath)
	}
	return GetSpxDefinitionForVar(field, selectorTypeName, forceVar, pkgDoc, r.classfileConfig)
}

// spxDefinitionForMethod returns the spx definition for the given method and
// optional selector type name.
func (r *compileResult) spxDefinitionForMethod(method *types.Func, selectorTypeName string) SpxDefinition {
	var pkgDoc *pkgdoc.PkgDoc
	if typeInfo, _ := r.proj.TypeInfo(); typeInfo != nil {
		if defIdent := typeInfo.ObjToDef[method]; defIdent != nil {
			if selectorTypeName == "" {
				selectorTypeName = SelectorTypeNameForIdent(r.proj, defIdent, r.classfileConfig)
			}
			pkgDoc, _ = r.proj.PkgDoc()
		}
	} else {
		if idx := strings.LastIndex(selectorTypeName, "."); idx >= 0 {
			selectorTypeName = selectorTypeName[idx+1:]
		}
		pkg := method.Pkg()
		pkgPath := xgoutil.PkgPath(pkg)
		pkgDoc, _ = pkgdata.GetPkgDoc(pkgPath)
	}
	return GetSpxDefinitionForFunc(method, selectorTypeName, pkgDoc, r.classfileConfig)
}

// isInSpxEventHandler checks if the given position is inside an spx event
// handler callback.
func (r *compileResult) isInSpxEventHandler(pos xgotoken.Pos) bool {
	astPkg, _ := r.proj.ASTPackage()
	astFile := xgoutil.PosASTFile(r.proj.Fset, astPkg, pos)
	if astFile == nil {
		return false
	}
	typeInfo, _ := r.proj.TypeInfo()
	if typeInfo == nil {
		return false
	}

	var isIn bool
	xgoutil.WalkPathEnclosingInterval(astFile, pos-1, pos, false, func(node xgoast.Node) bool {
		callExpr, ok := node.(*xgoast.CallExpr)
		if !ok || len(callExpr.Args) == 0 {
			return true
		}
		funcIdent, ok := callExpr.Fun.(*xgoast.Ident)
		if !ok {
			return true
		}
		funcObj := typeInfo.ObjectOf(funcIdent)
		if !IsInClassfilePkg(funcObj, r.classfileConfig) {
			return true
		}
		isIn = IsSpxEventHandlerFuncName(funcIdent.Name)
		return !isIn // Stop walking if we found a match.
	})
	return isIn
}

// spxImportsAtASTFilePosition returns the import at the given position in the given AST file.
func (r *compileResult) spxImportsAtASTFilePosition(astFile *xgoast.File, position xgotoken.Position) *SpxReferencePkg {
	fset := r.proj.Fset
	for _, imp := range astFile.Imports {
		nodePos := fset.Position(imp.Pos())
		nodeEnd := fset.Position(imp.End())
		if nodePos.Filename != position.Filename ||
			position.Line != nodePos.Line ||
			position.Column < nodePos.Column ||
			position.Column > nodeEnd.Column {
			continue
		}

		pkg, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		pkgDoc, err := pkgdata.GetPkgDoc(pkg)
		if err != nil {
			continue
		}
		return &SpxReferencePkg{
			Pkg:     pkgDoc,
			PkgPath: pkg,
			Node:    imp,
		}
	}
	return nil
}

// addDiagnostics adds diagnostics to the compile result.
func (r *compileResult) addDiagnostics(documentURI DocumentURI, diags ...Diagnostic) {
	if r.seenDiagnostics == nil {
		r.seenDiagnostics = make(map[DocumentURI]map[string]struct{})
	}
	seenDiagnostics := r.seenDiagnostics[documentURI]
	if seenDiagnostics == nil {
		seenDiagnostics = make(map[string]struct{})
		r.seenDiagnostics[documentURI] = seenDiagnostics
	}

	r.diagnostics[documentURI] = slices.Grow(r.diagnostics[documentURI], len(diags))
	for _, diag := range diags {
		fingerprint := fmt.Sprintf("%d\n%v\n%s", diag.Severity, diag.Range, diag.Message)
		if _, ok := seenDiagnostics[fingerprint]; ok {
			continue
		}
		seenDiagnostics[fingerprint] = struct{}{}

		r.diagnostics[documentURI] = append(r.diagnostics[documentURI], diag)
		if diag.Severity == SeverityError {
			r.hasErrorSeverityDiagnostic = true
		}
	}
}

// compile compiles spx source files and returns compile result. It uses cached
// result if available.
func (s *Server) compile() (*compileResult, error) {
	// NOTE(xsw): don't create a snapshot
	snapshot := s.workspaceRootFS // .Snapshot()

	// TODO(wyvern): remove this once we have a better way to update files.
	// IMPORTANT: Don't call UpdateFiles here, as it will overwrite the in-memory
	// changes made by textDocument/didChange with stale content from fileMapGetter.
	// The didChange handler already updates files via ModifyFiles.
	// snapshot.UpdateFiles(s.fileMapGetter())

	return s.compileAt(snapshot)
}

// compileAt compiles classfile source files at the given snapshot and returns the
// compile result.
func (s *Server) compileAt(snapshot *xgo.Project) (*compileResult, error) {
	var spxFiles []string
	// Get the project extension from classfile config
	projectExt := ""
	if s.classfileConfig != nil {
		projectExt = s.classfileConfig.ProjectExt
	}

	for file := range snapshot.Files() {
		if projectExt != "" && path.Ext(file) == projectExt {
			spxFiles = append(spxFiles, file)
		}
	}
	if len(spxFiles) == 0 {
		return nil, errNoMainSpxFile
	}

	result := newCompileResult(snapshot, s.classfileConfig)
	for _, spxFile := range spxFiles {
		documentURI := s.toDocumentURI(spxFile)
		result.diagnostics[documentURI] = []Diagnostic{}

		astFile, err := snapshot.ASTFile(spxFile)
		if err != nil {
			var (
				errorList xgoscanner.ErrorList
				codeError *gogen.CodeError
			)
			if errors.As(err, &errorList) && astFile.Pos().IsValid() {
				// Handle parse errors.
				for _, e := range errorList {
					result.addDiagnostics(documentURI, Diagnostic{
						Severity: SeverityError,
						Range:    RangeForASTFilePosition(result.proj, astFile, e.Pos),
						Message:  s.translate(e.Msg),
					})
				}
			} else if errors.As(err, &codeError) {
				// Handle code generation errors.
				result.addDiagnostics(documentURI, Diagnostic{
					Severity: SeverityError,
					Range:    RangeForPosEnd(result.proj, codeError.Pos, codeError.End),
					Message:  codeError.Error(),
				})
			} else {
				// Handle unknown errors (including recovered panics).
				result.addDiagnostics(documentURI, Diagnostic{
					Severity: SeverityError,
					Message:  s.translate(fmt.Sprintf("failed to parse spx file: %v", err)),
				})
			}
		}
		if astFile == nil {
			continue
		}
		if astFile.Name.Name != "main" && astFile.Pos().IsValid() {
			result.addDiagnostics(documentURI, Diagnostic{
				Severity: SeverityError,
				Range:    RangeForASTFileNode(result.proj, astFile, astFile.Name),
				Message:  s.translate("package name must be main"),
			})
			continue
		}

		// Check if this is the main classfile (e.g., main.spx, main.gmx, main.gox)
		spxFileBaseName := path.Base(spxFile)
		mainFileName := "main" + result.classfileConfig.ProjectExt
		if spxFileBaseName == mainFileName {
			result.mainSpxFile = spxFile
		}
	}
	if result.mainSpxFile == "" {
		if len(result.diagnostics) == 0 {
			return nil, errNoMainSpxFile
		}
		return result, nil
	}

	handleErr := func(err error) {
		if typeErr, ok := err.(typesutil.Error); ok {
			if !typeErr.Pos.IsValid() {
				panic(fmt.Sprintf("unexpected nopos error: %s", typeErr.Msg))
			}
			position := typeErr.Fset.Position(typeErr.Pos)
			documentURI := s.toDocumentURI(position.Filename)
			result.addDiagnostics(documentURI, Diagnostic{
				Severity: SeverityError,
				Range:    RangeForPosEnd(result.proj, typeErr.Pos, typeErr.End),
				Message:  typeErr.Msg,
			})
		}
	}

	if _, err := snapshot.TypeInfo(); err != nil {
		switch err := err.(type) {
		case errors.List:
			for _, e := range err {
				handleErr(e)
			}
		default:
			handleErr(err)
		}
	}

	s.inspectDiagnosticsAnalyzers(result)

	return result, nil
}

// compileAndGetASTFileForDocumentURI handles common compilation and file
// retrieval logic for a given document URI. The returned astFile is probably
// nil even if the compilation succeeded.
func (s *Server) compileAndGetASTFileForDocumentURI(uri DocumentURI) (result *compileResult, spxFile string, astFile *xgoast.File, err error) {
	spxFile, err = s.fromDocumentURI(uri)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to get file path from document URI %q: %w", uri, err)
	}
	fmt.Printf("[DEBUG] fromDocumentURI(%q) = %q\n", uri, spxFile)

	// Check if file has the correct classfile extension
	projectExt := ""
	if s.classfileConfig != nil {
		projectExt = s.classfileConfig.ProjectExt
	}
	if projectExt != "" && path.Ext(spxFile) != projectExt {
		return nil, "", nil, fmt.Errorf("file %q does not have %s extension", spxFile, projectExt)
	}
	result, err = s.compile()
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to compile: %w", err)
	}
	if astPkg, _ := result.proj.ASTPackage(); astPkg != nil {
		fmt.Printf("[DEBUG] Looking for astFile with key: %q\n", spxFile)
		fmt.Printf("[DEBUG] Available keys in astPkg.Files: %v\n", func() []string {
			keys := make([]string, 0, len(astPkg.Files))
			for k := range astPkg.Files {
				keys = append(keys, k)
			}
			return keys
		}())
		astFile = astPkg.Files[spxFile]
		if astFile == nil {
			fmt.Printf("[DEBUG] astFile is nil! Key not found in astPkg.Files\n")
		}
	} else {
		fmt.Printf("[DEBUG] astPkg is nil\n")
	}
	return
}

// inspectDiagnosticsAnalyzers runs registered analyzers on each spx source file
// and collects diagnostics.
//
// For each spx file in the main package, it:
//  1. Creates an analysis pass with file-specific information
//  2. Runs all registered analyzers on the file
//  3. Collects diagnostics from analyzers
//  4. Reports any analyzer errors as diagnostics
//
// Parameters:
//   - result: The compilation result containing AST and type information
//
// The function updates result.diagnostics with any issues found by analyzers.
// Diagnostic severity levels include:
//   - Error: For analyzer failures or serious code issues
//   - Warning: For potential problems that don't prevent compilation
func (s *Server) inspectDiagnosticsAnalyzers(result *compileResult) {
	proj := result.proj
	fset := proj.Fset
	typeInfo, _ := proj.TypeInfo()
	if typeInfo == nil {
		return
	}
	astPkg, _ := proj.ASTPackage()
	if astPkg == nil {
		return
	}
	for spxFile, astFile := range astPkg.Files {
		var diagnostics []Diagnostic
		pass := &protocol.Pass{
			Fset:      fset,
			Files:     []*xgoast.File{astFile},
			TypesInfo: typeInfo,
			Report: func(d protocol.Diagnostic) {
				diagnostics = append(diagnostics, Diagnostic{
					Range:    RangeForPosEnd(proj, d.Pos, d.End),
					Severity: SeverityError,
					Message:  s.translate(d.Message),
				})
			},
			ResultOf: map[*protocol.Analyzer]any{
				inspect.Analyzer: inspector.New([]*xgoast.File{astFile}),
			},
		}

		for _, analyzer := range s.analyzers {
			an := analyzer.Analyzer()
			if _, err := an.Run(pass); err != nil {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityError,
					Message:  s.translate(fmt.Sprintf("analyzer %q failed: %v", an.Name, err)),
				})
			}
		}

		if len(diagnostics) > 0 {
			documentURI := s.toDocumentURI(spxFile)
			result.addDiagnostics(documentURI, diagnostics...)
		}
	}
}
