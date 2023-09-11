package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

var (
	backendPattern = flag.String("f", "", "backend Go file glob")
	outputPath     = flag.String("o", "", "output directory")
)

// xCryptoBackendMapPrefix is the prefix for command comments. It would be nice
// to omit the " ", but the Go formatter adds it back in. (Sometimes? It does
// in VS Code. It doesn't seem like Go formatters should, though.)
const xCryptoBackendMapPrefix = "// xcrypto_backend_map:"

func main() {
	h := flag.Bool("h", false, "show help")
	flag.Parse()
	if *h {
		flag.Usage()
		return
	}
	if *backendPattern == "" {
		log.Fatalln("f is required")
	}
	if err := run(); err != nil {
		log.Fatalln(err)
	}
}

func run() error {
	fset := token.NewFileSet()
	matches, err := filepath.Glob(*backendPattern)
	if err != nil {
		return err
	}
	for _, filename := range matches {
		f, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		g := newFileGen(fset, f)
		if !g.backend() {
			continue
		}

		// Write to destination.
		if *outputPath != "" {
			if err := os.MkdirAll(*outputPath, 0o777); err != nil {
				return err
			}
			f, err := os.Create(filepath.Join(*outputPath, filepath.Base(filename)))
			if err != nil {
				return err
			}
			err = g.write(f)
			if closeErr := f.Close(); err == nil {
				err = closeErr
			}
			if err != nil {
				return err
			}
		} else {
			if err := g.write(os.Stdout); err != nil {
				return err
			}
		}
	}
	return nil
}

// fileGen filters and modifies an internal backend file to make a proxy file
// that can access it from another module.
type fileGen struct {
	// Context shared by filtering passes.
	fset *token.FileSet
	f    *ast.File

	okImports map[string]*ast.ImportSpec
}

func newFileGen(fset *token.FileSet, f *ast.File) *fileGen {
	return &fileGen{
		fset: fset,
		f:    f,
	}
}

func (g *fileGen) backend() bool {
	// Super simple heuristic that works for "crypto/internal/backend": does
	// the file define "Enabled"?
	enabledDecl := g.f.Scope.Lookup("Enabled")
	return enabledDecl != nil
}

func (g *fileGen) enabled() (bool, error) {
	enabledDecl := g.f.Scope.Lookup("Enabled")
	if enabledDecl == nil {
		return false, fmt.Errorf("no Enabled declaration")
	}
	vs, ok := enabledDecl.Decl.(*ast.ValueSpec)
	if !ok {
		return false, fmt.Errorf("expected ValueSpec for Enabled")
	}
	if len(vs.Values) != 1 {
		return false, fmt.Errorf("expected single value for Enabled")
	}
	b, ok := vs.Values[0].(*ast.Ident)
	if !ok {
		return false, fmt.Errorf("expected boolean value for Enabled")
	}
	// Is this an enabled backend, or the disabled nobackend "backend"?
	return b.Name == "true", nil
}

