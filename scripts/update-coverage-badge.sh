#!/bin/bash
set -e

# Script to automatically update coverage badge and documentation
# Usage: ./scripts/update-coverage-badge.sh

echo "🧪 Running tests and calculating coverage..."

# Run tests and generate coverage for core library (pkg/ excluding broker)
go test -short -coverprofile=coverage-core.out -covermode=atomic \
  $(go list ./pkg/... | grep -v '/pkg/broker') 2>&1 | tee test-output.txt

# Extract coverage percentage
COVERAGE=$(go tool cover -func=coverage-core.out | tail -1 | awk '{print $3}' | sed 's/%//')

# Round to 1 decimal place
COVERAGE_ROUNDED=$(printf "%.1f" "$COVERAGE")

echo ""
echo "📊 Core Library Coverage: ${COVERAGE_ROUNDED}%"
echo ""

# Determine badge color
if (( $(echo "$COVERAGE_ROUNDED >= 85" | bc -l) )); then
    COLOR="brightgreen"
elif (( $(echo "$COVERAGE_ROUNDED >= 75" | bc -l) )); then
    COLOR="green"
elif (( $(echo "$COVERAGE_ROUNDED >= 60" | bc -l) )); then
    COLOR="yellow"
else
    COLOR="red"
fi

echo "🎨 Badge Color: $COLOR"

# Coverage level at which the badge turns brightgreen. This is a badge-colour
# threshold only - it is deliberately not printed anywhere as a "target", since
# publishing a coverage goal next to the number invites treating the goal as
# the point rather than the coverage itself.
TARGET="85"

# URL encode the badge text
BADGE_TEXT="${COVERAGE_ROUNDED}%25"

# Update README.md badge
echo "📝 Updating README.md..."
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS
    sed -i '' "s|Coverage-[0-9.]*%25\(%20(target%20[0-9]*%25)\)\{0,1\}-[a-z]*|Coverage-${BADGE_TEXT}-${COLOR}|g" README.md
else
    # Linux
    sed -i "s|Coverage-[0-9.]*%25\(%20(target%20[0-9]*%25)\)\{0,1\}-[a-z]*|Coverage-${BADGE_TEXT}-${COLOR}|g" README.md
fi

# Update docs/TESTING.md - Current Status line
echo "📝 Updating docs/TESTING.md..."
if [[ "$OSTYPE" == "darwin"* ]]; then
    sed -i '' "s|\**Current Status\*\*: [0-9.]*% core library coverage|\*\*Current Status\*\*: ${COVERAGE_ROUNDED}% core library coverage|g" docs/TESTING.md
    sed -i '' "s|ships with \*\*[0-9.]*% core library coverage|\ships with \*\*${COVERAGE_ROUNDED}% core library coverage|g" docs/TESTING.md
    sed -i '' "s|\*\*[0-9.]*% current coverage\*\* (core library)|\*\*${COVERAGE_ROUNDED}% current coverage\*\* (core library)|g" docs/TESTING.md
    sed -i '' "s|solid test coverage ([0-9.]*% core library)|solid test coverage (${COVERAGE_ROUNDED}% core library)|g" docs/TESTING.md
else
    sed -i "s|\**Current Status\*\*: [0-9.]*% core library coverage|\*\*Current Status\*\*: ${COVERAGE_ROUNDED}% core library coverage|g" docs/TESTING.md
    sed -i "s|ships with \*\*[0-9.]*% core library coverage|\ships with \*\*${COVERAGE_ROUNDED}% core library coverage|g" docs/TESTING.md
    sed -i "s|\*\*[0-9.]*% current coverage\*\* (core library)|\*\*${COVERAGE_ROUNDED}% current coverage\*\* (core library)|g" docs/TESTING.md
    sed -i "s|solid test coverage ([0-9.]*% core library)|solid test coverage (${COVERAGE_ROUNDED}% core library)|g" docs/TESTING.md
fi


echo ""
echo "✅ Coverage badge and documentation updated!"
echo ""
echo "📋 Summary:"
echo "   Coverage: ${COVERAGE_ROUNDED}%"
echo "   Color:    ${COLOR}"
echo ""
echo "📁 Updated files:"
echo "   - README.md"
echo "   - docs/TESTING.md"
echo ""

# Check if running in CI
if [ -n "$CI" ]; then
    echo "🤖 Running in CI - setting output variables"
    echo "coverage=${COVERAGE_ROUNDED}" >> $GITHUB_OUTPUT
    echo "color=${COLOR}" >> $GITHUB_OUTPUT
fi

# Optional: Generate coverage report
if [ "$1" == "--report" ]; then
    echo "📊 Generating coverage report..."
    go tool cover -html=coverage-core.out -o coverage.html
    echo "✅ Coverage report saved to coverage.html"
fi

echo ""
echo "💡 Next steps:"
echo "   1. Review the changes: git diff"
echo "   2. Commit the changes: git add README.md docs/"
echo "   3. Push to GitHub: git push"
