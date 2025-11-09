package security

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultEncryptionConfig(t *testing.T) {
	config := DefaultEncryptionConfig()

	if config == nil {
		t.Fatal("Expected non-nil config")
	}

	if config.Enabled {
		t.Error("Expected encryption to be disabled by default")
	}

	if config.Algorithm != EncryptionAlgorithmAES256GCM {
		t.Errorf("Expected AES-256-GCM algorithm, got %s", config.Algorithm)
	}

	if config.KeyRotationInterval != 24*time.Hour*30 {
		t.Error("Expected 30 day key rotation interval")
	}

	if config.PBKDF2Iterations != 100000 {
		t.Error("Expected 100000 PBKDF2 iterations")
	}
}

func TestNewKeyManager_Disabled(t *testing.T) {
	config := &EncryptionConfig{
		Enabled: false,
	}

	km, err := NewKeyManager(config)
	if err != nil {
		t.Fatalf("Failed to create key manager: %v", err)
	}

	if km == nil {
		t.Fatal("Expected non-nil key manager")
	}
}

func TestNewKeyManager_Enabled(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "streambus-encryption-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config := &EncryptionConfig{
		Enabled:             true,
		Algorithm:           EncryptionAlgorithmAES256GCM,
		MasterKeyPath:       filepath.Join(tmpDir, "master.key"),
		KeyRotationInterval: 1 * time.Hour,
		PBKDF2Iterations:    100000,
	}

	km, err := NewKeyManager(config)
	if err != nil {
		t.Fatalf("Failed to create key manager: %v", err)
	}
	defer km.Close()

	if km.masterKey == nil {
		t.Error("Expected master key to be generated")
	}

	if len(km.masterKey.Key) != 32 {
		t.Errorf("Expected 32-byte master key, got %d bytes", len(km.masterKey.Key))
	}

	if km.activeKeyID == "" {
		t.Error("Expected active key ID to be set")
	}

	// Verify master key was saved
	if _, err := os.Stat(config.MasterKeyPath); os.IsNotExist(err) {
		t.Error("Master key file was not created")
	}
}

func TestKeyManager_LoadExistingMasterKey(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "streambus-encryption-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	masterKeyPath := filepath.Join(tmpDir, "master.key")

	config := &EncryptionConfig{
		Enabled:       true,
		MasterKeyPath: masterKeyPath,
	}

	// Create first key manager
	km1, err := NewKeyManager(config)
	if err != nil {
		t.Fatalf("Failed to create first key manager: %v", err)
	}
	originalKeyID := km1.masterKey.ID
	originalKey := make([]byte, len(km1.masterKey.Key))
	copy(originalKey, km1.masterKey.Key)
	km1.Close()

	// Create second key manager - should load existing key
	km2, err := NewKeyManager(config)
	if err != nil {
		t.Fatalf("Failed to create second key manager: %v", err)
	}
	defer km2.Close()

	if km2.masterKey.ID != originalKeyID {
		t.Error("Loaded key ID doesn't match original")
	}

	if !bytes.Equal(km2.masterKey.Key, originalKey) {
		t.Error("Loaded key doesn't match original")
	}
}

func TestKeyManager_GetActiveKey(t *testing.T) {
	config := &EncryptionConfig{
		Enabled: true,
	}

	km, err := NewKeyManager(config)
	if err != nil {
		t.Fatalf("Failed to create key manager: %v", err)
	}
	defer km.Close()

	key, keyID, err := km.GetActiveKey()
	if err != nil {
		t.Fatalf("Failed to get active key: %v", err)
	}

	if len(key) != 32 {
		t.Errorf("Expected 32-byte key, got %d bytes", len(key))
	}

	if keyID == "" {
		t.Error("Expected non-empty key ID")
	}

	// Verify we can get the same key by ID
	key2, err := km.GetKey(keyID)
	if err != nil {
		t.Fatalf("Failed to get key by ID: %v", err)
	}

	if !bytes.Equal(key, key2) {
		t.Error("Keys don't match")
	}
}

