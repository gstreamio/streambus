#!/bin/bash

# StreamBus Benchmark Runner
# Automates running benchmarks, comparing results, and saving baselines

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
BENCHMARK_DIR="bench"
RESULTS_DIR="bench/results"
BASELINE_FILE="bench/results/baseline.txt"
CURRENT_FILE="bench/results/current.txt"
BENCHTIME="${BENCHTIME:-10s}"
BENCHMEM="${BENCHMEM:-true}"

# Create results directory
mkdir -p "$RESULTS_DIR"

# Print colored message
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

# Print usage
usage() {
    cat <<EOF
Usage: $0 [OPTIONS] [SUITE]

Run StreamBus benchmarks and optionally compare with baseline.

SUITE:
    all            Run all benchmark suites (default)
    throughput     Run throughput benchmarks only
    latency        Run latency benchmarks only
    concurrent     Run concurrent benchmarks only
    storage        Run storage benchmarks only

OPTIONS:
    -h, --help              Show this help message
    -s, --save-baseline     Save results as new baseline
    -c, --compare           Compare with baseline (default if baseline exists)
    -n, --no-compare        Don't compare with baseline
    -t, --time DURATION     Benchmark time (default: 10s)
    -m, --no-mem            Don't report memory allocations
    --short                 Skip integration tests (no broker required)
    --cpu-profile FILE      Generate CPU profile
    --mem-profile FILE      Generate memory profile
    --trace FILE            Generate execution trace

EXAMPLES:
    # Run all benchmarks
    $0

    # Run throughput benchmarks with 30s duration
    $0 -t 30s throughput

    # Run and save as new baseline
    $0 --save-baseline

    # Run with profiling
    $0 --cpu-profile=cpu.prof --mem-profile=mem.prof

    # Run without broker (unit tests only)
    $0 --short storage

EOF
}

# Parse arguments
SAVE_BASELINE=false
COMPARE=true
SUITE="all"
SHORT_MODE=""
CPU_PROFILE=""
MEM_PROFILE=""
TRACE_FILE=""

while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            usage
            exit 0
            ;;
        -s|--save-baseline)
            SAVE_BASELINE=true
            shift
            ;;
        -c|--compare)
            COMPARE=true
            shift
            ;;
        -n|--no-compare)
            COMPARE=false
            shift
            ;;
        -t|--time)
            BENCHTIME="$2"
            shift 2
            ;;
        -m|--no-mem)
            BENCHMEM="false"
            shift
            ;;
        --short)
            SHORT_MODE="-short"
            shift
            ;;
        --cpu-profile)
            CPU_PROFILE="-cpuprofile=$2"
            shift 2
            ;;
        --mem-profile)
            MEM_PROFILE="-memprofile=$2"
            shift 2
            ;;
        --trace)
            TRACE_FILE="-trace=$2"
            shift 2
            ;;
        all|throughput|latency|concurrent|storage)
            SUITE="$1"
            shift
            ;;
        *)
            print_error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Build benchmark flags
BENCH_FLAGS="-benchtime=$BENCHTIME"
if [ "$BENCHMEM" = "true" ]; then
    BENCH_FLAGS="$BENCH_FLAGS -benchmem"
fi

if [ -n "$SHORT_MODE" ]; then
    BENCH_FLAGS="$BENCH_FLAGS $SHORT_MODE"
fi

if [ -n "$CPU_PROFILE" ]; then
    BENCH_FLAGS="$BENCH_FLAGS $CPU_PROFILE"
fi

if [ -n "$MEM_PROFILE" ]; then
    BENCH_FLAGS="$BENCH_FLAGS $MEM_PROFILE"
fi

if [ -n "$TRACE_FILE" ]; then
    BENCH_FLAGS="$BENCH_FLAGS $TRACE_FILE"
fi

# Determine benchmark pattern based on suite
case $SUITE in
    all)
        BENCH_PATTERN="."
        ;;
    throughput)
        BENCH_PATTERN="Throughput"
        ;;
    latency)
        BENCH_PATTERN="Latency"
        ;;
    concurrent)
        BENCH_PATTERN="Concurrent"
        ;;
    storage)
        BENCH_PATTERN="Storage"
        ;;
esac

print_info "StreamBus Benchmark Runner"
print_info "Suite: $SUITE"
print_info "Flags: $BENCH_FLAGS"
print_info "Pattern: $BENCH_PATTERN"
echo ""

# Check if baseline exists for comparison
BASELINE_EXISTS=false
if [ -f "$BASELINE_FILE" ]; then
    BASELINE_EXISTS=true
    print_info "Baseline file found: $BASELINE_FILE"
fi

# Check if broker is running (unless in short mode)
if [ -z "$SHORT_MODE" ]; then
    print_info "Checking broker connectivity..."
    if ! timeout 2 bash -c "echo > /dev/tcp/localhost/9092" 2>/dev/null; then
        print_warning "Broker not reachable on localhost:9092"
        print_warning "Integration benchmarks will be skipped"
        print_info "To run integration benchmarks, start a broker:"
        print_info "  ./bin/streambus-broker --config config/broker.yaml"
        echo ""
        sleep 2
    else
        print_success "Broker is reachable"
    fi
fi

# Run benchmarks
print_info "Running benchmarks..."
echo ""

if go test -bench="$BENCH_PATTERN" $BENCH_FLAGS ./"$BENCHMARK_DIR" > "$CURRENT_FILE" 2>&1; then
    print_success "Benchmarks completed successfully"
else
    print_error "Benchmarks failed"
    cat "$CURRENT_FILE"
    exit 1
fi

echo ""
cat "$CURRENT_FILE"
echo ""

# Save baseline if requested
if [ "$SAVE_BASELINE" = "true" ]; then
    cp "$CURRENT_FILE" "$BASELINE_FILE"
    print_success "Results saved as baseline: $BASELINE_FILE"
fi

# Compare with baseline
if [ "$COMPARE" = "true" ] && [ "$BASELINE_EXISTS" = "true" ] && [ "$SAVE_BASELINE" = "false" ]; then
    print_info "Comparing with baseline..."
    echo ""

    # Check if benchstat is installed
    if command -v benchstat &> /dev/null; then
        benchstat "$BASELINE_FILE" "$CURRENT_FILE"
    else
        print_warning "benchstat not found. Install with:"
        print_info "  go install golang.org/x/perf/cmd/benchstat@latest"
        print_info ""
        print_info "Manual comparison:"
        echo "  Baseline: $BASELINE_FILE"
        echo "  Current:  $CURRENT_FILE"
    fi
fi

# Profile analysis hints
if [ -n "$CPU_PROFILE" ]; then
    echo ""
    print_info "CPU profile generated. Analyze with:"
    print_info "  go tool pprof ${CPU_PROFILE#-cpuprofile=}"
fi

if [ -n "$MEM_PROFILE" ]; then
    echo ""
    print_info "Memory profile generated. Analyze with:"
    print_info "  go tool pprof ${MEM_PROFILE#-memprofile=}"
fi

if [ -n "$TRACE_FILE" ]; then
    echo ""
    print_info "Execution trace generated. Analyze with:"
    print_info "  go tool trace ${TRACE_FILE#-trace=}"
fi

echo ""
print_success "Benchmark run complete!"

# Exit with success
exit 0
