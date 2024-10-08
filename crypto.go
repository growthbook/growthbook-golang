package growthbook

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"strings"
)

var (
	ErrCryptoInvalidEncryptedFormat = errors.New("Crypto: encrypted data is in invalid format")
	ErrCryptoInvalidIVLength        = errors.New("Crypto: invalid IV length")
	ErrCryptoInvalidPadding         = errors.New("Crypto: invalid padding")
)

func decrypt(encrypted string, encKey string) (string, error) {
	key, err := base64.StdEncoding.DecodeString(encKey)
	if err != nil {
		return "", err
	}

	splits := strings.Split(encrypted, ".")
	if len(splits) != 2 {
		return "", ErrCryptoInvalidEncryptedFormat
	}

	iv, err := base64.StdEncoding.DecodeString(splits[0])
	if err != nil {
		return "", err
	}

	cipherText, err := base64.StdEncoding.DecodeString(splits[1])
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	if len(iv) != block.BlockSize() {
		return "", ErrCryptoInvalidIVLength
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(cipherText, cipherText)

	cipherText, err = unpad(cipherText)
	if err != nil {
		return "", err
	}

	return string(cipherText), nil
}

// Remove PKCS #7 padding.

func unpad(buf []byte) ([]byte, error) {
	bufLen := len(buf)
	if bufLen == 0 {
		return nil, ErrCryptoInvalidPadding
	}

	pad := buf[bufLen-1]
	if pad == 0 {
		return nil, ErrCryptoInvalidPadding
	}

	padLen := int(pad)
	if padLen > bufLen || padLen > 16 {
		return nil, ErrCryptoInvalidPadding
	}

	for _, v := range buf[bufLen-padLen : bufLen-1] {
		if v != pad {
			return nil, errors.New("crypto/padding: invalid padding size")
		}
	}

	return buf[:bufLen-padLen], nil
}
