package server

import (
	"go/types"
	"regexp"

	xgoast "github.com/goplus/xgo/ast"
	"github.com/goplus/xgolsw/xgo"
	xgotypes "github.com/goplus/xgolsw/xgo/types"
	"github.com/goplus/xgolsw/xgo/xgoutil"
)

// spxEventHandlerFuncNameRE is the regular expression of the spx event handler
// function name.
var spxEventHandlerFuncNameRE = regexp.MustCompile(`^on[A-Z]\w*$`)

// IsSpxEventHandlerFuncName reports whether the given function name is an
// spx event handler function name.
func IsSpxEventHandlerFuncName(name string) bool {
	return spxEventHandlerFuncNameRE.MatchString(name)
}

// IsInClassfilePkg reports whether the given object is defined in one of the classfile packages.
func IsInClassfilePkg(obj types.Object, config *ClassfileConfig) bool {
	if obj == nil || obj.Pkg() == nil || config == nil {
		return false
	}
	return config.IsClassfilePkg(xgoutil.PkgPath(obj.Pkg()))
}

// GetSimplifiedTypeString returns the string representation of the given type,
// with the classfile package names omitted while other packages use their short names.
func GetSimplifiedTypeString(typ types.Type, config *ClassfileConfig) string {
	return types.TypeString(typ, func(p *types.Package) string {
		if config != nil && config.IsClassfilePkg(xgoutil.PkgPath(p)) {
			return ""
		}
		return p.Name()
	})
}

// SelectorTypeNameForIdent returns the selector type name for the given
// identifier. It returns empty string if no selector can be inferred.
func SelectorTypeNameForIdent(proj *xgo.Project, ident *xgoast.Ident, config *ClassfileConfig) string {
	typeInfo, _ := proj.TypeInfo()
	if typeInfo == nil {
		return ""
	}
	astPkg, _ := proj.ASTPackage()
	astFile := xgoutil.NodeASTFile(proj.Fset, astPkg, ident)
	if astFile == nil {
		return ""
	}

	obj := typeInfo.ObjectOf(ident)
	if obj == nil || obj.Pkg() == nil {
		return ""
	}

	// Handle classfile package's implicit receiver semantics.
	if typeName := tryGetClassfileImplicitReceiver(proj, astFile, ident, obj, config); typeName != "" {
		return typeName
	}

	// Infer type from object properties.
	return getTypeFromObject(typeInfo, obj, config)
}

// tryGetClassfileImplicitReceiver handles classfile package's special implicit receiver semantics.
func tryGetClassfileImplicitReceiver(proj *xgo.Project, astFile *xgoast.File, ident *xgoast.Ident, obj types.Object, config *ClassfileConfig) string {
	if !IsInClassfilePkg(obj, config) || config == nil {
		return ""
	}
	typeInfo, _ := proj.TypeInfo()
	if typeInfo == nil {
		return ""
	}
	astPkg, _ := proj.ASTPackage()

	astFileScope := typeInfo.Scopes[astFile]
	innermostScope := xgoutil.InnermostScopeAt(proj.Fset, typeInfo, astPkg, ident.Pos())

	// Check if we're in the right scope context.
	if innermostScope != astFileScope && (!astFile.HasShadowEntry() || xgoutil.InnermostScopeAt(proj.Fset, typeInfo, astPkg, astFile.ShadowEntry.Pos()) != innermostScope) {
		return ""
	}

	// Determine the class based on the file
	filePath := xgoutil.NodeFilename(proj.Fset, ident)
	return config.GetDisplayClassForFile(filePath)
}

// getTypeFromObject infers type from the identifier's object.
func getTypeFromObject(typeInfo *xgotypes.Info, obj types.Object, config *ClassfileConfig) string {
	switch obj := obj.(type) {
	case *types.Var:
		if !obj.IsField() {
			return ""
		}
		return findFieldOwnerType(typeInfo, obj, config)
	case *types.Func:
		sig, ok := obj.Type().(*types.Signature)
		if !ok {
			return ""
		}
		recv := sig.Recv()
		if recv == nil {
			return ""
		}
		return extractTypeName(xgoutil.DerefType(recv.Type()), config)
	}
	return ""
}

// extractTypeName extracts a clean type name from a types.Type.
func extractTypeName(typ types.Type, config *ClassfileConfig) string {
	switch typ := typ.(type) {
	case *types.Named:
		obj := typ.Obj()
		typeName := obj.Name()
		// Handle embedded classes that end with "Impl"
		if IsInClassfilePkg(obj, config) && len(typeName) > 4 && typeName[len(typeName)-4:] == "Impl" {
			return typeName[:len(typeName)-4]
		}
		return typeName
	case *types.Interface:
		if typ.String() == "interface{}" {
			return ""
		}
		return typ.String()
	}
	return ""
}

// findFieldOwnerType finds the type that owns a given field.
func findFieldOwnerType(typeInfo *xgotypes.Info, field *types.Var, config *ClassfileConfig) string {
	if !field.IsField() {
		return ""
	}

	fieldPkg := field.Pkg()
	if fieldPkg == nil {
		return ""
	}

	// Search through named types in the same package.
	scope := fieldPkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		typeName, ok := obj.(*types.TypeName)
		if !ok {
			continue
		}

		named, ok := xgoutil.DerefType(typeName.Type()).(*types.Named)
		if !ok || !xgoutil.IsNamedStructType(named) {
			continue
		}

		// Check if this struct contains our field.
		if ownerName := checkStructForField(named, field, fieldPkg, config); ownerName != "" {
			return ownerName
		}
	}

	// Fallback: search through all type definitions.
	return searchAllDefsForField(typeInfo, field, config)
}

// checkStructForField checks if a struct type contains the given field.
func checkStructForField(named *types.Named, field *types.Var, fieldPkg *types.Package, config *ClassfileConfig) string {
	foundObj, indices, _ := types.LookupFieldOrMethod(named, false, fieldPkg, field.Name())
	if foundObj == nil || len(indices) == 0 {
		return ""
	}

	foundField, ok := foundObj.(*types.Var)
	if !ok || foundField != field {
		return ""
	}

	typeName := named.Obj().Name()
	// Handle embedded classes that end with "Impl"
	if IsInClassfilePkg(named.Obj(), config) && len(typeName) > 4 && typeName[len(typeName)-4:] == "Impl" {
		return typeName[:len(typeName)-4]
	}
	return typeName
}

// searchAllDefsForField is a fallback method that searches all type definitions.
func searchAllDefsForField(typeInfo *xgotypes.Info, field *types.Var, config *ClassfileConfig) string {
	fieldPkg := field.Pkg()
	for _, def := range typeInfo.Defs {
		if def == nil || def.Pkg() != fieldPkg {
			continue
		}

		named, ok := xgoutil.DerefType(def.Type()).(*types.Named)
		if !ok || !xgoutil.IsNamedStructType(named) {
			continue
		}

		if ownerName := checkStructForField(named, field, fieldPkg, config); ownerName != "" {
			return ownerName
		}
	}
	return ""
}
