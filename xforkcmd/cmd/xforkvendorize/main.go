package main

import (
	"flag"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

// TODO: Make it so you run this program inside a Go tree to set up vendoring automatically?

var submoduleDir = flag.String("submodule", "", "submodule directory")
var outputDir = flag.String("out", "", "output directory")

func main() {
	h := flag.Bool("h", false, "show help")
	flag.Parse()
	if *h {
		flag.Usage()
		return
	}
	if *submoduleDir == "" {
		log.Fatalln("submodule is required")
	}
	if *outputDir == "" {
		log.Fatalln("out is required")
	}
	if err := run(); err != nil {
		log.Fatalln(err)
	}
}

func run() error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	gitDir := *submoduleDir
	outDir, err := filepath.Abs(*outputDir)
	if err != nil {
		return err
	}
	cmd := exec.Command(
		"git",
		"checkout-index",
		"--all",
		"-f",
		"--prefix="+outDir+"/",
	)
	cmd.Dir = gitDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Printf("Wd %q, in %q, running: %v", wd, cmd.Dir, cmd)
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}
