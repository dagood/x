package main

import (
	"flag"
	"log"

	"github.com/dagood/x/xforkcmd/internal/fork"
)

var cryptoForkRootDir = flag.String("fork", "", "crypto fork root directory")
var backendDir = flag.String("backend", "", "directory with Go files that implement the backend")
var outputDir = flag.String("out", "", "output directory")

var autoYes = flag.Bool("y", false, "delete old output and overwrite without prompting")

func main() {
	h := flag.Bool("h", false, "show help")
	flag.Parse()
	if *h {
		flag.Usage()
		return
	}
	if *cryptoForkRootDir == "" {
		log.Fatalln("missing -fork")
	}
	if *backendDir == "" {
		log.Fatalln("missing -backend")
	}
	if *outputDir == "" {
		log.Fatalln("missing -out")
	}
	if err := run(); err != nil {
		log.Fatalln(err)
	}
}

func run() error {
	if err := fork.GitCheckoutTo(*cryptoForkRootDir, *outputDir, !*autoYes); err != nil {
		return err
	}
	return nil
}
