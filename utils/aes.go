package utils

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"log"
)

var key = []byte("an example very very secret key.") // 32 bytes key
var iv = []byte("unique init vec.")                  // 16 bytes IV

func pkcs7Padding(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

func pkcs7UnPadding(data []byte) ([]byte, error) {
	length := len(data)
	unpadding := int(data[length-1])
	return data[:(length - unpadding)], nil
}

func AesEncrypt(msg string) string {
	block, err := aes.NewCipher(key)
	if err != nil {
		log.Fatalln(err)
	}

	msgBytes := []byte(msg)
	msgBytes = pkcs7Padding(msgBytes, block.BlockSize())

	ciphertext := make([]byte, len(msgBytes))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, msgBytes)

	return hex.EncodeToString(ciphertext)
}

func AesDecrypt(hexMsg string) string {
	block, err := aes.NewCipher(key)
	if err != nil {
		log.Fatalln(err)
	}

	ciphertext, err := hex.DecodeString(hexMsg)
	if err != nil {
		log.Fatalln(err)
	}

	if len(ciphertext)%aes.BlockSize != 0 {
		log.Fatalln("ciphertext is not a multiple of the block size")
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(ciphertext, ciphertext)

	plaintext, err := pkcs7UnPadding(ciphertext)
	if err != nil {
		log.Fatalln(err)
	}

	return string(plaintext)
}
