package main

import (
	"fmt"

	"golang.org/x/crypto/sha3"
)

func main() {
	s := sha3.New256()
	fmt.Printf("#v\n", s)
}
