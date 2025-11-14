package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gstreamio/streambus/pkg/broker"
	"github.com/gstreamio/streambus/pkg/logging"
	"github.com/gstreamio/streambus/pkg/security"
)

func main() {
	fmt.Println("StreamBus Security Example")
	fmt.Println("===========================")

	// Create data directory
	dataDir := filepath.Join(os.TempDir(), "streambus-security-example")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}
	defer os.RemoveAll(dataDir)

	// Example 1: Configure TLS/mTLS
	fmt.Println("\n1. TLS/mTLS Configuration:")
	tlsConfig := &security.TLSConfig{
		Enabled:           true,
		RequireClientCert: false, // Set to true for mTLS
		VerifyClientCert:  false,
		MinVersion:        tls.VersionTLS12,
		CipherSuites:      security.GetRecommendedCipherSuites(),
	}
	fmt.Printf("   - TLS Enabled: %v\n", tlsConfig.Enabled)
	fmt.Printf("   - Minimum Version: TLS 1.2\n")
	fmt.Printf("   - Cipher Suites: %d configured\n", len(tlsConfig.CipherSuites))

	// Example 2: Configure SASL Authentication
	fmt.Println("\n2. SASL Authentication Configuration:")
	saslConfig := &security.SASLConfig{
		Enabled:    true,
		Mechanisms: []security.AuthMethod{security.AuthMethodSASLPlain, security.AuthMethodSASLSCRAM256},
		Users:      make(map[string]*security.User),
	}

	// Create users
	users := []struct {
		username string
		password string
		method   security.AuthMethod
		groups   []string
	}{
		{"admin", "admin-password", security.AuthMethodSASLPlain, []string{"admins", "users"}},
		{"producer", "producer-password", security.AuthMethodSASLSCRAM256, []string{"producers"}},
		{"consumer", "consumer-password", security.AuthMethodSASLSCRAM256, []string{"consumers"}},
	}

	for _, u := range users {
		user, err := security.CreateUser(u.username, u.password, u.method, u.groups)
		if err != nil {
			log.Fatalf("Failed to create user %s: %v", u.username, err)
		}
		saslConfig.Users[u.username] = user
		fmt.Printf("   - Created user: %s (method: %s, groups: %v)\n", u.username, u.method, u.groups)
	}

	// Example 3: Configure ACL Authorization
	fmt.Println("\n3. ACL Authorization Configuration:")
	securityConfig := &security.SecurityConfig{
		TLS:            tlsConfig,
		SASL:           saslConfig,
		AuthzEnabled:   true,
		SuperUsers:     []string{"admin"},
		AllowAnonymous: false,
		AuditEnabled:   true,
		AuditLogFile:   filepath.Join(dataDir, "audit.log"),
		UseDefaultACLs: false, // We'll add custom ACLs
	}

	// Create logger
	logger := logging.New(&logging.Config{
		Level:        logging.LevelInfo,
		Output:       os.Stdout,
		Component:    "security-example",
		IncludeTrace: false,
		IncludeFile:  true,
	})

	// Create security manager
	secManager, err := security.NewManager(securityConfig, logger)
	if err != nil {
		log.Fatalf("Failed to create security manager: %v", err)
	}
	defer secManager.Close()

	fmt.Printf("   - Authorization: %v\n", secManager.IsAuthorizationEnabled())
	fmt.Printf("   - Super Users: %v\n", securityConfig.SuperUsers)

	// Example 4: Add ACL Rules
	fmt.Println("\n4. Adding ACL Rules:")

	acls := []*security.ACLEntry{
		// Allow producers to write to topics starting with "events."
		security.NewACLBuilder().
			ForPrincipal("group:producers").
			OnResource(security.ResourceTypeTopic, "events.").
			WithPatternType(security.PatternTypePrefix).
			ForAction(security.ActionTopicWrite).
			Allow().
			CreatedBy("admin").
			Build(),

		// Allow consumers to read from topics starting with "events."
		security.NewACLBuilder().
			ForPrincipal("group:consumers").
			OnResource(security.ResourceTypeTopic, "events.").
			WithPatternType(security.PatternTypePrefix).
			ForAction(security.ActionTopicRead).
			Allow().
			CreatedBy("admin").
			Build(),

		// Deny everyone from deleting the "critical-data" topic
		security.NewACLBuilder().
			ForPrincipal("*").
			OnResource(security.ResourceTypeTopic, "critical-data").
			WithPatternType(security.PatternTypeLiteral).
			ForAction(security.ActionTopicDelete).
			Deny().
			CreatedBy("admin").
			Build(),

		// Allow all authenticated users to describe topics
		security.NewACLBuilder().
			ForPrincipal("*").
			OnResource(security.ResourceTypeTopic, "*").
			WithPatternType(security.PatternTypeWildcard).
			ForAction(security.ActionTopicDescribe).
			Allow().
			CreatedBy("system").
			Build(),
	}

	for _, acl := range acls {
		if err := secManager.AddACL(acl); err != nil {
			log.Fatalf("Failed to add ACL: %v", err)
		}
		fmt.Printf("   - Added ACL: %s %s on %s (%s pattern)\n",
			acl.Permission, acl.Action, acl.ResourceName, acl.PatternType)
	}

	// Example 5: Test Authentication
	fmt.Println("\n5. Testing Authentication:")

	ctx := context.Background()

	// Test SASL PLAIN authentication
	plainCreds := &security.SASLCredentials{
		Username:  "admin",
		Password:  "admin-password",
		Mechanism: security.AuthMethodSASLPlain,
	}

	principal, err := secManager.Authenticate(ctx, security.AuthMethodSASLPlain, plainCreds)
	if err != nil {
		log.Fatalf("Authentication failed: %v", err)
	}
	fmt.Printf("   - Authenticated user: %s (type: %s, method: %s)\n",
		principal.ID, principal.Type, principal.AuthMethod)

	// Test SCRAM authentication
	scramCreds := &security.SASLCredentials{
		Username:  "producer",
		Password:  "producer-password",
		Mechanism: security.AuthMethodSASLSCRAM256,
	}

	producerPrincipal, err := secManager.Authenticate(ctx, security.AuthMethodSASLSCRAM256, scramCreds)
	if err != nil {
		log.Fatalf("Producer authentication failed: %v", err)
	}
	fmt.Printf("   - Authenticated producer: %s (method: %s)\n",
		producerPrincipal.ID, producerPrincipal.AuthMethod)

	// Example 6: Test Authorization
	fmt.Println("\n6. Testing Authorization:")

	authTests := []struct {
		principal *security.Principal
		action    security.Action
		resource  security.Resource
	}{
		{
			producerPrincipal,
			security.ActionTopicWrite,
			security.Resource{Type: security.ResourceTypeTopic, Name: "events.orders"},
		},
		{
			producerPrincipal,
			security.ActionTopicWrite,
			security.Resource{Type: security.ResourceTypeTopic, Name: "system.internal"},
		},
		{
			principal, // admin (super user)
			security.ActionTopicDelete,
			security.Resource{Type: security.ResourceTypeTopic, Name: "critical-data"},
		},
	}

	for _, test := range authTests {
		allowed, err := secManager.Authorize(ctx, test.principal, test.action, test.resource)
		if err != nil {
			log.Printf("Authorization error: %v", err)
			continue
		}
		status := "DENIED"
		if allowed {
			status = "ALLOWED"
		}
		fmt.Printf("   - %s: %s %s on %s (%s)\n",
			status, test.principal.ID, test.action, test.resource.Name, test.resource.Type)
	}

	// Example 7: Encryption at Rest
	fmt.Println("\n7. Encryption at Rest:")

	encConfig := &security.EncryptionConfig{
		Enabled:             true,
		Algorithm:           security.EncryptionAlgorithmAES256GCM,
		MasterKeyPath:       filepath.Join(dataDir, "master.key"),
		KeyRotationInterval: 24 * time.Hour * 30, // 30 days
		PBKDF2Iterations:    100000,
	}

	encService, err := security.NewEncryptionService(encConfig)
	if err != nil {
		log.Fatalf("Failed to create encryption service: %v", err)
	}
	defer encService.Close()

	// Encrypt some data
	sensitiveData := []byte("This is sensitive message data that should be encrypted at rest")
	encrypted, err := encService.Encrypt(sensitiveData)
	if err != nil {
		log.Fatalf("Encryption failed: %v", err)
	}

	fmt.Printf("   - Original size: %d bytes\n", len(sensitiveData))
	fmt.Printf("   - Encrypted size: %d bytes\n", len(encrypted.Data))
	fmt.Printf("   - Key ID: %s\n", encrypted.KeyID)
	fmt.Printf("   - Algorithm: %s\n", encrypted.Algorithm)

	// Decrypt the data
	decrypted, err := encService.Decrypt(encrypted)
	if err != nil {
		log.Fatalf("Decryption failed: %v", err)
	}

	if string(decrypted) == string(sensitiveData) {
		fmt.Println("   - ✓ Decryption successful - data matches original")
	} else {
		fmt.Println("   - ✗ Decryption failed - data mismatch")
	}

	// Example 8: Audit Logging
	fmt.Println("\n8. Audit Logging:")
	fmt.Printf("   - Audit log file: %s\n", securityConfig.AuditLogFile)

	// Simulate some audited actions
	secManager.AuditAction(ctx, principal, security.ActionTopicCreate,
		security.Resource{Type: security.ResourceTypeTopic, Name: "new-topic"},
		map[string]string{"partitions": "3", "replication": "2"})

	secManager.AuditAction(ctx, producerPrincipal, security.ActionTopicWrite,
		security.Resource{Type: security.ResourceTypeTopic, Name: "events.orders"},
		map[string]string{"message_count": "100"})

	fmt.Println("   - Audit events logged successfully")

	// Example 9: Create a Secure Broker
	fmt.Println("\n9. Creating Secure Broker:")

	brokerConfig := &broker.Config{
		BrokerID:    1,
		Host:        "localhost",
		Port:        9092,
		DataDir:     filepath.Join(dataDir, "broker-data"),
		RaftDataDir: filepath.Join(dataDir, "raft-data"),
		Security:    securityConfig,
		LogLevel:    "info",
	}

	b, err := broker.New(brokerConfig)
	if err != nil {
		log.Fatalf("Failed to create broker: %v", err)
	}

	fmt.Println("   - Broker created successfully with security enabled")
	fmt.Println("   - Security features:")
	if b.SecurityManager() != nil {
		fmt.Printf("     * Authentication: %v\n", b.SecurityManager().IsAuthenticationEnabled())
		fmt.Printf("     * Authorization: %v\n", b.SecurityManager().IsAuthorizationEnabled())
		fmt.Printf("     * Audit Logging: %v\n", b.SecurityManager().IsAuditEnabled())
		fmt.Printf("     * Encryption: %v\n", b.SecurityManager().IsEncryptionEnabled())
	}

	// Start the broker
	fmt.Println("\n10. Starting Secure Broker...")
	if err := b.Start(); err != nil {
		log.Fatalf("Failed to start broker: %v", err)
	}

	fmt.Println("    - Broker started successfully")
	fmt.Println("    - Press Ctrl+C to stop...")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\n11. Stopping Broker...")
	if err := b.Stop(); err != nil {
		log.Fatalf("Failed to stop broker: %v", err)
	}

	fmt.Println("    - Broker stopped successfully")
	fmt.Println("\nSecurity Example Completed!")
}
