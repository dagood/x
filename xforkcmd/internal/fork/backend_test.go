package fork

import (
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dagood/x/xforkcmd/internal/goldentest"
)

func TestFindBackendFiles(t *testing.T) {
	got, err := FindBackendFiles("testdata/exampleRealBackend")
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{
		"testdata/exampleRealBackend/cng_windows.go",
		"testdata/exampleRealBackend/boring_linux.go",
		"testdata/exampleRealBackend/openssl_linux.go",
		"testdata/exampleRealBackend/nobackend.go",
	}
	for i, w := range wantPaths {
		wantPaths[i], err = filepath.Abs(w)
		if err != nil {
			t.Fatal(err)
		}
	}
	var gotPaths []string
	for _, b := range got {
		gotPaths = append(gotPaths, b.Filename)
	}
	sort.Strings(wantPaths)
	sort.Strings(gotPaths)
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Errorf("FindBackendFiles() got = %v, want %v", gotPaths, wantPaths)
	}
}

func TestPlaceholderGeneration(t *testing.T) {
	b, err := NewBackendFile("testdata/exampleRealBackend/nobackend.go")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.PlaceholderTrim(); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	if err := b.Write(&sb); err != nil {
		t.Fatal(err)
	}
	got := sb.String()
	goldentest.Check(t, "go test internal/fork", "testdata/derivedplaceholder.go", got)
}

func TestDerivedProxyGeneration(t *testing.T) {
	b, err := NewBackendFile("testdata/exampleRealBackend/openssl_linux.go")
	if err != nil {
		t.Fatal(err)
	}
	api, err := NewBackendFile("testdata/derivedplaceholder.go")
	if err != nil {
		t.Fatal(err)
	}
	proxyAPI, err := b.ProxyAPI(api)
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	if err := proxyAPI.Write(&sb); err != nil {
		t.Fatal(err)
	}
	got := sb.String()
	goldentest.Check(t, "go test internal/fork", "testdata/derivedlinuxproxy.go", got)
}
