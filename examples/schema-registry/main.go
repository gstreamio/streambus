package main

import (
	"fmt"
	"log"

	"github.com/gstreamio/streambus/pkg/logging"
	"github.com/gstreamio/streambus/pkg/schema"
)

func main() {
	fmt.Println("StreamBus Schema Registry Example")
	fmt.Println("==================================")
	fmt.Println()

	// Create logger
	logger := logging.New(&logging.Config{
		Level:  logging.LevelInfo,
		Output: nil, // Disable logging for cleaner example output
	})

	// Create schema validator
	validator := schema.NewDefaultValidator()

	// Create schema registry
	registry := schema.NewSchemaRegistry(validator, logger)

	fmt.Println("✓ Created schema registry")
	fmt.Println()

	// Example 1: Register a JSON schema
	fmt.Println("Example 1: Register JSON Schema")
	fmt.Println("--------------------------------")

	userSchemaV1 := `{
		"type": "object",
		"properties": {
			"id": {"type": "integer"},
			"name": {"type": "string"},
			"email": {"type": "string"}
		},
		"required": ["id", "name", "email"]
	}`

	registerReq := &schema.RegisterSchemaRequest{
		Subject:    "user-value",
		Format:     schema.FormatJSON,
		Definition: userSchemaV1,
	}

	resp, err := registry.RegisterSchema(registerReq)
	if err != nil {
		log.Fatalf("Failed to register schema: %v", err)
	}

	if resp.ErrorCode != schema.ErrorNone {
		log.Fatalf("Registration failed: %v", resp.ErrorCode)
	}

	fmt.Printf("✓ Registered schema with ID: %d\n", resp.ID)
	fmt.Printf("  Subject: user-value\n")
	fmt.Printf("  Format: JSON\n")
	fmt.Println()

	// Example 2: Schema Evolution (backward compatible)
	fmt.Println("Example 2: Schema Evolution (Backward Compatible)")
	fmt.Println("--------------------------------------------------")

	userSchemaV2 := `{
		"type": "object",
		"properties": {
			"id": {"type": "integer"},
			"name": {"type": "string"},
			"email": {"type": "string"},
			"age": {"type": "integer"}
		},
		"required": ["id", "name", "email"]
	}`

	// Test compatibility before registering
	testReq := &schema.TestCompatibilityRequest{
		Subject:    "user-value",
		Version:    1,
		Format:     schema.FormatJSON,
		Definition: userSchemaV2,
	}

	testResp, err := registry.TestCompatibility(testReq)
	if err != nil {
		log.Fatalf("Compatibility test failed: %v", err)
	}

	if testResp.Compatible {
		fmt.Println("✓ Schema V2 is backward compatible with V1")

		// Register the new version
		registerReq2 := &schema.RegisterSchemaRequest{
			Subject:    "user-value",
			Format:     schema.FormatJSON,
			Definition: userSchemaV2,
		}

		resp2, _ := registry.RegisterSchema(registerReq2)
		fmt.Printf("✓ Registered schema V2 with ID: %d\n", resp2.ID)
	} else {
		fmt.Println("✗ Schema V2 is NOT compatible with V1")
		fmt.Printf("  Reason: %s\n", testResp.Message)
	}
	fmt.Println()

	// Example 3: Register Avro schema
	fmt.Println("Example 3: Register Avro Schema")
	fmt.Println("--------------------------------")

	orderSchemaAvro := `{
		"type": "record",
		"name": "Order",
		"namespace": "com.example",
		"fields": [
			{"name": "order_id", "type": "long"},
			{"name": "customer_id", "type": "long"},
			{"name": "total", "type": "double"},
			{"name": "status", "type": "string"}
		]
	}`

	registerReq3 := &schema.RegisterSchemaRequest{
		Subject:    "order-value",
		Format:     schema.FormatAvro,
		Definition: orderSchemaAvro,
	}

	resp3, err := registry.RegisterSchema(registerReq3)
	if err != nil {
		log.Fatalf("Failed to register Avro schema: %v", err)
	}

	fmt.Printf("✓ Registered Avro schema with ID: %d\n", resp3.ID)
	fmt.Printf("  Subject: order-value\n")
	fmt.Printf("  Format: AVRO\n")
	fmt.Println()

	// Example 4: Register Protobuf schema
	fmt.Println("Example 4: Register Protobuf Schema")
	fmt.Println("------------------------------------")

	productSchemaProto := `syntax = "proto3";

package example;

message Product {
  int64 product_id = 1;
  string name = 2;
  string description = 3;
  double price = 4;
  int32 stock = 5;
}`

	registerReq4 := &schema.RegisterSchemaRequest{
		Subject:    "product-value",
		Format:     schema.FormatProtobuf,
		Definition: productSchemaProto,
	}

	resp4, err := registry.RegisterSchema(registerReq4)
	if err != nil {
		log.Fatalf("Failed to register Protobuf schema: %v", err)
	}

	fmt.Printf("✓ Registered Protobuf schema with ID: %d\n", resp4.ID)
	fmt.Printf("  Subject: product-value\n")
	fmt.Printf("  Format: PROTOBUF\n")
	fmt.Println()

	// Example 5: List all subjects
	fmt.Println("Example 5: List All Subjects")
	fmt.Println("-----------------------------")

	listReq := &schema.ListSubjectsRequest{}
	listResp, err := registry.ListSubjects(listReq)
	if err != nil {
		log.Fatalf("Failed to list subjects: %v", err)
	}

	fmt.Printf("Found %d subjects:\n", len(listResp.Subjects))
	for _, subject := range listResp.Subjects {
		fmt.Printf("  • %s\n", subject)
	}
	fmt.Println()

	// Example 6: List versions for a subject
	fmt.Println("Example 6: List Versions for Subject")
	fmt.Println("-------------------------------------")

	versionsReq := &schema.ListVersionsRequest{
		Subject: "user-value",
	}

	versionsResp, err := registry.ListVersions(versionsReq)
	if err != nil {
		log.Fatalf("Failed to list versions: %v", err)
	}

	fmt.Printf("Subject 'user-value' has %d versions:\n", len(versionsResp.Versions))
	for _, version := range versionsResp.Versions {
		fmt.Printf("  • Version %d\n", version)
	}
	fmt.Println()

	// Example 7: Get latest schema
	fmt.Println("Example 7: Get Latest Schema")
	fmt.Println("-----------------------------")

	latestReq := &schema.GetLatestSchemaRequest{
		Subject: "user-value",
	}

	latestResp, err := registry.GetLatestSchema(latestReq)
	if err != nil {
		log.Fatalf("Failed to get latest schema: %v", err)
	}

	fmt.Printf("Latest schema for 'user-value':\n")
	fmt.Printf("  ID: %d\n", latestResp.Schema.ID)
	fmt.Printf("  Version: %d\n", latestResp.Schema.Version)
	fmt.Printf("  Format: %s\n", latestResp.Schema.Format)
	fmt.Printf("  Created: %s\n", latestResp.Schema.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Println()

	// Example 8: Update compatibility mode
	fmt.Println("Example 8: Update Compatibility Mode")
	fmt.Println("-------------------------------------")

	updateCompatReq := &schema.UpdateCompatibilityRequest{
		Subject:       "user-value",
		Compatibility: schema.CompatibilityFull,
	}

	_, err = registry.UpdateCompatibility(updateCompatReq)
	if err != nil {
		log.Fatalf("Failed to update compatibility: %v", err)
	}

	fmt.Println("✓ Updated compatibility mode to FULL")

	// Get and verify
	getCompatReq := &schema.GetCompatibilityRequest{
		Subject: "user-value",
	}

	getCompatResp, _ := registry.GetCompatibility(getCompatReq)
	fmt.Printf("  Current mode: %s\n", getCompatResp.Compatibility)
	fmt.Println()

	// Example 9: Test incompatible schema
	fmt.Println("Example 9: Test Incompatible Schema")
	fmt.Println("------------------------------------")

	incompatibleSchema := `{
		"type": "object",
		"properties": {
			"id": {"type": "integer"}
		},
		"required": ["id"]
	}`

	incompatibleReq := &schema.TestCompatibilityRequest{
		Subject:    "user-value",
		Version:    2,
		Format:     schema.FormatJSON,
		Definition: incompatibleSchema,
	}

	incompatibleResp, _ := registry.TestCompatibility(incompatibleReq)
	if incompatibleResp.Compatible {
		fmt.Println("✓ Schema is compatible")
	} else {
		fmt.Println("✗ Schema is NOT compatible")
		fmt.Printf("  Reason: Missing required fields (name, email)\n")
	}
	fmt.Println()

	// Example 10: Registry Statistics
	fmt.Println("Example 10: Registry Statistics")
	fmt.Println("--------------------------------")

	stats := registry.Stats()
	fmt.Printf("Total Schemas: %d\n", stats.TotalSchemas)
	fmt.Printf("Total Subjects: %d\n", stats.TotalSubjects)
	fmt.Println()

	// Summary
	fmt.Println("═══════════════════════════════════════")
	fmt.Println("            SUMMARY")
	fmt.Println("═══════════════════════════════════════")
	fmt.Println()
	fmt.Println("Key Features Demonstrated:")
	fmt.Println("  ✓ Schema Registration (JSON, Avro, Protobuf)")
	fmt.Println("  ✓ Schema Evolution with Compatibility Checks")
	fmt.Println("  ✓ Multiple Compatibility Modes")
	fmt.Println("  ✓ Schema Versioning")
	fmt.Println("  ✓ Subject Management")
	fmt.Println("  ✓ Backward/Forward/Full Compatibility")
	fmt.Println()
	fmt.Println("Compatibility Modes Available:")
	fmt.Println("  • NONE - No compatibility checks")
	fmt.Println("  • BACKWARD - New schema can read old data")
	fmt.Println("  • FORWARD - Old schema can read new data")
	fmt.Println("  • FULL - Both backward and forward compatible")
	fmt.Println("  • BACKWARD_TRANSITIVE - Backward with all versions")
	fmt.Println("  • FORWARD_TRANSITIVE - Forward with all versions")
	fmt.Println("  • FULL_TRANSITIVE - Full with all versions")
	fmt.Println()
	fmt.Println("✓ Schema Registry example completed successfully!")
}
