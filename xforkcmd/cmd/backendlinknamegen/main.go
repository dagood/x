package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
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
		if en := f.Scope.Lookup("Enabled"); en == nil {
			continue
		}
		var parseErrs []string
		var skipMessages []string
		okImports := make(map[string]struct{})
		// Mutate the AST in place.
		_ = astutil.Apply(f, func(c *astutil.Cursor) bool {
			switch n := (c.Node()).(type) {

			case *ast.GenDecl:
				switch n.Tok {
				case token.IMPORT:
					// Only keep non-internal standard library imports.
					// This might leave some unused imports, but that's ok.
					newSpecs := make([]ast.Spec, 0, len(n.Specs))
					hasUnsafe := false
					for _, spec := range n.Specs {
						is := spec.(*ast.ImportSpec)
						// Simple heuristic checks.
						if strings.Contains(is.Path.Value, "internal") {
							continue
						}
						if strings.Contains(is.Path.Value, ".") {
							continue
						}
						unquotedPath, err := strconv.Unquote(is.Path.Value)
						if err != nil {
							return false
						}
						name := path.Base(unquotedPath)
						if name == "unsafe" {
							hasUnsafe = true
						}
						if is.Name != nil {
							name = is.Name.Name
						}
						okImports[name] = struct{}{}
						// Reset the position to remove gaps where imports are omitted.
						is.Path.ValuePos = 0
						newSpecs = append(newSpecs, is)
					}
					// Linkname requires unsafe.
					if !hasUnsafe {
						newSpecs = append(newSpecs, &ast.ImportSpec{
							Path: &ast.BasicLit{
								Kind:  token.STRING,
								Value: strconv.Quote("unsafe"),
							},
							Name: &ast.Ident{Name: "_"},
						})
					}
					n.Specs = newSpecs
					return false

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
											fset.Position(n.Pos()),
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
					c.Delete()
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
							fset.Position(n.Pos()),
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

		var sb strings.Builder
		sb.WriteString("// Code generated by xcrypto_backend_map. DO NOT EDIT.\n\n")
		if len(skipMessages) > 0 {
			sb.WriteString("// Some backend functionality was skipped during mapping generation:\n//\n")
			for _, msg := range skipMessages {
				sb.WriteString("// ")
				sb.WriteString(msg)
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}
		if len(parseErrs) > 0 {
			return fmt.Errorf("failed to parse backend file:\n  %v", strings.Join(parseErrs, "\n  "))
		}
		// Preserve the constraint.
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if strings.HasPrefix(c.Text, "//go:build ") {
					sb.WriteString(c.Text)
					sb.WriteString("\n\n")
					break
				}
			}
		}
		// Force the printer to use the comments associated with the nodes.
		f.Comments = nil
		if err := format.Node(&sb, fset, f); err != nil {
			return err
		}
		// Write to destination.
		if *outputPath != "" {
			outPath := filepath.Join(*outputPath, filepath.Base(filename))
			if err := os.WriteFile(outPath, []byte(sb.String()), 0o666); err != nil {
				return err
			}
		} else {
			fmt.Println(sb.String())
		}
	}
	return nil
}

func ensureSimpleType(t ast.Expr, imports map[string]struct{}) error {
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
