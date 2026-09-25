package growthbook

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// Encrypt already-padded bytes so tests can exercise invalid padding as well as
// valid messages. The fixed IV is for reproducible test fixtures only.
func encryptCryptoTestBytes(t *testing.T, key, padded []byte) string {
	t.Helper()
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	iv := bytes.Repeat([]byte{1}, aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)
	return base64.StdEncoding.EncodeToString(iv) + "." + base64.StdEncoding.EncodeToString(ciphertext)
}

func TestDecryptRoundTrip(t *testing.T) {
	// Complement the fixed interoperability fixtures in cases.json with empty,
	// block-boundary, multiblock, and UTF-8 messages for every supported AES key size.
	for _, keySize := range []int{16, 24, 32} {
		for _, plaintext := range []string{"", "hello", "123456789012345", "1234567890123456", "12345678901234567", "こんにちは 🌍 — a multiblock message"} {
			t.Run(fmt.Sprintf("key%d/bytes%d", keySize, len(plaintext)), func(t *testing.T) {
				key := bytes.Repeat([]byte{2}, keySize)
				padding := aes.BlockSize - len(plaintext)%aes.BlockSize
				padded := append([]byte(plaintext), bytes.Repeat([]byte{byte(padding)}, padding)...)
				encrypted := encryptCryptoTestBytes(t, key, padded)
				actual, err := decrypt(encrypted, base64.StdEncoding.EncodeToString(key))
				require.NoError(t, err)
				require.Equal(t, plaintext, actual)
			})
		}
	}
}

func TestDecryptRejectsMalformedInput(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, aes.BlockSize))
	iv := base64.StdEncoding.EncodeToString(make([]byte, aes.BlockSize))
	body := base64.StdEncoding.EncodeToString(make([]byte, aes.BlockSize))
	for _, tc := range []struct {
		name, encrypted, key string
		wantErr              error
	}{
		{"invalid key encoding", iv + "." + body, "!", base64.CorruptInputError(0)},
		{"invalid key size", iv + "." + body, "AQ==", aes.KeySizeError(1)},
		{"empty key", iv + "." + body, "", aes.KeySizeError(0)},
		{"empty payload", "", key, ErrCryptoInvalidEncryptedFormat},
		{"missing separator", body, key, ErrCryptoInvalidEncryptedFormat},
		{"extra separator", iv + "." + body + ".extra", key, ErrCryptoInvalidEncryptedFormat},
		{"invalid IV encoding", "!." + body, key, base64.CorruptInputError(0)},
		{"invalid body encoding", iv + ".!", key, base64.CorruptInputError(0)},
		{"empty IV", "." + body, key, ErrCryptoInvalidIVLength},
		{"short IV", "AQ==." + body, key, ErrCryptoInvalidIVLength},
		{"long IV", base64.StdEncoding.EncodeToString(make([]byte, 17)) + "." + body, key, ErrCryptoInvalidIVLength},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := decrypt(tc.encrypted, tc.key)
			require.ErrorIs(t, err, tc.wantErr)
			require.Empty(t, actual)
		})
	}
}

func TestDecryptRejectsInvalidCiphertextLength(t *testing.T) {
	for _, size := range []int{0, 1, 15, 17, 31} {
		t.Run(fmt.Sprintf("bytes%d", size), func(t *testing.T) {
			body := base64.StdEncoding.EncodeToString(make([]byte, size))
			key := base64.StdEncoding.EncodeToString(make([]byte, aes.BlockSize))
			actual, err := decrypt("AQEBAQEBAQEBAQEBAQEBAQ==."+body, key)
			require.ErrorIs(t, err, ErrCryptoInvalidCiphertextLength)
			require.Empty(t, actual)
		})
	}
}

func TestDecryptRejectsInvalidPadding(t *testing.T) {
	key := make([]byte, aes.BlockSize)
	// These decrypt to known bad padding, avoiding probabilistic wrong-key tests.
	for _, padded := range [][]byte{
		make([]byte, aes.BlockSize),
		bytes.Repeat([]byte{17}, aes.BlockSize),
		append(bytes.Repeat([]byte{1}, aes.BlockSize-1), 2),
	} {
		t.Run(fmt.Sprintf("lastByte%d", padded[len(padded)-1]), func(t *testing.T) {
			encrypted := encryptCryptoTestBytes(t, key, padded)
			actual, err := decrypt(encrypted, base64.StdEncoding.EncodeToString(key))
			require.Error(t, err)
			require.Empty(t, actual)
		})
	}
}

func TestUnpad(t *testing.T) {
	// PKCS #7 allows 1–16 identical padding bytes, including a full padding block.
	for padding := 1; padding <= aes.BlockSize; padding++ {
		t.Run(fmt.Sprintf("padding%d", padding), func(t *testing.T) {
			want := bytes.Repeat([]byte{'x'}, aes.BlockSize-padding)
			padded := append(bytes.Clone(want), bytes.Repeat([]byte{byte(padding)}, padding)...)
			actual, err := unpad(padded)
			require.NoError(t, err)
			require.Equal(t, want, actual)
		})
	}
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"zero padding", []byte{0}},
		{"padding exceeds buffer", []byte{2}},
		{"padding exceeds block", bytes.Repeat([]byte{17}, 17)},
		{"mismatched padding bytes", []byte{'x', 1, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := unpad(tc.data)
			require.Error(t, err)
			require.Nil(t, actual)
		})
	}
}