func TestKeyManager_RotateDataKey(t *testing.T) {
	config := &EncryptionConfig{
		Enabled:             true,
		KeyRotationInterval: 1 * time.Hour,
	}

	km, err := NewKeyManager(config)
	if err != nil {
		t.Fatalf("Failed to create key manager: %v", err)
	}
	defer km.Close()

	// Get initial active key
	_, initialKeyID, err := km.GetActiveKey()
	if err != nil {
		t.Fatalf("Failed to get initial key: %v", err)
	}

	// Rotate key
	if err := km.rotateDataKey(); err != nil {
		t.Fatalf("Failed to rotate key: %v", err)
	}

	// Get new active key
	_, newKeyID, err := km.GetActiveKey()
	if err != nil {
		t.Fatalf("Failed to get new key: %v", err)
	}

	// Keys should be different
	if initialKeyID == newKeyID {
		t.Error("Expected different key IDs after rotation")
	}

	// Old key should still be accessible
	_, err = km.GetKey(initialKeyID)
	if err != nil {
		t.Error("Old key should still be accessible")
	}

	// Old key should be marked inactive
	oldKey := km.dataKeys[initialKeyID]
	if oldKey.Active {
		t.Error("Old key should be marked as inactive")
	}

	// New key should be active
	newKey := km.dataKeys[newKeyID]
	if !newKey.Active {
		t.Error("New key should be marked as active")
	}
}

func TestKeyManager_GetKey_NotFound(t *testing.T) {
	config := &EncryptionConfig{
		Enabled: true,
	}

	km, err := NewKeyManager(config)
	if err != nil {
		t.Fatalf("Failed to create key manager: %v", err)
	}
	defer km.Close()

	_, err = km.GetKey("nonexistent-key")
	if err == nil {
		t.Error("Expected error for non-existent key")
	}
}

func TestNewEncryptionService(t *testing.T) {
	config := &EncryptionConfig{
		Enabled: true,
	}

	es, err := NewEncryptionService(config)
	if err != nil {
		t.Fatalf("Failed to create encryption service: %v", err)
	}
	defer es.Close()

	if es == nil {
		t.Fatal("Expected non-nil encryption service")
	}

	if es.keyManager == nil {
		t.Error("Expected key manager to be initialized")
	}
}

func TestEncryptionService_Encrypt_Decrypt(t *testing.T) {
	config := &EncryptionConfig{
		Enabled: true,
	}

	es, err := NewEncryptionService(config)
	if err != nil {
		t.Fatalf("Failed to create encryption service: %v", err)
	}
	defer es.Close()

	testData := []byte("Hello, StreamBus! This is secret data.")

	// Encrypt
	encrypted, err := es.Encrypt(testData)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	if encrypted == nil {
		t.Fatal("Expected non-nil encrypted data")
	}

	if encrypted.KeyID == "" {
		t.Error("Expected key ID to be set")
	}

	if len(encrypted.Nonce) == 0 {
		t.Error("Expected nonce to be set")
	}

	if len(encrypted.Data) == 0 {
		t.Error("Expected encrypted data")
	}

	// Encrypted data should be different from plaintext
	if bytes.Equal(encrypted.Data, testData) {
		t.Error("Encrypted data should differ from plaintext")
	}

	// Decrypt
	decrypted, err := es.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	// Decrypted should match original
	if !bytes.Equal(decrypted, testData) {
		t.Errorf("Decrypted data doesn't match original.\nExpected: %s\nGot: %s", testData, decrypted)
	}
}

func TestEncryptionService_Disabled(t *testing.T) {
	config := &EncryptionConfig{
		Enabled: false,
	}

	es, err := NewEncryptionService(config)
	if err != nil {
		t.Fatalf("Failed to create encryption service: %v", err)
	}
	defer es.Close()

	testData := []byte("Unencrypted data")

	// Encrypt should pass through
	encrypted, err := es.Encrypt(testData)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	// Data should be unchanged
	if !bytes.Equal(encrypted.Data, testData) {
		t.Error("Data should be unchanged when encryption is disabled")
	}

	// Decrypt should also pass through
	decrypted, err := es.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	if !bytes.Equal(decrypted, testData) {
		t.Error("Decrypted data doesn't match original")
	}
}

