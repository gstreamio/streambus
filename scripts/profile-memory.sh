#!/bin/bash

# StreamBus Memory Profiling Script
# Helps identify memory allocation hotspots and patterns

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
PROFILE_DIR="bench/profiles"
DURATION="${DURATION:-30s}"

mkdir -p "$PROFILE_DIR"

print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

usage() {
    cat <<EOF
Usage: $0 [OPTIONS] BENCHMARK

Profile memory allocations for a specific benchmark.

OPTIONS:
    -h, --help          Show this help message
    -d, --duration DUR  Benchmark duration (default: 30s)
    -o, --output FILE   Output profile file name
    -a, --analyze       Automatically analyze profile after collection
    --top N            Show top N allocation sites (default: 20)

BENCHMARK:
    Name of the benchmark to profile (e.g., BenchmarkE2E_ProducerThroughput)

EXAMPLES:
    # Profile producer throughput
    $0 BenchmarkE2E_ProducerThroughput

    # Profile for 60 seconds with analysis
    $0 -d 60s -a BenchmarkE2E_ProduceLatency

    # Custom output file
    $0 -o producer-mem.prof BenchmarkE2E_ProducerThroughput

EOF
}

# Parse arguments
ANALYZE=false
OUTPUT_FILE=""
TOP_N=20
BENCHMARK=""

while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            usage
            exit 0
            ;;
        -d|--duration)
            DURATION="$2"
            shift 2
            ;;
        -o|--output)
            OUTPUT_FILE="$2"
            shift 2
            ;;
        -a|--analyze)
            ANALYZE=true
            shift
            ;;
        --top)
            TOP_N="$2"
            shift 2
            ;;
        *)
            if [ -z "$BENCHMARK" ]; then
                BENCHMARK="$1"
                shift
            else
                print_error "Unknown option: $1"
                usage
                exit 1
            fi
            ;;
    esac
done

if [ -z "$BENCHMARK" ]; then
    print_error "Benchmark name required"
    usage
    exit 1
fi

# Generate output filename if not specified
if [ -z "$OUTPUT_FILE" ]; then
    TIMESTAMP=$(date +%Y%m%d-%H%M%S)
    OUTPUT_FILE="$PROFILE_DIR/mem-${BENCHMARK}-${TIMESTAMP}.prof"
fi

print_info "StreamBus Memory Profiler"
print_info "Benchmark: $BENCHMARK"
print_info "Duration: $DURATION"
print_info "Output: $OUTPUT_FILE"
echo ""

# Run benchmark with memory profiling
print_info "Running benchmark with memory profiling..."
if go test -bench="$BENCHMARK" -benchtime="$DURATION" -benchmem \
    -memprofile="$OUTPUT_FILE" \
    ./bench > "$OUTPUT_FILE.txt" 2>&1; then
    print_success "Profile collected: $OUTPUT_FILE"
else
    print_error "Benchmark failed"
    cat "$OUTPUT_FILE.txt"
    exit 1
fi

echo ""
print_info "Benchmark results:"
cat "$OUTPUT_FILE.txt"

# Analyze if requested
if [ "$ANALYZE" = "true" ]; then
    echo ""
    print_info "Analyzing memory profile..."
    echo ""

    print_info "=== Top $TOP_N Allocation Sites by Count ==="
    go tool pprof -text -top"$TOP_N" -nodefraction=0 -edgefraction=0 \
        -sample_index=alloc_objects "$OUTPUT_FILE" | head -n $((TOP_N + 5))

    echo ""
    print_info "=== Top $TOP_N Allocation Sites by Size ==="
    go tool pprof -text -top"$TOP_N" -nodefraction=0 -edgefraction=0 \
        -sample_index=alloc_space "$OUTPUT_FILE" | head -n $((TOP_N + 5))

    echo ""
    print_info "=== In-Use Memory ==="
    go tool pprof -text -top10 -nodefraction=0 -edgefraction=0 \
        -sample_index=inuse_space "$OUTPUT_FILE" | head -15

    echo ""
    print_info "Interactive analysis commands:"
    echo "  # Text-based top allocations"
    echo "  go tool pprof -text $OUTPUT_FILE"
    echo ""
    echo "  # Interactive CLI"
    echo "  go tool pprof $OUTPUT_FILE"
    echo ""
    echo "  # Web UI (requires graphviz)"
    echo "  go tool pprof -http=:8081 $OUTPUT_FILE"
    echo ""
    echo "  # Focus on specific package"
    echo "  go tool pprof -text -focus=storage $OUTPUT_FILE"
    echo ""
    echo "  # Show call graph"
    echo "  go tool pprof -text -call_tree $OUTPUT_FILE"
fi

print_success "Memory profiling complete!"

exit 0
