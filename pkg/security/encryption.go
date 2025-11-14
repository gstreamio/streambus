package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

// EncryptionAlgorithm represents the encryption algorithm used
type EncryptionAlgorithm string

const (
	// EncryptionAlgorithmAES256GCM uses AES-256 in GCM mode
	EncryptionAlgorithmAES256GCM EncryptionAlgorithm = "AES-256-GCM"
)

// EncryptionConfig holds encryption configuration
type EncryptionConfig struct {
	// Enabled indicates if encryption at rest is enabled
	Enabled bool

	// Algorithm is the encryption algorithm to use
	Algorithm EncryptionAlgorithm

	// MasterKeyPath is the path to the master key file
	MasterKeyPath string

	// KeyRotationInterval is how often to rotate data encryption keys
	KeyRotationInterval time.Duration

	// PBKDF2Iterations is the number of iterations for PBKDF2 key derivation
	PBKDF2Iterations int
}

// DefaultEncryptionConfig returns default encryption configuration
func DefaultEncryptionConfig() *EncryptionConfig {
	return &EncryptionConfig{
		Enabled:             false,
		Algorithm:           EncryptionAlgorithmAES256GCM,
		KeyRotationInterval: 24 * time.Hour * 30, // 30 days
		PBKDF2Iterations:    100000,
	}
}

// MasterKey represents the master encryption key
type MasterKey struct {
	ID        string    `json:"id"`
	Key       []byte    `json:"key"`
	CreatedAt time.Time `json:"created_at"`
}

// DataEncryptionKey represents a data encryption key
type DataEncryptionKey struct {
	ID        string    `json:"id"`
	Key       []byte    `json:"key"` // Encrypted with master key
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Active    bool      `json:"active"`
}