func TestEncryptionService_EncryptToBase64_DecryptFromBase64(t *testing.T) {
	config := &EncryptionConfig{
		Enabled: true,
	}

	es, err := NewEncryptionService(config)
	if err != nil {
		t.Fatalf("Failed to create encryption service: %v", err)
	}
	defer es.Close()

	testData := []byte("Base64 encoded encrypted data")

	// Encrypt to base64
	encoded, err := es.EncryptToBase64(testData)
	if err != nil {
		t.Fatalf("Failed to encrypt to base64: %v", err)
	}

	if encoded == "" {
		t.Error("Expected non-empty base64 string")
	}

	// Decrypt from base64
	decrypted, err := es.DecryptFromBase64(encoded)
	if err != nil {
		t.Fatalf("Failed to decrypt from base64: %v", err)
	}

	if !bytes.Equal(decrypted, testData) {
		t.Errorf("Decrypted data doesn't match original.\nExpected: %s\nGot: %s", testData, decrypted)
	}
}

func TestEncryptionService_MultipleEncryptions(t *testing.T) {
	config := &EncryptionConfig{
		Enabled: true,
	}

	es, err := NewEncryptionService(config)
	if err != nil {
		t.Fatalf("Failed to create encryption service: %v", err)
	}
	defer es.Close()

	testData := []byte("Test data for multiple encryptions")

	// Encrypt same data multiple times
	encrypted1, err := es.Encrypt(testData)
	if err != nil {
		t.Fatalf("Failed first encryption: %v", err)
	}

	encrypted2, err := es.Encrypt(testData)
	if err != nil {
		t.Fatalf("Failed second encryption: %v", err)
	}

	// Encrypted data should be different due to different nonces
	if bytes.Equal(encrypted1.Data, encrypted2.Data) {
		t.Error("Multiple encryptions of same data should produce different ciphertexts")
	}

	// Both should decrypt to same plaintext
	decrypted1, err := es.Decrypt(encrypted1)
	if err != nil {
		t.Fatalf("Failed to decrypt first: %v", err)
	}

	decrypted2, err := es.Decrypt(encrypted2)
	if err != nil {
		t.Fatalf("Failed to decrypt second: %v", err)
	}

	if !bytes.Equal(decrypted1, testData) || !bytes.Equal(decrypted2, testData) {
		t.Error("Decrypted data doesn't match original")
	}
}

func TestEncryptionService_DecryptWithOldKey(t *testing.T) {
	config := &EncryptionConfig{
		Enabled:             true,
		KeyRotationInterval: 1 * time.Hour,
	}

	es, err := NewEncryptionService(config)
	if err != nil {
		t.Fatalf("Failed to create encryption service: %v", err)
	}
	defer es.Close()

	testData := []byte("Data encrypted with old key")

	// Encrypt with current key
	encrypted, err := es.Encrypt(testData)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	oldKeyID := encrypted.KeyID

	// Rotate key
	if err := es.keyManager.rotateDataKey(); err != nil {
		t.Fatalf("Failed to rotate key: %v", err)
	}

	// Verify new active key is different
	_, newKeyID, err := es.keyManager.GetActiveKey()
	if err != nil {
		t.Fatalf("Failed to get new key: %v", err)
	}

	if oldKeyID == newKeyID {
		t.Error("Key should have changed after rotation")
	}

	// Should still be able to decrypt with old key
	decrypted, err := es.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Failed to decrypt with old key: %v", err)
	}

	if !bytes.Equal(decrypted, testData) {
		t.Error("Decrypted data doesn't match original")
	}
}

func TestEncryptionService_LargeData(t *testing.T) {
	config := &EncryptionConfig{
		Enabled: true,
	}

	es, err := NewEncryptionService(config)
	if err != nil {
		t.Fatalf("Failed to create encryption service: %v", err)
	}
	defer es.Close()

	// Create 1MB of test data
	testData := make([]byte, 1024*1024)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	// Encrypt
	encrypted, err := es.Encrypt(testData)
	if err != nil {
		t.Fatalf("Failed to encrypt large data: %v", err)
	}

	// Decrypt
	decrypted, err := es.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Failed to decrypt large data: %v", err)
	}

	if !bytes.Equal(decrypted, testData) {
		t.Error("Large data decryption failed")
	}
}

