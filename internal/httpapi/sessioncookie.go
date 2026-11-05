package httpapi

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"
)

// Passkey 的挑战码会话需要临时保存在客户端（HttpOnly Cookie），
// 这里用进程内随机密钥做 AES-GCM 封装，保证内容不可伪造、也不会超出 4KB Cookie 上限。

var cookieKey [32]byte

func init() {
	_, _ = rand.Read(cookieKey[:])
}

func encodeSessionCookie(plain []byte) string {
	block, err := aes.NewCipher(cookieKey[:])
	if err != nil {
		return ""
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return ""
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return ""
	}
	out := gcm.Seal(nonce, nonce, plain, nil)
	return base64.RawURLEncoding.EncodeToString(out)
}

func decodeSessionCookie(value string) ([]byte, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, false
	}
	block, err := aes.NewCipher(cookieKey[:])
	if err != nil {
		return nil, false
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(raw) < gcm.NonceSize() {
		return nil, false
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return nil, false
	}
	return plain, true
}
