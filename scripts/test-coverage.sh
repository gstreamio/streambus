#!/bin/bash

# StreamBus Test Coverage Analysis
# Generates coverage reports and identifies gaps

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
COVERAGE_DIR="coverage"
COVERAGE_FILE="$COVERAGE_DIR/coverage.out"
HTML_FILE="$COVERAGE_DIR/coverage.html"
TARGET_COVERAGE=85.0

mkdir -p "$COVERAGE_DIR"

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
Usage: $0 [OPTIONS]

Generate and analyze test coverage for StreamBus.

OPTIONS:
    -h, --help          Show this help message
    -p, --package PKG   Test specific package (default: all)
    -t, --threshold N   Coverage threshold percentage (default: 85.0)
    -f, --format FMT    Output format: text, html, json (default: text)
    --html              Generate HTML report
    --func              Show function-level coverage
    --short             Skip integration tests

EXAMPLES:
    # Run all tests with coverage
    $0

    # Generate HTML report
    $0 --html

    # Test specific package
    $0 -p ./pkg/storage

    # Show function-level coverage
    $0 --func

EOF
}

# Parse arguments
PACKAGE="./..."
FORMAT="text"
GENERATE_HTML=false
SHOW_FUNC=false
SHORT_MODE=""

while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            usage
            exit 0
            ;;
        -p|--package)
            PACKAGE="$2"
            shift 2
            ;;
        -t|--threshold)
            TARGET_COVERAGE="$2"
            shift 2
            ;;
        -f|--format)
            FORMAT="$2"
            shift 2
            ;;
        --html)
            GENERATE_HTML=true
            shift
            ;;
        --func)
            SHOW_FUNC=true
            shift
            ;;
        --short)
            SHORT_MODE="-short"
            shift
            ;;
        *)
            print_error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

print_info "StreamBus Test Coverage Analysis"
print_info "Package: $PACKAGE"
print_info "Target: ${TARGET_COVERAGE}%"
echo ""

# Run tests with coverage
print_info "Running tests with coverage..."
if go test $SHORT_MODE -coverprofile="$COVERAGE_FILE" -covermode=atomic $PACKAGE; then
    print_success "Tests passed"
else
    print_error "Tests failed"
    exit 1
fi

echo ""

# Calculate overall coverage
print_info "Analyzing coverage..."
COVERAGE=$(go tool cover -func="$COVERAGE_FILE" | grep total | awk '{print $3}' | sed 's/%//')

echo ""
print_info "=========================================="
print_info "Overall Coverage: ${COVERAGE}%"
print_info "Target Coverage:  ${TARGET_COVERAGE}%"
print_info "=========================================="
echo ""

# Check if target met
if (( $(echo "$COVERAGE >= $TARGET_COVERAGE" | bc -l) )); then
    print_success "Coverage target met!"
else
    DIFF=$(echo "$TARGET_COVERAGE - $COVERAGE" | bc)
    print_warning "Coverage below target by ${DIFF}%"
fi

# Show function-level coverage if requested
if [ "$SHOW_FUNC" = true ]; then
    echo ""
    print_info "Function-level coverage:"
    go tool cover -func="$COVERAGE_FILE" | sort -k3 -n | head -20
fi

# Generate HTML report if requested
if [ "$GENERATE_HTML" = true ]; then
    print_info "Generating HTML report..."
    go tool cover -html="$COVERAGE_FILE" -o "$HTML_FILE"
    print_success "HTML report: $HTML_FILE"

    # Try to open in browser
    if command -v open &> /dev/null; then
        open "$HTML_FILE"
    elif command -v xdg-open &> /dev/null; then
        xdg-open "$HTML_FILE"
    fi
fi

# Package-level coverage breakdown
echo ""
print_info "Package-level coverage:"
go tool cover -func="$COVERAGE_FILE" | awk -F: '{print $1}' | sort -u | while read -r file; do
    if [ ! -z "$file" ] && [ "$file" != "total" ]; then
        pkg=$(dirname "$file")
        if [ ! -z "$pkg" ]; then
            cov=$(go tool cover -func="$COVERAGE_FILE" | grep "^$pkg" | \
                  awk '{sum+=$3; count++} END {if(count>0) print sum/count; else print 0}')
            printf "  %-50s %6.1f%%\n" "$pkg" "$cov"
        fi
    fi
done | sort -k2 -n | tail -20

# Identify low-coverage files
echo ""
print_info "Files with lowest coverage (< 70%):"
go tool cover -func="$COVERAGE_FILE" | \
    awk '{if ($3 != "total:" && substr($3, 1, length($3)-1) < 70) print $1, $3}' | \
    sort -k2 -n | head -10

# Summary statistics
echo ""
print_info "Coverage Statistics:"
TOTAL_LINES=$(go tool cover -func="$COVERAGE_FILE" | grep total | awk '{print $2}')
COVERED_LINES=$(echo "$TOTAL_LINES * $COVERAGE / 100" | bc)
UNCOVERED_LINES=$(echo "$TOTAL_LINES - $COVERED_LINES" | bc)

echo "  Total statements:     $TOTAL_LINES"
echo "  Covered statements:   $(printf "%.0f" $COVERED_LINES)"
echo "  Uncovered statements: $(printf "%.0f" $UNCOVERED_LINES)"

# JSON output
if [ "$FORMAT" = "json" ]; then
    cat > "$COVERAGE_DIR/coverage.json" <<EOF
{
  "overall_coverage": $COVERAGE,
  "target_coverage": $TARGET_COVERAGE,
  "total_statements": $TOTAL_LINES,
  "covered_statements": $(printf "%.0f" $COVERED_LINES),
  "uncovered_statements": $(printf "%.0f" $UNCOVERED_LINES),
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF
    print_success "JSON report: $COVERAGE_DIR/coverage.json"
fi

echo ""

# Exit with appropriate code
if (( $(echo "$COVERAGE >= $TARGET_COVERAGE" | bc -l) )); then
    print_success "Coverage analysis complete!"
    exit 0
else
    print_warning "Coverage below target"
    exit 1
fi
