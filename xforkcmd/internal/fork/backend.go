package fork

import (
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

// xCryptoBackendMapPrefix is the prefix for command comments. It would be nice
// to omit the " ", but the Go formatter adds it back in. (Sometimes? It does
// in VS Code. It doesn't seem like Go formatters should, though.)
const xCryptoBackendMapPrefix = "// xcrypto_backend_map:"

func commands(n ast.Node) []string {
	var cmds []string
	ast.Inspect(n, func(n ast.Node) bool {
		if n, ok := n.(*ast.Comment); !ok {
			return true
		} else if cmd, ok := strings.CutPrefix(n.Text, xCryptoBackendMapPrefix); !ok {
			return true
		} else {
			cmds = append(cmds, cmd)
		}
		return false
	})
	return cmds
}

// FindBackendFiles returns the Go files that appear to be backends in the
// given directory. Returns the parsed trees rather than only the filenames: we
// parsed the file to determine if it's a backend, and the parsed data is
// useful later.
func FindBackendFiles(dir string) ([]*BackendFile, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	var backends []*BackendFile
	for _, match := range matches {
		filename, err := filepath.Abs(match)
		if err != nil {
			return nil, err
		}
		b, err := NewBackendFile(filename)
		if err != nil {
			if errors.Is(err, errNotBackend) {
				continue
			}
			return nil, err
		}
		backends = append(backends, b)
	}
	return backends, nil
}

var errNotBackend = errors.New("not a crypto backend file")

type BackendFile struct {
	// Filename is the absolute path to the original file.
	Filename string

	f    *ast.File
	fset *token.FileSet
}

func NewBackendFile(filename string) (*BackendFile, error) {
	b := &BackendFile{
		Filename: filename,
		fset:     token.NewFileSet(),
	}
	f, err := parser.ParseFile(b.fset, filename, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	b.f = f
	// Super simple heuristic that works for "crypto/internal/backend": does
	// the file define "Enabled"?
	enabledDecl := f.Scope.Lookup("Enabled")
	if enabledDecl == nil {
		return nil, errNotBackend
	}
	return b, nil
}

// PlaceholderTrim changes b to include a placeholder API, following
// conventions that assume b is a "nobackend" crypto backend. The placeholder
// API is buildable, but panics if used.
func (b *BackendFile) PlaceholderTrim() error {
	var err error
	localPackageType := make(map[string]*ast.TypeSpec)
	_ = astutil.Apply(b.f, func(c *astutil.Cursor) bool {
		switch n := (c.Node()).(type) {
		// Only look into top-level declarations, nothing else.
		case *ast.File, *ast.GenDecl:
			return true

		case *ast.TypeSpec:
			// Remove type names declared in this package and keep track of
			// them to remove any functions that use them in another pass.
			localPackageType[n.Name.Name] = n
			c.Delete()

		case *ast.ValueSpec:
			// Remove all var/const declarations other than Enabled.
			declaresEnabled := false
			for _, name := range n.Names {
				if name.Name == "Enabled" {
					declaresEnabled = true
				}
			}
			if !declaresEnabled {
				c.Delete()
			} else if len(n.Names) != 1 {
				err = fmt.Errorf(
					"declaration for Enabled %v includes multiple names",
					b.fset.Position(n.Pos()))
			}
			// We could detect "const RandReader = ..." and change it to
			// "var RandReader io.Reader". go:linkname supports mapping a var
			// to a const in this way. However, this is already accessible via
			// "crypto/rand" and there is no need to provide direct access.
			// So, simply leave it out.
		}
		return false
	}, nil)
	if err != nil {
		return err
	}
	_ = astutil.Apply(b.f, func(c *astutil.Cursor) bool {
		switch n := (c.Node()).(type) {
		case *ast.File:
			return true
		case *ast.FuncDecl:
			// Remove unexported functions and all methods.
			if !n.Name.IsExported() || n.Recv != nil {
				c.Delete()
				return false
			}
			var remove bool
			ast.Inspect(n.Type, func(tn ast.Node) bool {
				switch tn := tn.(type) {
				case *ast.Ident:
					if _, ok := localPackageType[tn.Name]; ok {
						remove = true
						return false
					}
				}
				return true
			})
			if remove {
				c.Delete()
			}
		}
		return false
	}, nil)
	return cleanImports(b.f)
}

// ProxyAPI creates a proxy for b implementing each var/func in the given api.
// If b is missing some part of api, this method will succeed, but the returned
// proxy object will keep track of the gaps in its data.
//
// If a func in b uses the "noescape" command, the proxy includes
// "//go:noescape" on that func.
func (b *BackendFile) ProxyAPI(api *BackendFile) (*BackendProxy, error) {
	p := &BackendProxy{
		backend: b,
		api:     api,
		f:       &ast.File{Name: b.f.Name},
		fset:    token.NewFileSet(),
	}

	// Copy the imports that are used to define the API.
	// Ignore the imports used by b: those will include internal packages and
	// backend-specific packages that we don't have access to.
	ast.Inspect(api.f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.File:
			return true
		case *ast.GenDecl:
			if n.Tok == token.IMPORT {
				return true
			}
		case *ast.ImportSpec:
			astutil.AddNamedImport(p.fset, p.f, n.Name.Name, n.Path.Value)
		}
		return false
	})

	// Add unsafe import needed for go:linkname.
	astutil.AddNamedImport(p.fset, p.f, "_", "unsafe")

	// For each API, find it in b. If exists, generate linkname "proxy" func.
	var err error
	ast.Inspect(api.f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.File:
			return true
		case *ast.FuncDecl:
			// Find the corresponding func in b.
			o := b.f.Scope.Lookup(n.Name.Name)
			if o == nil {
				p.missing = append(p.missing, n)
				return false
			}
			fn, ok := o.Decl.(*ast.FuncDecl)
			if !ok {
				p.missing = append(p.missing, n)
				return false
			}
			comments := []*ast.Comment{
				{Text: "//go:linkname " + n.Name.Name + " crypto/internal/backend." + n.Name.Name},
			}
			for _, cmd := range commands(fn) {
				switch cmd {
				case "noescape":
					comments = append(comments, &ast.Comment{Text: "//go:noescape"})
				default:
					err = fmt.Errorf("unknown command %q (%v)", cmd, b.fset.Position(n.Pos()))
					return false
				}
			}
			proxyFn := &ast.FuncDecl{
				Name: n.Name,
				Type: fn.Type,
				Doc:  &ast.CommentGroup{List: comments},
			}
			p.f.Decls = append(p.f.Decls, proxyFn)
		}
		return false
	})
	if err != nil {
		return nil, err
	}

	if err := cleanImports(p.f); err != nil {
		return nil, err
	}
	return p, nil
}

