package util

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"log"
)

// H1 计算消息与盐值的 SHA256 哈希
func H1(message string) []byte {
	salt := "01"
	data := []byte(message + salt) // 拼接消息和盐值
	hash := sha256.Sum256(data)    // 计算 SHA256 哈希
	return hash[:]                 // 32字节
}

// H2 计算消息与盐值的 SHA256 哈希
func H2(message string) []byte {
	salt := "02"
	data := []byte(message + salt) // 拼接消息和盐值
	hash := sha256.Sum256(data)    // 计算 SHA256 哈希
	return hash[:]                 // 32字节
}

// 加密函数
func AESEncryptCBC(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// 填充明文到块大小的整数倍
	plaintext = pkcs7Pad(plaintext, aes.BlockSize)

	// 生成随机初始化向量 (IV)
	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	// 加密
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext[aes.BlockSize:], plaintext)

	return ciphertext, nil
}

// 解密函数
func AESDecryptCBC(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	if len(ciphertext) < aes.BlockSize {
		return nil, errors.New("ciphertext too short")
	}

	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	// 解密
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(ciphertext, ciphertext)

	// 去除填充
	return pkcs7Unpad(ciphertext, aes.BlockSize)
}

// Xor
func Xor(s1, s2 []byte) []byte {
	if len(s1) != len(s2) {
		log.Fatalln(errors.New("length of s1 and s2 must be equal"))
	}
	r := make([]byte, len(s1))
	for i := 0; i < len(s1); i++ {
		r[i] = s1[i] ^ s2[i]
	}
	return r
}

// PKCS7 填充
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

// PKCS7 去除填充
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data)%blockSize != 0 {
		return nil, errors.New("data is not padded correctly")
	}
	padding := int(data[len(data)-1])
	if padding < 1 || padding > blockSize {
		return nil, errors.New("invalid padding")
	}
	return data[:len(data)-padding], nil
}
