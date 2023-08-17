package sha3

import (
	"testing"
)

func Test256(t *testing.T) {
	s := New256()
	t.Log(s)
}