// EncryptedData represents encrypted data with metadata
type EncryptedData struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"key_id"`
	Nonce     []byte `json:"nonce"`
	Data      []byte `json:"data"`
	CreatedAt time.Time `json:"created_at"`
}

// KeyManager manages encryption keys
type KeyManager struct {
	mu              sync.RWMutex
	masterKey       *MasterKey
	dataKeys        map[string]*DataEncryptionKey
	activeKeyID     string
	config          *EncryptionConfig
	keyRotationStop chan struct{}
}

// NewKeyManager creates a new key manager
func NewKeyManager(config *EncryptionConfig) (*KeyManager, error) {
	if !config.Enabled {
		return &KeyManager{
			config:  config,
			dataKeys: make(map[string]*DataEncryptionKey),
		}, nil
	}

	km := &KeyManager{
		config:          config,
		dataKeys:        make(map[string]*DataEncryptionKey),
		keyRotationStop: make(chan struct{}),
	}

	// Load or generate master key
	if err := km.loadOrGenerateMasterKey(); err != nil {
		return nil, fmt.Errorf("failed to load master key: %w", err)
	}

	// Generate initial data encryption key
	if err := km.rotateDataKey(); err != nil {
		return nil, fmt.Errorf("failed to generate initial data key: %w", err)
	}

	// Start key rotation if configured
	if config.KeyRotationInterval > 0 {
		go km.startKeyRotation()
	}

	return km, nil
}

// loadOrGenerateMasterKey loads or generates a master key
func (km *KeyManager) loadOrGenerateMasterKey() error {
	if km.config.MasterKeyPath != "" {
		// Try to load existing key
		if err := km.loadMasterKey(km.config.MasterKeyPath); err == nil {
			return nil
		}
	}

	// Generate new master key
	return km.generateMasterKey()
}

// generateMasterKey generates a new master key
func (km *KeyManager) generateMasterKey() error {
	key := make([]byte, 32) // 256 bits
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return fmt.Errorf("failed to generate master key: %w", err)
	}

	km.masterKey = &MasterKey{
		ID:        generateKeyID(),
		Key:       key,
		CreatedAt: time.Now(),
	}

	// Save master key if path is configured
	if km.config.MasterKeyPath != "" {
		if err := km.saveMasterKey(km.config.MasterKeyPath); err != nil {
			return fmt.Errorf("failed to save master key: %w", err)
		}
	}

	return nil
}

// loadMasterKey loads a master key from file
func (km *KeyManager) loadMasterKey(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var masterKey MasterKey
	if err := json.Unmarshal(data, &masterKey); err != nil {
		return err
	}

	km.masterKey = &masterKey
	return nil
}

// saveMasterKey saves the master key to file
func (km *KeyManager) saveMasterKey(path string) error {
	data, err := json.Marshal(km.masterKey)
	if err != nil {
		return err
	}

	// Write with restricted permissions
	return os.WriteFile(path, data, 0600)
}

// rotateDataKey generates a new data encryption key
func (km *KeyManager) rotateDataKey() error {
	km.mu.Lock()
	defer km.mu.Unlock()

	// Generate new data key
	key := make([]byte, 32) // 256 bits
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return fmt.Errorf("failed to generate data key: %w", err)
	}

	// Encrypt data key with master key
	encryptedKey, err := km.encryptWithMasterKey(key)
	if err != nil {
		return fmt.Errorf("failed to encrypt data key: %w", err)
	}

	keyID := generateKeyID()
	dataKey := &DataEncryptionKey{
		ID:        keyID,
		Key:       encryptedKey,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(km.config.KeyRotationInterval),
		Active:    true,
	}

	// Mark old active key as inactive
	if km.activeKeyID != "" {
		if oldKey, exists := km.dataKeys[km.activeKeyID]; exists {
			oldKey.Active = false
		}
	}

	// Set new key as active
	km.dataKeys[keyID] = dataKey
	km.activeKeyID = keyID

	return nil
}

// encryptWithMasterKey encrypts data with the master key
func (km *KeyManager) encryptWithMasterKey(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(km.masterKey.Key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// decryptWithMasterKey decrypts data with the master key
func (km *KeyManager) decryptWithMasterKey(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(km.masterKey.Key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// GetActiveKey returns the active data encryption key
func (km *KeyManager) GetActiveKey() ([]byte, string, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()

	if km.activeKeyID == "" {
		return nil, "", fmt.Errorf("no active key available")
	}

	dataKey, exists := km.dataKeys[km.activeKeyID]
	if !exists {
		return nil, "", fmt.Errorf("active key not found")
	}

	// Decrypt data key with master key
	key, err := km.decryptWithMasterKey(dataKey.Key)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decrypt data key: %w", err)
	}

	return key, dataKey.ID, nil
}

// GetKey returns a specific data encryption key by ID
func (km *KeyManager) GetKey(keyID string) ([]byte, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()

	dataKey, exists := km.dataKeys[keyID]
	if !exists {
		return nil, fmt.Errorf("key not found: %s", keyID)
	}

	// Decrypt data key with master key
	key, err := km.decryptWithMasterKey(dataKey.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data key: %w", err)
	}

	return key, nil
}

// startKeyRotation starts automatic key rotation
func (km *KeyManager) startKeyRotation() {
	ticker := time.NewTicker(km.config.KeyRotationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := km.rotateDataKey(); err != nil {
				// Log error but continue
				continue
			}
		case <-km.keyRotationStop:
			return
		}
	}
}

// Close stops key rotation
func (km *KeyManager) Close() {
	if km.keyRotationStop != nil {
		close(km.keyRotationStop)
	}
}

// EncryptionService provides encryption/decryption services
type EncryptionService struct {
	keyManager *KeyManager
	config     *EncryptionConfig
}

// NewEncryptionService creates a new encryption service
func NewEncryptionService(config *EncryptionConfig) (*EncryptionService, error) {
	keyManager, err := NewKeyManager(config)
	if err != nil {
		return nil, err
	}

	return &EncryptionService{
		keyManager: keyManager,
		config:     config,
	}, nil
}

// Encrypt encrypts data
func (es *EncryptionService) Encrypt(plaintext []byte) (*EncryptedData, error) {
	if !es.config.Enabled {
		return &EncryptedData{
			Algorithm: string(EncryptionAlgorithmAES256GCM),
			Data:      plaintext,
			CreatedAt: time.Now(),
		}, nil
	}

	// Get active encryption key
	key, keyID, err := es.keyManager.GetActiveKey()
	if err != nil {
		return nil, err
	}

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Encrypt
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	return &EncryptedData{
		Algorithm: string(EncryptionAlgorithmAES256GCM),
		KeyID:     keyID,
		Nonce:     nonce,
		Data:      ciphertext,
		CreatedAt: time.Now(),
	}, nil
}

// Decrypt decrypts data
func (es *EncryptionService) Decrypt(encrypted *EncryptedData) ([]byte, error) {
	if !es.config.Enabled || encrypted.KeyID == "" {
		return encrypted.Data, nil
	}

	// Get decryption key
	key, err := es.keyManager.GetKey(encrypted.KeyID)
	if err != nil {
		return nil, err
	}

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Decrypt
	plaintext, err := gcm.Open(nil, encrypted.Nonce, encrypted.Data, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// EncryptToBase64 encrypts data and returns base64-encoded string
func (es *EncryptionService) EncryptToBase64(plaintext []byte) (string, error) {
	encrypted, err := es.Encrypt(plaintext)
	if err != nil {
		return "", err
	}

	data, err := json.Marshal(encrypted)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

// DecryptFromBase64 decrypts data from base64-encoded string
func (es *EncryptionService) DecryptFromBase64(encoded string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}

	var encrypted EncryptedData
	if err := json.Unmarshal(data, &encrypted); err != nil {
		return nil, err
	}

	return es.Decrypt(&encrypted)
}

// Close closes the encryption service
func (es *EncryptionService) Close() {
	es.keyManager.Close()
}

// generateKeyID generates a unique key ID
func generateKeyID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("key-%x", b)
}

// DeriveKeyFromPassword derives an encryption key from a password
func DeriveKeyFromPassword(password string, salt []byte, iterations int) []byte {
	if iterations == 0 {
		iterations = 100000
	}
	return pbkdf2.Key([]byte(password), salt, iterations, 32, sha256.New)
}

// GenerateSalt generates a random salt
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	return salt, nil
}