func TestDeriveKeyFromPassword(t *testing.T) {
	password := "test-password"
	salt, err := GenerateSalt()
	if err != nil {
		t.Fatalf("Failed to generate salt: %v", err)
	}

	key1 := DeriveKeyFromPassword(password, salt, 100000)
	if len(key1) != 32 {
		t.Errorf("Expected 32-byte key, got %d", len(key1))
	}

	// Same password and salt should produce same key
	key2 := DeriveKeyFromPassword(password, salt, 100000)
	if !bytes.Equal(key1, key2) {
		t.Error("Keys derived from same password and salt should match")
	}

	// Different salt should produce different key
	salt2, _ := GenerateSalt()
	key3 := DeriveKeyFromPassword(password, salt2, 100000)
	if bytes.Equal(key1, key3) {
		t.Error("Keys derived with different salts should differ")
	}

	// Different password should produce different key
	key4 := DeriveKeyFromPassword("different-password", salt, 100000)
	if bytes.Equal(key1, key4) {
		t.Error("Keys derived from different passwords should differ")
	}
}

func TestGenerateSalt(t *testing.T) {
	salt1, err := GenerateSalt()
	if err != nil {
		t.Fatalf("Failed to generate salt: %v", err)
	}

	if len(salt1) != 32 {
		t.Errorf("Expected 32-byte salt, got %d", len(salt1))
	}

	// Generate another salt - should be different
	salt2, err := GenerateSalt()
	if err != nil {
		t.Fatalf("Failed to generate second salt: %v", err)
	}

	if bytes.Equal(salt1, salt2) {
		t.Error("Generated salts should be different")
	}
}

func TestEncryptionService_EmptyData(t *testing.T) {
	config := &EncryptionConfig{
		Enabled: true,
	}

	es, err := NewEncryptionService(config)
	if err != nil {
		t.Fatalf("Failed to create encryption service: %v", err)
	}
	defer es.Close()

	testData := []byte("")

	encrypted, err := es.Encrypt(testData)
	if err != nil {
		t.Fatalf("Failed to encrypt empty data: %v", err)
	}

	decrypted, err := es.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Failed to decrypt empty data: %v", err)
	}

	if !bytes.Equal(decrypted, testData) {
		t.Error("Empty data encryption/decryption failed")
	}
}

func TestEncryptedData_Tampering(t *testing.T) {
	config := &EncryptionConfig{
		Enabled: true,
	}

	es, err := NewEncryptionService(config)
	if err != nil {
		t.Fatalf("Failed to create encryption service: %v", err)
	}
	defer es.Close()

	testData := []byte("Sensitive data")

	encrypted, err := es.Encrypt(testData)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	// Tamper with encrypted data
	encrypted.Data[0] ^= 0xFF

	// Decryption should fail
	_, err = es.Decrypt(encrypted)
	if err == nil {
		t.Error("Expected decryption to fail with tampered data")
	}
}

func TestKeyManager_ConcurrentAccess(t *testing.T) {
	config := &EncryptionConfig{
		Enabled: true,
	}

	km, err := NewKeyManager(config)
	if err != nil {
		t.Fatalf("Failed to create key manager: %v", err)
	}
	defer km.Close()

	// Concurrent access to keys
	done := make(chan bool)
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_, _, err := km.GetActiveKey()
				if err != nil {
					t.Errorf("Failed to get active key: %v", err)
				}
			}
			done <- true
		}()
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

func BenchmarkEncryption(b *testing.B) {
	config := &EncryptionConfig{
		Enabled: true,
	}

	es, err := NewEncryptionService(config)
	if err != nil {
		b.Fatalf("Failed to create encryption service: %v", err)
	}
	defer es.Close()

	testData := make([]byte, 1024) // 1KB
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := es.Encrypt(testData)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
	}
}

func BenchmarkDecryption(b *testing.B) {
	config := &EncryptionConfig{
		Enabled: true,
	}

	es, err := NewEncryptionService(config)
	if err != nil {
		b.Fatalf("Failed to create encryption service: %v", err)
	}
	defer es.Close()

	testData := make([]byte, 1024) // 1KB
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	encrypted, err := es.Encrypt(testData)
	if err != nil {
		b.Fatalf("Encryption failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := es.Decrypt(encrypted)
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
	}
}
