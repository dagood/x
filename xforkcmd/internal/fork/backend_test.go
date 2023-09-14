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
	if err := b.APITrim(); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	if err := b.Write(&sb); err != nil {
		t.Fatal(err)
	}
	got := sb.String()
	goldentest.Check(t, "go test internal/fork", "testdata/derivedapi.golden.go", got)
}

func TestBackendFile_ProxyAPI(t *testing.T) {
	// Note: This uses the golden output of TestPlaceholderGeneration as its input.
	api, err := NewBackendFile("testdata/derivedapi.golden.go")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
	}{
		{"boring_linux"},
		{"cng_windows"},
		{"openssl_linux"},
		{"nobackend"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := NewBackendFile("testdata/exampleRealBackend/" + tt.name + ".go")
			if err != nil {
				t.Fatal(err)
			}
			proxy, err := b.ProxyAPI(api)
			if err != nil {
				t.Fatal(err)
			}
			var sb strings.Builder
			if err := proxy.Write(&sb); err != nil {
				t.Fatal(err)
			}
			got := sb.String()
			goldentest.Check(t, "go test internal/fork", "testdata/proxyDerivedAPI.golden/"+tt.name+"_proxy.go", got)
		})
	}
}