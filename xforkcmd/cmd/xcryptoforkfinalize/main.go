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

var dev = flag.Bool("dev", false, "development mode: place files in crypto fork. -out must not be specified")

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
	if *dev {
		if *outputDir != "" {
			log.Fatalln("-dev and -out are mutually exclusive")
		}
		*outputDir = *cryptoForkRootDir
	} else {
		if *outputDir == "" {
			log.Fatalln("missing -out")
		}
	}
	if err := run(); err != nil {
		log.Fatalln(err)
	}
}

func run() error {
	proxyDir := filepath.Join(*outputDir, "internal", "backend")
	if *dev {
		if err := fork.RemoveDirContent(proxyDir, !*autoYes); err != nil {
			return err
		}
	} else {
		if err := fork.GitCheckoutTo(*cryptoForkRootDir, *outputDir, !*autoYes); err != nil {
			return err
		}
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
	// Create a proxy for each backend.
	for _, b := range backends {
		if b == backendAPI {
			// This is the unimplemented placeholder API, not a proxy. It's ready to write.
			if err := writeBackend(b, filepath.Join(proxyDir, "nobackend.go")); err != nil {
				return err
			}
			continue
		}
		proxy, err := b.ProxyAPI(backendAPI)
		if err != nil {
			return err
		}
		err = writeBackend(proxy, filepath.Join(proxyDir, filepath.Base(b.Filename)))
		if err != nil {
			return err
		}
	}
	return nil
}

func writeBackend(b fork.FormattedWriterTo, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		return err
	}
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
