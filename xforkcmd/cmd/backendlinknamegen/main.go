package main

import (
	"flag"
	"go/parser"
	"go/token"
	"log"
	"path/filepath"
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
		f, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		_ = f
	}
	return nil
}
