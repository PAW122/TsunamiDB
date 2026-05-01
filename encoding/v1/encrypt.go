package encoding_v1

import (
	"crypto/aes"
	"crypto/cipher"
	cryptoRand "crypto/rand"
	"errors"
	"fmt"
	"io"
)

var (
	newCipher  = aes.NewCipher
	newGCM     = cipher.NewGCM
	randReader = cryptoRand.Reader
)

// **Funkcja szyfrująca `Encrypt()`**
func Encrypt(data []byte, key string) ([]byte, error) {
	//  Zamiana klucza na 32-bajtowy klucz AES-256
	aesKey, err := deriveKey(key)
	if err != nil {
		return nil, err
	}

	//  Tworzenie nowej instancji AES
	block, err := newCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("error creating cipher: %w", err)
	}

	//  Użycie GCM (Galois/Counter Mode)
	aesGCM, err := newGCM(block)
	if err != nil {
		return nil, fmt.Errorf("error creating GCM: %w", err)
	}

	//  Tworzenie unikalnego Nonce (12 bajtów)
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(randReader, nonce); err != nil {
		return nil, fmt.Errorf("error generating nonce: %w", err)
	}

	//  Szyfrowanie danych
	ciphertext := aesGCM.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// **Funkcja deszyfrująca `Decrypt()`**
func Decrypt(ciphertext []byte, key string) ([]byte, error) {
	//  Zamiana klucza na 32-bajtowy klucz AES-256
	aesKey, err := deriveKey(key)
	if err != nil {
		return nil, err
	}

	//  Tworzenie AES
	block, err := newCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("error creating cipher: %w", err)
	}

	//  Użycie GCM
	aesGCM, err := newGCM(block)
	if err != nil {
		return nil, fmt.Errorf("error creating GCM: %w", err)
	}

	//  Odczytanie nonce (pierwsze 12 bajtów)
	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	//  Deszyfrowanie
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("error decrypting: %w", err)
	}

	return plaintext, nil
}

// **Konwersja klucza na 32 bajty (AES-256)**
func deriveKey(key string) ([]byte, error) {
	keyBytes := []byte(key)
	if len(keyBytes) == 0 {
		return nil, errors.New("key cannot be empty")
	}

	finalKey := make([]byte, 32)

	for i := 0; i < 32; i++ {
		finalKey[i] = keyBytes[i%len(keyBytes)]
	}

	return finalKey, nil
}
