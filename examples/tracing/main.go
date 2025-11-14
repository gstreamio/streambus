package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gstreamio/streambus/pkg/broker"
	"github.com/gstreamio/streambus/pkg/client"
	"github.com/gstreamio/streambus/pkg/tracing"
	"go.opentelemetry.io/otel/codes"
)

func main() {
	fmt.Println("StreamBus OpenTelemetry Tracing Example")
	fmt.Println("========================================")
	fmt.Println()

	// Create data directory
	dataDir := filepath.Join(os.TempDir(), "streambus-tracing-example")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}
	defer os.RemoveAll(dataDir)

	// Example 1: Configure OpenTelemetry Tracing
	fmt.Println("1. Configuring OpenTelemetry Tracing")

	tracingConfig := &tracing.Config{
		Enabled:        true,
		ServiceName:    "streambus-example",
		ServiceVersion: "1.0.0",
		Environment:    "development",
		Exporter: tracing.ExporterConfig{
			Type: tracing.ExporterTypeStdout, // Output to stdout for demo
		},
		Sampling: tracing.SamplingConfig{
			SamplingRate: 1.0,       // Sample 100% of traces
			ParentBased:  true,      // Respect parent sampling decisions
		},
		ResourceAttributes: map[string]string{
			"region": "us-west-2",
			"env":    "demo",
		},
		Propagators: []string{"tracecontext", "baggage"},
	}

	fmt.Printf("   - Service Name: %s\n", tracingConfig.ServiceName)
	fmt.Printf("   - Environment: %s\n", tracingConfig.Environment)
	fmt.Printf("   - Sampling Rate: %.0f%%\n", tracingConfig.Sampling.SamplingRate*100)
	fmt.Printf("   - Exporter: %s\n", tracingConfig.Exporter.Type)

	// Example 2: Initialize Tracer
	fmt.Println("\n2. Initializing Tracer")

	tracer, err := tracing.New(tracingConfig)
	if err != nil {
		log.Fatalf("Failed to create tracer: %v", err)
	}
	defer tracer.Shutdown(context.Background())

	fmt.Println("   - Tracer initialized successfully")
	fmt.Printf("   - Tracing enabled: %v\n", tracer.IsEnabled())

	// Example 3: Create and start broker with tracing
	fmt.Println("\n3. Creating Broker")

	brokerConfig := &broker.Config{
		BrokerID:    1,
		Host:        "localhost",
		Port:        9092,
		DataDir:     filepath.Join(dataDir, "broker-data"),
		RaftDataDir: filepath.Join(dataDir, "raft-data"),
		LogLevel:    "info",
	}

	b, err := broker.New(brokerConfig)
	if err != nil {
		log.Fatalf("Failed to create broker: %v", err)
	}

	if err := b.Start(); err != nil {
		log.Fatalf("Failed to start broker: %v", err)
	}
	fmt.Println("   - Broker started successfully")

	// Example 4: Create traced operations
	fmt.Println("\n4. Creating Topic with Tracing")

	ctx := context.Background()

	// Start a span for topic creation
	ctx, createTopicSpan := tracer.StartWithAttributes(ctx, "create-topic", map[string]interface{}{
		"topic.name":       "traced-events",
		"topic.partitions": 3,
		"topic.replicas":   1,
	})

	// Create client
	clientConfig := client.DefaultConfig()
	clientConfig.Brokers = []string{"localhost:9092"}

	c, err := client.New(clientConfig)
	if err != nil {
		tracing.RecordError(createTopicSpan, err)
		log.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	// Create topic
	err = c.CreateTopic("traced-events", 3, 1)
	if err != nil {
		tracing.RecordError(createTopicSpan, err)
		log.Printf("Topic may already exist: %v", err)
	} else {
		tracing.SetSpanStatus(createTopicSpan, codes.Ok, "Topic created successfully")
		fmt.Println("   - Topic 'traced-events' created")
	}
	createTopicSpan.End()

	// Example 5: Traced message production
	fmt.Println("\n5. Producing Messages with Tracing")

	producer := client.NewProducer(c)
	defer producer.Close()

	messageCount := 10

	for i := 0; i < messageCount; i++ {
		// Start a span for each produce operation
		produceCtx, produceSpan := tracer.StartWithAttributes(ctx, "produce-message", map[string]interface{}{
			"topic.name":  "traced-events",
			"message.key": fmt.Sprintf("key-%d", i),
		})

		key := []byte(fmt.Sprintf("key-%d", i))
		value := []byte(fmt.Sprintf("Traced message %d at %s", i, time.Now().Format(time.RFC3339)))

		startTime := time.Now()

		err := producer.Send("traced-events", key, value)
		if err != nil {
			tracing.RecordError(produceSpan, err)
			log.Printf("Failed to send message: %v", err)
		} else {
			latency := time.Since(startTime)

			// Add success attributes
			tracing.SetSpanAttributes(produceSpan,
				tracing.AttrMessageSize.Int(len(value)),
			)

			// Add event for successful send
			tracing.AddSpanEvent(produceSpan, "message-sent",
				tracing.AttrMessageSize.Int(len(value)),
			)

			tracing.SetSpanStatus(produceSpan, codes.Ok, "Message sent")

			if (i+1)%5 == 0 {
				fmt.Printf("   - Produced %d messages (latency: %v)\n", i+1, latency)
			}
		}

		produceSpan.End()
		produceCtx.Done()

		time.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("   - Total messages produced: %d\n", messageCount)

	// Example 6: Simulated error with tracing
	fmt.Println("\n6. Demonstrating Error Tracing")

	errorCtx, errorSpan := tracer.Start(ctx, "simulate-error")

	// Simulate an error
	simulatedErr := fmt.Errorf("simulated validation error")
	tracing.RecordError(errorSpan, simulatedErr)

	// Add error attributes
	tracing.SetSpanAttributes(errorSpan,
		tracing.ErrorAttributes("validation_error", simulatedErr.Error(), 400)...,
	)

	fmt.Println("   - Simulated error recorded in trace")

	errorSpan.End()
	errorCtx.Done()

	// Example 7: Nested spans (parent-child relationships)
	fmt.Println("\n7. Demonstrating Nested Spans")

	parentCtx, parentSpan := tracer.Start(ctx, "batch-operation")

	fmt.Println("   - Parent span: batch-operation")

	for i := 0; i < 3; i++ {
		// Child span inherits parent's trace context
		childCtx, childSpan := tracer.StartWithAttributes(parentCtx, fmt.Sprintf("sub-operation-%d", i), map[string]interface{}{
			"message.count": i,
		})

		fmt.Printf("   - Child span %d: sub-operation-%d\n", i, i)

		// Simulate some work
		time.Sleep(50 * time.Millisecond)

		tracing.SetSpanStatus(childSpan, codes.Ok, "Sub-operation completed")
		childSpan.End()
		childCtx.Done()
	}

	tracing.SetSpanStatus(parentSpan, codes.Ok, "Batch operation completed")
	parentSpan.End()
	parentCtx.Done()

	// Example 8: Tracing with attributes
	fmt.Println("\n8. Adding Custom Attributes")

	attrCtx, attrSpan := tracer.Start(ctx, "operation-with-attributes")

	// Add various StreamBus-specific attributes
	tracing.SetSpanAttributes(attrSpan,
		tracing.BrokerAttributes(1, "localhost", 9092)...,
	)

	tracing.SetSpanAttributes(attrSpan,
		tracing.TopicAttributes("traced-events", 3, 1)...,
	)

	tracing.SetSpanAttributes(attrSpan,
		tracing.PartitionAttributes(0, 100)...,
	)

	fmt.Println("   - Added broker, topic, and partition attributes")

	attrSpan.End()
	attrCtx.Done()

	// Example 9: Span events for key milestones
	fmt.Println("\n9. Adding Span Events")

	eventCtx, eventSpan := tracer.Start(ctx, "complex-operation")

	tracing.AddSpanEvent(eventSpan, "started", tracing.AttrMessageCount.Int(0))
	fmt.Println("   - Event: started")

	time.Sleep(50 * time.Millisecond)

	tracing.AddSpanEvent(eventSpan, "checkpoint-1", tracing.AttrMessageCount.Int(5))
	fmt.Println("   - Event: checkpoint-1")

	time.Sleep(50 * time.Millisecond)

	tracing.AddSpanEvent(eventSpan, "completed", tracing.AttrMessageCount.Int(10))
	fmt.Println("   - Event: completed")

	eventSpan.End()
	eventCtx.Done()

	// Example 10: Summary
	fmt.Println("\n10. Tracing Summary")
	fmt.Println("   -----------------------------")
	fmt.Println("   Traces Generated:")
	fmt.Println("   - Topic creation span")
	fmt.Printf("   - %d produce message spans\n", messageCount)
	fmt.Println("   - Error simulation span")
	fmt.Println("   - Batch operation with 3 child spans")
	fmt.Println("   - Operations with custom attributes")
	fmt.Println("   - Operations with events")
	fmt.Println("   -----------------------------")
	fmt.Println()
	fmt.Println("   Integration Points:")
	fmt.Println("   - OTLP Collector: Configure endpoint in config")
	fmt.Println("   - Jaeger: Use tracing.ExporterTypeJaeger")
	fmt.Println("   - Zipkin: Use tracing.ExporterTypeZipkin")
	fmt.Println("   - Stdout: Use tracing.ExporterTypeStdout (demo)")
	fmt.Println()
	fmt.Println("   Trace Context Propagation:")
	fmt.Println("   - W3C Trace Context (tracecontext)")
	fmt.Println("   - W3C Baggage (baggage)")
	fmt.Println("   - Automatic context propagation in nested spans")

	// Wait a bit for traces to flush
	fmt.Println("\n11. Press Ctrl+C to shutdown...")
	time.Sleep(2 * time.Second)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	// Cleanup
	fmt.Println("\n12. Shutting Down")
	fmt.Println("   - Flushing traces...")

	// Shutdown tracer (flushes remaining spans)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tracer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error shutting down tracer: %v", err)
	}

	fmt.Println("   - Stopping broker...")
	if err := b.Stop(); err != nil {
		log.Printf("Error stopping broker: %v", err)
	}

	fmt.Println("   - Broker stopped successfully")
	fmt.Println("\nTracing Example Completed!")
	fmt.Println("\nKey Takeaways:")
	fmt.Println("1. OpenTelemetry provides standardized distributed tracing")
	fmt.Println("2. Traces show request flow and performance bottlenecks")
	fmt.Println("3. Parent-child span relationships capture operation hierarchy")
	fmt.Println("4. Attributes provide rich context for debugging")
	fmt.Println("5. Events mark important milestones within spans")
	fmt.Println("6. Error recording helps identify failures in traces")
	fmt.Println("7. Multiple exporter options for different backends")
}
