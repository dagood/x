package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"reflect"

	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/sha3"
)

func main() {
	hkdfExample()
	sha3Example()
	fmt.Println(autocert.DefaultACMEDirectory)
}

func sha3Example() {
	buf := []byte("some data to hash")
	hash := sha3.New256()
	fmt.Println(reflect.TypeOf(hash).String())
	r := hash.Sum(buf)
	fmt.Printf("%x\n", r)
}

func hkdfExample() {
	// Underlying hash function for HMAC.
	hash := sha256.New

	// Cryptographically secure master secret.
	secret := []byte{0x00, 0x01, 0x02, 0x03} // i.e. NOT this.

	// Non-secret salt, optional (can be nil).
	// Recommended: hash-length random value.
	salt := make([]byte, hash().Size())
	if _, err := rand.Read(salt); err != nil {
		panic(err)
	}

	// Non-secret context info, optional (can be nil).
	info := []byte("hkdf example")

	// Generate three 128-bit derived keys.
	hkdf := hkdf.New(hash, secret, salt, info)

	var keys [][]byte
	for i := 0; i < 3; i++ {
		key := make([]byte, 16)
		if _, err := io.ReadFull(hkdf, key); err != nil {
			panic(err)
		}
		keys = append(keys, key)
	}

	for i := range keys {
		fmt.Printf("Key #%d: %v\t", i+1, !bytes.Equal(keys[i], make([]byte, 16)))
	}
	fmt.Println()
}
