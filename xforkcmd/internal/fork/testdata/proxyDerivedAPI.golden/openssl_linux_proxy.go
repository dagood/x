// Generated code. DO NOT EDIT.

// This file implements a proxy that links into a specific crypto backend.

//go:build goexperiment.opensslcrypto && linux && cgo

package backend

import (
	"crypto/cipher"
	_ "unsafe"
	"hash"
)
//go:linkname NewSHA1 crypto/internal/backend.NewSHA1
func NewSHA1() hash.Hash
//go:linkname NewSHA224 crypto/internal/backend.NewSHA224
func NewSHA224() hash.Hash
//go:linkname NewSHA256 crypto/internal/backend.NewSHA256
func NewSHA256() hash.Hash
//go:linkname NewSHA384 crypto/internal/backend.NewSHA384
func NewSHA384() hash.Hash
//go:linkname NewSHA512 crypto/internal/backend.NewSHA512
func NewSHA512() hash.Hash
//go:linkname NewSHA3_256 crypto/internal/backend.NewSHA3_256
func NewSHA3_256() hash.Hash
//go:linkname SHA1 crypto/internal/backend.SHA1
//go:noescape
func SHA1(p []byte) (sum []byte)
//go:linkname SHA224 crypto/internal/backend.SHA224
//go:noescape
func SHA224(p []byte) (sum []byte)
//go:linkname SHA256 crypto/internal/backend.SHA256
//go:noescape
func SHA256(p []byte) (sum []byte)
//go:linkname SHA384 crypto/internal/backend.SHA384
//go:noescape
func SHA384(p []byte) (sum []byte)
//go:linkname SHA512 crypto/internal/backend.SHA512
//go:noescape
func SHA512(p []byte) (sum []byte)
//go:linkname SHA3_256 crypto/internal/backend.SHA3_256
//go:noescape
func SHA3_256(p []byte) (sum []byte)
//go:linkname NewHMAC crypto/internal/backend.NewHMAC
func NewHMAC(h func() hash.Hash, key []byte) hash.Hash
//go:linkname NewAESCipher crypto/internal/backend.NewAESCipher
func NewAESCipher(key []byte) (cipher.Block, error)
//go:linkname NewGCMTLS crypto/internal/backend.NewGCMTLS
func NewGCMTLS(c cipher.Block) (cipher.AEAD, error)
