#!/bin/bash

# StreamBus Load Testing Script
# Simplifies running load tests with common scenarios

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Default values
BROKERS="${STREAMBUS_BROKERS:-localhost:9092}"
TOPIC="loadtest-$(date +%s)"
PRODUCERS=1
CONSUMERS=1
MESSAGE_SIZE=1024
RATE=1000
DURATION="60s"
BATCH_SIZE=100
COMPRESSION="none"
REPORT_INTERVAL="10s"

print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

usage() {
    cat <<EOF
Usage: $0 [OPTIONS] [SCENARIO]

Run load tests against StreamBus broker.

SCENARIO:
    throughput       High-throughput test (default)
    latency          Low-latency test
    mixed            Mixed producer/consumer workload
    stress           Stress test with many connections
    sustained        Long-running sustained load
    burst            Burst traffic pattern

OPTIONS:
    -h, --help              Show this help message
    -b, --brokers ADDRS     Broker addresses (default: localhost:9092)
    -t, --topic TOPIC       Topic name (default: auto-generated)
    -p, --producers N       Number of producers (default: 1)
    -c, --consumers N       Number of consumers (default: 1)
    -s, --size BYTES        Message size in bytes (default: 1024)
    -r, --rate N            Target messages/sec (default: 1000)
    -d, --duration DUR      Test duration (default: 60s)
    --batch-size N          Batch size (default: 100)
    --compression TYPE      Compression: none, gzip, snappy, lz4
    --report-interval DUR   Stats reporting interval (default: 10s)

EXAMPLES:
    # Basic throughput test
    $0

    # High throughput scenario
    $0 throughput

    # Low latency with small batches
    $0 latency

    # Custom configuration
    $0 -p 10 -c 5 -r 50000 -d 5m

    # Stress test
    $0 stress

    # Different broker
    $0 -b broker.example.com:9092 throughput

EOF
}

# Parse arguments
SCENARIO="throughput"

while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            usage
            exit 0
            ;;
        -b|--brokers)
            BROKERS="$2"
            shift 2
            ;;
        -t|--topic)
            TOPIC="$2"
            shift 2
            ;;
        -p|--producers)
            PRODUCERS="$2"
            shift 2
            ;;
        -c|--consumers)
            CONSUMERS="$2"
            shift 2
            ;;
        -s|--size)
            MESSAGE_SIZE="$2"
            shift 2
            ;;
        -r|--rate)
            RATE="$2"
            shift 2
            ;;
        -d|--duration)
            DURATION="$2"
            shift 2
            ;;
        --batch-size)
            BATCH_SIZE="$2"
            shift 2
            ;;
        --compression)
            COMPRESSION="$2"
            shift 2
            ;;
        --report-interval)
            REPORT_INTERVAL="$2"
            shift 2
            ;;
        throughput|latency|mixed|stress|sustained|burst)
            SCENARIO="$1"
            shift
            ;;
        *)
            print_error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Apply scenario presets
case $SCENARIO in
    throughput)
        print_info "Scenario: High Throughput"
        PRODUCERS=10
        CONSUMERS=5
        MESSAGE_SIZE=1024
        RATE=100000
        BATCH_SIZE=1000
        COMPRESSION="lz4"
        ;;
    latency)
        print_info "Scenario: Low Latency"
        PRODUCERS=5
        CONSUMERS=5
        MESSAGE_SIZE=100
        RATE=10000
        BATCH_SIZE=10
        COMPRESSION="none"
        ;;
    mixed)
        print_info "Scenario: Mixed Workload"
        PRODUCERS=20
        CONSUMERS=20
        MESSAGE_SIZE=512
        RATE=50000
        BATCH_SIZE=100
        COMPRESSION="snappy"
        ;;
    stress)
        print_info "Scenario: Stress Test"
        PRODUCERS=50
        CONSUMERS=50
        MESSAGE_SIZE=1024
        RATE=200000
        DURATION="10m"
        BATCH_SIZE=500
        COMPRESSION="lz4"
        ;;
    sustained)
        print_info "Scenario: Sustained Load"
        PRODUCERS=5
        CONSUMERS=5
        MESSAGE_SIZE=1024
        RATE=10000
        DURATION="1h"
        BATCH_SIZE=100
        COMPRESSION="snappy"
        ;;
    burst)
        print_info "Scenario: Burst Pattern"
        PRODUCERS=20
        CONSUMERS=10
        MESSAGE_SIZE=2048
        RATE=50000
        DURATION="5m"
        BATCH_SIZE=200
        COMPRESSION="lz4"
        ;;
esac

print_info "StreamBus Load Test"
echo ""
print_info "Configuration:"
echo "  Brokers:          $BROKERS"
echo "  Topic:            $TOPIC"
echo "  Producers:        $PRODUCERS"
echo "  Consumers:        $CONSUMERS"
echo "  Message Size:     $MESSAGE_SIZE bytes"
echo "  Target Rate:      $RATE msgs/s"
echo "  Duration:         $DURATION"
echo "  Batch Size:       $BATCH_SIZE"
echo "  Compression:      $COMPRESSION"
echo ""

# Check if broker is reachable
BROKER_HOST=$(echo $BROKERS | cut -d: -f1)
BROKER_PORT=$(echo $BROKERS | cut -d: -f2)

print_info "Checking broker connectivity..."
if timeout 2 bash -c "echo > /dev/tcp/$BROKER_HOST/$BROKER_PORT" 2>/dev/null; then
    print_success "Broker is reachable"
else
    print_error "Cannot reach broker at $BROKERS"
    print_info "Make sure the broker is running:"
    print_info "  ./bin/streambus-broker --config config/broker.yaml"
    exit 1
fi

# Build load test tool if needed
if [ ! -f "./bin/loadtest" ]; then
    print_info "Building load test tool..."
    go build -o ./bin/loadtest ./tools/loadtest
    print_success "Load test tool built"
fi

# Run load test
print_info "Starting load test..."
echo ""

./bin/loadtest \
    -brokers "$BROKERS" \
    -topic "$TOPIC" \
    -producers $PRODUCERS \
    -consumers $CONSUMERS \
    -message-size $MESSAGE_SIZE \
    -rate $RATE \
    -duration $DURATION \
    -batch-size $BATCH_SIZE \
    -compression "$COMPRESSION" \
    -report-interval "$REPORT_INTERVAL"

EXIT_CODE=$?

echo ""
if [ $EXIT_CODE -eq 0 ]; then
    print_success "Load test completed successfully"
else
    print_error "Load test failed with exit code $EXIT_CODE"
fi

exit $EXIT_CODE
