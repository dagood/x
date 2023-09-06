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
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

var (
	backendPattern = flag.String("f", "", "backend Go file glob")
	outputPath     = flag.String("o", "", "output directory")
)

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
		if filepath.Base(filename) == "nobackend.go" {
			continue
		}
		f, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		if en := f.Scope.Lookup("Enabled"); en == nil {
			continue
		}
		var parseErrs []string
		//v := &visitor{}
		//ast.Walk(v, f)
		//cmap := ast.NewCommentMap(fset, f, f.Comments)
		_ = astutil.Apply(f, func(c *astutil.Cursor) bool {
			switch n := (c.Node()).(type) {
			//case *ast.File:
			// Reset comments to force the printer to use the values associated with the nodes.
			//n.Comments = nil
			// Remove the function body of every function.
			case *ast.GenDecl:
				// If this is something we care about, we need it to be "var"
				// so linkname to the backend's const works. If this isn't
				// something we care about, we'll delete it later anyway.
				if n.Tok == token.CONST {
					n.Tok = token.VAR
				}
				return true
			case *ast.ValueSpec:
				var ns []*ast.Ident
				for _, name := range n.Names {
					if name.IsExported() {
						ns = append(ns, name)
					}
				}
				if len(ns) == 0 {
					c.Delete()
					return false
				}
			case *ast.TypeSpec:
				if !n.Name.IsExported() {
					c.Delete()
					return false
				}
			case *ast.FuncDecl:
				if !n.Name.IsExported() {
					c.Delete()
					return false
				}
				var cg ast.CommentGroup
				for _, cmd := range funcCommands(n) {
					switch cmd {
					case "noinline":
						cg.List = append(cg.List, &ast.Comment{
							Text: "//go:noinline",
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
			default:
			}
			return true
		}, nil)

		if len(parseErrs) > 0 {
			return fmt.Errorf("failed to parse backend file:\n  %v", strings.Join(parseErrs, "\n  "))
		}

		// Force the printer to use the comments associated with the nodes.
		f.Comments = nil
		if err := format.Node(os.Stdout, fset, f); err != nil {
			return err
		}
	}
	return nil
}

func funcCommands(n ast.Node) []string {
	var cmds []string
	ast.Inspect(n, func(n ast.Node) bool {
		if n, ok := n.(*ast.Comment); !ok {
			return true
		} else if cmd, ok := strings.CutPrefix(n.Text, "//xcrypto_backend_map:"); !ok {
			return true
		} else {
			cmds = append(cmds, cmd)
		}
		return false
	})
	return cmds
}
