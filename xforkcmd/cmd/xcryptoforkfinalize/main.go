package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

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
	// For now, use the nobackend as a source of truth for the API. This keeps
	// maintenance cost low while only one Go toolset implements the API.
	//
	// When sharing the API among multiple Go toolset forks, it is probably
	// better to make the API/placeholder itself be the source of truth, so it
	// receives only intentional changes.
	backends, err := fork.FindBackendFiles(*backendDir)
	if err != nil {
		return err
	}
	var backendAPI *fork.BackendFile
	for _, b := range backends {
		if b.Filename == filepath.Join(*backendDir, "nobackend.go") {
			if err := b.APITrim(); err != nil {
				return err
			}
			backendAPI = b
			break
		}
	}
	if backendAPI == nil {
		return fmt.Errorf("no backend found appears to be nobackend: %v", backends)
	}
	backendPath := filepath.Join(*outputDir, "backend")
	// Create a proxy for each backend.
	for _, b := range backends {
		if b == backendAPI {
			// This is the unimplemented placeholder API, not a proxy. It's ready to write.
			if err := writeBackend(b, filepath.Join(backendPath, "nobackend.go")); err != nil {
				return err
			}
			continue
		}
		proxy, err := b.ProxyAPI(backendAPI)
		if err != nil {
			return err
		}
		err = writeBackend(proxy, filepath.Join(backendPath, filepath.Base(b.Filename)))
		if err != nil {
			return err
		}
	}
	return nil
}

func writeBackend(b fork.FormattedWriterTo, path string) error {
	apiFile, err := os.Create(path)
	if err != nil {
		return err
	}
	err = b.Format(apiFile)
	if err2 := apiFile.Close(); err == nil {
		err = err2
	}
	return err
}