func (g *fileGen) write(w io.Writer) error {
	_, err := g.enabled()
	if err != nil {
		return err
	}

	var parseErrs []string
	var skipMessages []string

	// Scan to find existing imports that are ok to keep.
	okImports := make(map[string]*ast.ImportSpec)
	_ = astutil.Apply(g.f, func(c *astutil.Cursor) bool {
		switch n := (c.Node()).(type) {
		case *ast.ImportSpec:
			// Simple heuristic checks.
			if strings.Contains(n.Path.Value, "internal") {
				return false
			}
			if strings.Contains(n.Path.Value, ".") {
				return false
			}
			unquotedPath, err := strconv.Unquote(n.Path.Value)
			if err != nil {
				panic(err)
			}
			name := path.Base(unquotedPath)
			if n.Name != nil {
				name = n.Name.Name
			}
			okImports[name] = n
		}
		return true
	}, nil)

	// Mutate the AST in place.
	_ = astutil.Apply(g.f, func(c *astutil.Cursor) bool {
		switch n := (c.Node()).(type) {

		case *ast.GenDecl:
			switch n.Tok {
			case token.CONST, token.VAR:
				// If there are multiple specs in a single "var ()" block, or
				// multiple names in a single spec, split them up. This way, we
				// can easily assign a comment to each one.
				for _, spec := range n.Specs {
					vs := spec.(*ast.ValueSpec)
					for _, name := range vs.Names {
						if !name.IsExported() {
							continue
						}
						var gd ast.GenDecl
						if name.Name == "Enabled" {
							// Special case: constant Enabled. Keeping this
							// const can be important for performance, so copy
							// it rather than using linkname.
							gd.Tok = token.CONST
							gd.Specs = n.Specs
						} else {
							// General case: linkname.
							// Make sure we have a type for the generated var.
							if vs.Type == nil {
								cmds := commands(n)
								for _, cmd := range cmds {
									if t, ok := strings.CutPrefix(cmd, "type="); ok {
										vs.Type = &ast.Ident{Name: t}
										break
									}
								}
								if name.Name == "RandReader" {
									vs.Type = &ast.SelectorExpr{
										X:   &ast.Ident{Name: "io"},
										Sel: &ast.Ident{Name: "Reader"},
									}
								}
								if vs.Type == nil {
									parseErrs = append(parseErrs, fmt.Sprintf(
										"%v: could not determine type for %s; define it, or use %q",
										g.fset.Position(n.Pos()),
										name.Name,
										xCryptoBackendMapPrefix+"type=<...>"))
									continue
								}
							}
							// Always use var: linkname only works on a
							// variable. It's fine to link to a constant.
							gd.Tok = token.VAR
							// Remove value from the spec. Linkname will
							// provide. We can't reference internal and
							// vendored APIs like "openssl.RandReader".
							gd.Specs = []ast.Spec{
								&ast.ValueSpec{
									Names: []*ast.Ident{name},
									Type:  vs.Type,
								},
							}
							gd.Doc = &ast.CommentGroup{
								List: []*ast.Comment{{
									Text: "//go:linkname " + name.Name + " crypto/internal/backend." + name.Name,
								}},
							}
						}
						c.InsertAfter(&gd)
					}
				}
				c.Delete()
				return false

			case token.TYPE:
				// Don't include any type definitions. These normally look
				// like "type RSA = openssl.RSA", which we can't link to.
				c.Delete()
				return false

			case token.IMPORT:
				// Don't modify imports here. We'll do that in the next pass.
				return false
			}

		case *ast.FuncDecl:
			if !n.Name.IsExported() {
				c.Delete()
				return false
			}
			var cg ast.CommentGroup
			// Remove funcs that depend on internal and vendored types and
			// include a comment explaining why.
			if err := ensureSimpleType(n.Type, okImports); err != nil {
				skipMessages = append(skipMessages, fmt.Sprintf("Skipped %q: %v", n.Name.Name, err))
				c.Delete()
				return false
			}
			for _, cmd := range commands(n) {
				switch cmd {
				case "noescape":
					cg.List = append(cg.List, &ast.Comment{
						Text: "//go:noescape",
					})
				default:
					parseErrs = append(parseErrs, fmt.Sprintf(
						"%v: unrecognized xcrypto_backend_map command: %s",
						g.fset.Position(n.Pos()),
						cmd))
				}
			}
			cg.List = append(cg.List, &ast.Comment{
				Text: "//go:linkname " + n.Name.Name + " crypto/internal/backend." + n.Name.Name,
			})
			n.Doc = &cg
			n.Body = nil
			return false
		}
		return true // Walk deeper into this node of the AST.
	}, nil)

	// Remove unused imports. At this point, we've removed all usage of
	// internal and vendored APIs, so this should clean those up without
	// having to do any more guesswork.
	var cleanedImports []ast.Spec
	_ = astutil.Apply(g.f, func(c *astutil.Cursor) bool {
		switch n := (c.Node()).(type) {
		case *ast.ImportSpec:
			p, err := strconv.Unquote(n.Path.Value)
			if err != nil {
				panic(err)
			}
			if astutil.UsesImport(g.f, p) {
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
			return false
		}
		return true
	})

	if len(parseErrs) > 0 {
		return fmt.Errorf("failed to parse backend file:\n  %v", strings.Join(parseErrs, "\n  "))
	}

	io.WriteString(w, "// Code generated by xcrypto_backend_map. DO NOT EDIT.\n\n")
	if len(skipMessages) > 0 {
		io.WriteString(w, "// Some backend functionality was skipped during mapping generation:\n//\n")
		for _, msg := range skipMessages {
			fmt.Fprintf(w, "// %s\n", msg)
		}
		io.WriteString(w, "\n")
	}
	// Preserve the constraint.
	for _, cg := range g.f.Comments {
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

	if err := format.Node(w, g.fset, g.f); err != nil {
		return err
	}
	return nil
}

func ensureSimpleType(t ast.Expr, imports map[string]*ast.ImportSpec) error {
	switch t := t.(type) {
	case *ast.Ident:
		// Just an identity: this is a simple type like "byte".
	case *ast.SelectorExpr:
		// A selector is from an imported package. Check if the package is ok.
		x := t.X.(*ast.Ident)
		if _, ok := imports[x.Name]; !ok {
			return fmt.Errorf("%v.%v uses unimported package", x.Name, t.Sel.Name)
		}
	case *ast.ArrayType:
		return ensureSimpleType(t.Elt, imports)
	case *ast.FuncType:
		if t.TypeParams != nil {
			return fmt.Errorf("generic function")
		}
		for _, field := range append(t.Params.List, t.Results.List...) {
			if err := ensureSimpleType(field.Type, imports); err != nil {
				return err
			}
		}
	case *ast.StarExpr:
		return ensureSimpleType(t.X, imports)
	default:
		return fmt.Errorf("unrecognized node: %T", t)
	}
	return nil
}

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