func (b *BackendFile) Write(w io.Writer) error {
	io.WriteString(w, "// Generated code. DO NOT EDIT.\n\n")
	return write(b.f, b.fset, w)
}

type BackendProxy struct {
	backend *BackendFile
	api     *BackendFile

	f    *ast.File
	fset *token.FileSet

	missing []*ast.FuncDecl
}

func (p *BackendProxy) Write(w io.Writer) error {
	io.WriteString(w, "// Generated code. DO NOT EDIT.\n\n")
	io.WriteString(w, "// This file implements a proxy that links into a specific crypto backend.\n\n")
	return write(p.f, p.fset, w)
}

func write(f *ast.File, fset *token.FileSet, w io.Writer) error {
	// Preserve the build constraint. It's in a comment that isn't reachable
	// from the node tree.
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.HasPrefix(c.Text, "//go:build ") {
				io.WriteString(w, c.Text)
				io.WriteString(w, "\n\n")
				break
			}
		}
	}
	// Force the printer to use the comments associated with the nodes by
	// clearing the cache-like (but not just a cache) Comments slice.
	g.f.Comments = nil
	return format.Node(w, fset, f)
}

func cleanImports(f *ast.File) error {
	var err error
	var cleanedImports []ast.Spec
	_ = astutil.Apply(f, func(c *astutil.Cursor) bool {
		switch n := (c.Node()).(type) {
		case *ast.GenDecl:
			// Support multiple import declarations. Import blocks can't be
			// nested, so simply reset the slice.
			if n.Tok == token.IMPORT {
				cleanedImports = cleanedImports[:0]
			}
		case *ast.ImportSpec:
			var p string
			if p, err = strconv.Unquote(n.Path.Value); err != nil {
				return false
			}
			if n.Name != nil && n.Name.Name == "_" || astutil.UsesImport(f, p) {
				// Reset the position to remove unnecessary newlines when
				// imports are omitted.
				n.Path.ValuePos = 0
				cleanedImports = append(cleanedImports, n)
			}
			return false
		}
		return true
	}, func(c *astutil.Cursor) bool {
		switch n := (c.Node()).(type) {
		case *ast.GenDecl:
			if n.Tok == token.IMPORT {
				n.Specs = cleanedImports
			}
		}
		return true
	})
	return nil
}