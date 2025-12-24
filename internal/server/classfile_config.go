package server

import (
	"path"

	"github.com/goplus/mod/modfile"
	"github.com/goplus/mod/xgomod"
)

// ClassfileConfig holds the configuration for a classfile project.
type ClassfileConfig struct {
	// ProjectClass is the main class name (e.g., "Game" for SPX)
	ProjectClass string
	// ProjectExt is the file extension for project files (e.g., ".spx")
	ProjectExt string
	// WorkClasses maps file extensions to class names (e.g., ".spx" -> "SpriteImpl")
	WorkClasses map[string]string
	// PkgPaths are the package paths that define the classes (e.g., "github.com/goplus/spx/v2")
	PkgPaths []string
}

// LoadClassfileConfig loads classfile configuration from xgomod.Module.
// If mod is nil and manualConfig is provided, uses the manual configuration instead.
func LoadClassfileConfig(mod *xgomod.Module, manualConfig *ClassfileInitOptions) *ClassfileConfig {
	// If no gop.mod but manual configuration is provided, use manual config
	if mod == nil && manualConfig != nil {
		return &ClassfileConfig{
			ProjectClass: manualConfig.ProjectClass,
			ProjectExt:   manualConfig.ProjectExt,
			WorkClasses:  manualConfig.WorkClasses,
			PkgPaths:     manualConfig.PkgPaths,
		}
	}

	if mod == nil {
		return nil
	}

	// Find the first project configuration
	var proj *modfile.Project
	for _, p := range mod.Projects() {
		proj = p
		break
	}

	if proj == nil {
		return nil
	}

	config := &ClassfileConfig{
		ProjectClass: proj.Class,
		ProjectExt:   proj.Ext,
		WorkClasses:  make(map[string]string),
		PkgPaths:     proj.PkgPaths,
	}

	// Map work classes
	for _, work := range proj.Works {
		config.WorkClasses[work.Ext] = work.Class
	}

	return config
}

// GetClassForFile returns the class name for a given file path.
// Returns ProjectClass if the file extension matches ProjectExt and it's not a work class,
// or the corresponding WorkClass if it matches a work extension.
func (c *ClassfileConfig) GetClassForFile(filePath string) string {
	if c == nil {
		return ""
	}

	ext := path.Ext(filePath)

	// First check if this is a work file
	if workClass, ok := c.WorkClasses[ext]; ok {
		return workClass
	}

	// If extension matches project extension, it's a project class file
	if ext == c.ProjectExt {
		return c.ProjectClass
	}

	return ""
}

// GetDisplayClassForFile returns the display name for a class.
// For embedded classes that end with "Impl", strips the "Impl" suffix.
func (c *ClassfileConfig) GetDisplayClassForFile(filePath string) string {
	className := c.GetClassForFile(filePath)

	// Handle embedded classes that end with "Impl"
	// This is a common pattern in XGo classfiles
	if len(className) > 4 && className[len(className)-4:] == "Impl" {
		// Strip "Impl" suffix for display
		return className[:len(className)-4]
	}

	return className
}

// IsClassfilePkg returns true if the package path is one of the classfile packages.
func (c *ClassfileConfig) IsClassfilePkg(pkgPath string) bool {
	if c == nil {
		return false
	}

	for _, p := range c.PkgPaths {
		if p == pkgPath {
			return true
		}
	}
	return false
}

// NormalizeClassName converts display class name to actual class name if needed.
// For example, "Sprite" might become "SpriteImpl" if SpriteImpl is a work class.
func (c *ClassfileConfig) NormalizeClassName(displayName string) string {
	if c == nil {
		return displayName
	}

	// Check if displayName + "Impl" is a work class
	implName := displayName + "Impl"
	for _, workClass := range c.WorkClasses {
		if workClass == implName {
			return implName
		}
	}

	return displayName
}

// GetDisplayName converts actual class name to display name.
// For example, "SpriteImpl" becomes "Sprite".
func (c *ClassfileConfig) GetDisplayName(className string) string {
	if c == nil {
		return className
	}

	// Handle embedded classes that end with "Impl"
	if len(className) > 4 && className[len(className)-4:] == "Impl" {
		// Check if this is actually a work class
		for _, workClass := range c.WorkClasses {
			if workClass == className {
				return className[:len(className)-4]
			}
		}
	}

	return className
}

