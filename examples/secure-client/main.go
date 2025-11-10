package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/shawntherrien/streambus/pkg/client"
)

func main() {
	// Example 1: Client with TLS (server authentication only)
	tlsClient := createTLSClient()
	defer tlsClient.Close()

	// Example 2: Client with mTLS (mutual authentication)
	mtlsClient := createMTLSClient()
	defer mtlsClient.Close()

	// Example 3: Client with TLS + SASL
	secureClient := createSecureClient()
	defer secureClient.Close()

	// Use the client
	if err := produceWithTLS(tlsClient); err != nil {
		log.Printf("Failed to produce with TLS: %v", err)
	}
}

// createTLSClient creates a client with TLS (server auth only)
func createTLSClient() *client.Client {
	config := client.DefaultConfig()
	config.Brokers = []string{"localhost:9092"}

	// Configure TLS
	config.Security = &client.SecurityConfig{
		TLS: &client.TLSConfig{
			Enabled:  true,
			CAFile:   "/path/to/ca.crt",
			ServerName: "localhost",
			// No client cert - server auth only
		},
	}

	c, err := client.New(config)
	if err != nil {
		log.Fatalf("Failed to create TLS client: %v", err)
	}

	return c
}

// createMTLSClient creates a client with mutual TLS authentication
func createMTLSClient() *client.Client {
	config := client.DefaultConfig()
	config.Brokers = []string{"localhost:9092"}

	// Configure mTLS
	config.Security = &client.SecurityConfig{
		TLS: &client.TLSConfig{
			Enabled:    true,
			CAFile:     "/path/to/ca.crt",
			CertFile:   "/path/to/client.crt",
			KeyFile:    "/path/to/client.key",
			ServerName: "localhost",
		},
	}

	c, err := client.New(config)
	if err != nil {
		log.Fatalf("Failed to create mTLS client: %v", err)
	}

	return c
}

// createSecureClient creates a client with both TLS and SASL
func createSecureClient() *client.Client {
	config := client.DefaultConfig()
	config.Brokers = []string{"localhost:9092"}

	// Configure TLS + SASL
	config.Security = &client.SecurityConfig{
		TLS: &client.TLSConfig{
			Enabled:    true,
			CAFile:     "/path/to/ca.crt",
			ServerName: "localhost",
		},
		SASL: &client.SASLConfig{
			Enabled:   true,
			Mechanism: "SCRAM-SHA-256",
			Username:  "producer1",
			Password:  "secure-password",
		},
	}

	c, err := client.New(config)
	if err != nil {
		log.Fatalf("Failed to create secure client: %v", err)
	}

	return c
}

// produceWithTLS demonstrates producing messages with a secure client
func produceWithTLS(c *client.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	producer, err := c.CreateProducer("secure-topic")
	if err != nil {
		return fmt.Errorf("failed to create producer: %w", err)
	}
	defer producer.Close()

	// Produce messages
	messages := []client.Message{
		{
			Key:   []byte("key1"),
			Value: []byte("Secure message 1"),
		},
		{
			Key:   []byte("key2"),
			Value: []byte("Secure message 2"),
		},
	}

	for _, msg := range messages {
		offset, err := producer.Send(ctx, msg)
		if err != nil {
			return fmt.Errorf("failed to send message: %w", err)
		}
		fmt.Printf("Message sent successfully at offset %d\n", offset)
	}

	return nil
}

// Example with insecure skip verify (development only!)
func createInsecureClient() *client.Client {
	config := client.DefaultConfig()
	config.Brokers = []string{"localhost:9092"}

	// CAUTION: This skips certificate verification - ONLY use for development!
	config.Security = &client.SecurityConfig{
		TLS: &client.TLSConfig{
			Enabled:            true,
			InsecureSkipVerify: true, // WARNING: Not safe for production!
		},
	}

	c, err := client.New(config)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	return c
}
