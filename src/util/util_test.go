package util

import (
	"encoding/hex"
	"fmt"
	"testing"
)

func TestAES(t *testing.T) {
	var KEYFORAES = []byte("secure-KEYFORAES")
	plaintext := []byte("Hello, AES对称加密!")

	// 加密
	ciphertext, err := AESEncryptCBC(KEYFORAES, plaintext)
	if err != nil {
		panic(err)
	}
	fmt.Printf("加密结果 (HEX): %s\n", hex.EncodeToString(ciphertext))
	// 解密
	decrypted, err := AESDecryptCBC(KEYFORAES, ciphertext)
	if err != nil {
		panic(err)
	}
	fmt.Printf("解密结果: %s\n", decrypted)
}

func TestXor(t *testing.T) {
	a, b := []byte("aa"), []byte("bb")
	r := make([]byte, len(a))
	for i := 0; i < len(a); i++ {
		r[i] = a[i] ^ b[i]
		r[i] = r[i] ^ b[i]
	}
	fmt.Printf("%s\n", r)
}
