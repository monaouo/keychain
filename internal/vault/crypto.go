package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"errors"
)

const (
	keyLen  = 32
	saltLen = 16
)

// Iterations 為新建檔的 PBKDF2 次數。
var Iterations = 600_000

var aad = []byte("keychain-v1")

// ErrBadPassword 表示主密碼錯誤或檔案遭竄改。
var ErrBadPassword = errors.New("主密碼錯誤")

func deriveKey(password string, salt []byte, iter int) ([]byte, error) {
	return pbkdf2.Key(sha256.New, password, salt, iter, keyLen)
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func seal(key, plain []byte) (nonce, ct []byte, err error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, nil, err
	}
	nonce = randomBytes(gcm.NonceSize())
	return nonce, gcm.Seal(nil, nonce, plain, aad), nil
}

func open(key, nonce, ct []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, ct, aad)
	if err != nil {
		return nil, ErrBadPassword
	}
	return plain, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
