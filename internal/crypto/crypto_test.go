package crypto

import (
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := "my-secret-key-for-testing-12345"
	plaintext := "EAABsbCS1iHgBO..."

	encrypted, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if encrypted == plaintext {
		t.Fatal("Encrypted value should differ from plaintext")
	}

	if !IsEncrypted(encrypted) {
		t.Fatal("Encrypted value should have enc: prefix")
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != plaintext {
		t.Fatalf("Decrypted value %q != plaintext %q", decrypted, plaintext)
	}
}

func TestDecrypt_LegacyUnencrypted(t *testing.T) {
	key := "my-secret-key"
	legacy := "plain-text-token-without-prefix"

	decrypted, err := Decrypt(legacy, key)
	if err != nil {
		t.Fatalf("Decrypt legacy failed: %v", err)
	}
	if decrypted != legacy {
		t.Fatalf("Legacy value should be returned as-is, got %q", decrypted)
	}
}

func TestEncryptDecrypt_EmptyKey(t *testing.T) {
	plaintext := "some-secret"

	encrypted, err := Encrypt(plaintext, "")
	if err != nil {
		t.Fatalf("Encrypt with empty key failed: %v", err)
	}
	if encrypted != plaintext {
		t.Fatal("Empty key should return plaintext unchanged")
	}

	decrypted, err := Decrypt(plaintext, "")
	if err != nil {
		t.Fatalf("Decrypt with empty key failed: %v", err)
	}
	if decrypted != plaintext {
		t.Fatal("Empty key should return ciphertext unchanged")
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	key1 := "correct-key"
	key2 := "wrong-key"

	encrypted, _ := Encrypt("secret", key1)
	_, err := Decrypt(encrypted, key2)
	if err == nil {
		t.Fatal("Decrypt with wrong key should fail")
	}
}

func TestEncryptFields(t *testing.T) {
	key := "field-key-for-testing"
	token := "page-token"
	secret := "app-secret"
	empty := ""

	if err := EncryptFields(key, &token, &secret, &empty, nil); err != nil {
		t.Fatalf("EncryptFields failed: %v", err)
	}
	if !IsEncrypted(token) || !IsEncrypted(secret) {
		t.Fatal("non-empty fields should be encrypted")
	}
	if empty != "" {
		t.Fatal("empty field should stay empty")
	}

	// Already-encrypted values must not be double-encrypted.
	before := token
	if err := EncryptFields(key, &token); err != nil {
		t.Fatalf("EncryptFields (second pass) failed: %v", err)
	}
	if token != before {
		t.Fatal("already-encrypted field should be left unchanged")
	}

	DecryptFields(key, &token, &secret)
	if token != "page-token" || secret != "app-secret" {
		t.Fatalf("round trip mismatch: %q %q", token, secret)
	}
}

func TestEncryptFields_EmptyKey(t *testing.T) {
	v := "plain"
	if err := EncryptFields("", &v); err != nil {
		t.Fatalf("EncryptFields failed: %v", err)
	}
	if v != "plain" {
		t.Fatal("empty key should leave value unchanged (dev mode)")
	}
}
