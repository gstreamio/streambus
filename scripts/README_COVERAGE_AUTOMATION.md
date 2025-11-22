# Automated Coverage Badge System

This directory contains automation for keeping the test coverage badge and documentation up-to-date.

## Overview

The coverage badge automation:
1. **Calculates** actual test coverage for core library packages
2. **Updates** the badge in README.md with the correct percentage and color
3. **Updates** all documentation references to coverage
4. **Runs automatically** in CI/CD on every push to main
5. **Comments on PRs** with coverage information

## Quick Start

### Local Usage

```bash
# Update coverage badge locally
make update-coverage-badge

# Or run the script directly
./scripts/update-coverage-badge.sh

# Generate coverage report too
./scripts/update-coverage-badge.sh --report
```

### What It Does

The script will:
1. Run tests on core library packages (`pkg/` excluding `pkg/broker`)
2. Calculate coverage percentage
3. Determine badge color based on coverage:
   - **Green (brightgreen)**: ≥ 85%
   - **Green**: ≥ 75%
   - **Yellow**: ≥ 60%
   - **Red**: < 60%
4. Update the following files:
   - `README.md` - Coverage badge
   - `docs/README.md` - Coverage references
   - `docs/TESTING.md` - Current status and summary

### Example Output

```
🧪 Running tests and calculating coverage...
ok      github.com/gstreamio/streambus/pkg/client       coverage: 78.5%
ok      github.com/gstreamio/streambus/pkg/cluster      coverage: 80.4%
...

📊 Core Library Coverage: 81.0%
🎨 Badge Color: green
📝 Updating README.md...
📝 Updating docs/TESTING.md...
📝 Updating docs/README.md...

✅ Coverage badge and documentation updated!

📋 Summary:
   Coverage: 81.0%
   Target:   85%
   Color:    green

📁 Updated files:
   - README.md
   - docs/README.md
   - docs/TESTING.md
```

## GitHub Actions Automation

### Workflow: `coverage-badge.yml`

**Triggers:**
- Push to `main` branch (when `.go`, `go.mod`, or `go.sum` files change)
- Pull requests to `main`
- Manual dispatch

**What It Does:**

#### On Push to Main
1. Runs tests and calculates coverage
2. Updates badge and documentation
3. **Automatically commits** changes with message: `chore: update coverage badge to X.X% [skip ci]`
4. Pushes to main branch

#### On Pull Request
1. Runs tests and calculates coverage
2. **Comments on the PR** with coverage information
3. Shows whether coverage target is met
4. Does NOT commit (just informs)

### Example PR Comment

```markdown
## 🟢 Test Coverage Update

**Core Library Coverage:** 81.5%
**Target:** 85%

⚠️ Close to target

<details>
<summary>Coverage Details</summary>

This PR would update the coverage badge to **81.5%**.

To view the full coverage report, download the `coverage-report` artifact from this workflow run.

</details>
```

## Configuration

### Coverage Calculation

The script calculates coverage for **core library packages only**:

```bash
# Included: All pkg/ packages except broker
go test -short -coverprofile=coverage-core.out -covermode=atomic \
  $(go list ./pkg/... | grep -v '/pkg/broker')
```

**Why exclude broker?**
- `pkg/broker` is an application/integration layer
- It has integration tests that require a running broker
- Core library coverage is more representative of library quality

### Badge Colors

| Coverage | Color | Status |
|----------|-------|--------|
| ≥ 85% | `brightgreen` | ✅ Target met |
| 75-84% | `green` | 🟢 Good |
| 60-74% | `yellow` | 🟡 Needs improvement |
| < 60% | `red` | 🔴 Critical |

### Target Coverage

Current target: **85%**

To change the target, update:
- `scripts/update-coverage-badge.sh` - Line with `TARGET="85"`
- `.github/workflows/coverage-badge.yml` - References to 85%

## Files

### Scripts

- **`update-coverage-badge.sh`** - Main automation script
  - Calculates coverage
  - Updates badge and docs
  - Works on macOS and Linux
  - CI/CD compatible

### Workflows

- **`.github/workflows/coverage-badge.yml`** - GitHub Actions workflow
  - Auto-commits on main
  - Comments on PRs
  - Uploads coverage artifacts

### Documentation

- **`README.md`** - Coverage badge (line 9)
- **`docs/README.md`** - Coverage reference (line 114)
- **`docs/TESTING.md`** - Multiple references (lines 5, 681, 687, 694)

## Troubleshooting

### Script fails with "command not found: bc"

Install `bc` (basic calculator):
```bash
# macOS
brew install bc

# Ubuntu/Debian
sudo apt-get install bc

# Alpine
apk add bc
```

### Badge not updating on GitHub

1. **Check if workflow ran**: Go to Actions tab
2. **Check for errors**: View workflow logs
3. **Check permissions**: Workflow needs `contents: write` permission
4. **Clear cache**: Hard refresh (Cmd+Shift+R or Ctrl+Shift+R)

### Coverage seems wrong

```bash
# Check what's being tested
go list ./pkg/... | grep -v '/pkg/broker'

# Run coverage manually
go test -short -coverprofile=coverage.out -covermode=atomic \
  $(go list ./pkg/... | grep -v '/pkg/broker')

# View coverage
go tool cover -func=coverage.out | tail -1
```

### Workflow not triggering

The workflow only triggers when:
- `.go` files change
- `go.mod` or `go.sum` changes
- Manually dispatched

To manually trigger:
1. Go to Actions tab
2. Select "Update Coverage Badge"
3. Click "Run workflow"

## Best Practices

### When to Run Locally

Run `make update-coverage-badge` when:
- You've added significant tests
- You want to see current coverage before pushing
- You're preparing a release

### When to Commit Coverage Changes

The workflow auto-commits on main, but you can also:
- Include coverage updates in feature PRs
- Update coverage as part of test improvement efforts
- Commit manually after major refactoring

### Reviewing Coverage

```bash
# Generate HTML report
./scripts/update-coverage-badge.sh --report
open coverage.html

# Or use the existing coverage script
./scripts/test-coverage.sh --html
```

## Integration with CI/CD

### Required Permissions

The workflow needs:
```yaml
permissions:
  contents: write      # To commit changes
  pull-requests: write # To comment on PRs
```

### Preventing Infinite Loops

Commits include `[skip ci]` to prevent triggering the workflow again:
```
chore: update coverage badge to 81.0% [skip ci]
```

### Artifacts

Coverage reports are uploaded as artifacts:
- **Name**: `coverage-report`
- **Files**: `coverage-core.out`, `test-output.txt`
- **Retention**: 30 days

Download from: Actions → Workflow Run → Artifacts

## Future Enhancements

Potential improvements:
- [ ] Track coverage trends over time
- [ ] Generate coverage diff for PRs
- [ ] Fail PRs that decrease coverage
- [ ] Integration with Codecov or Coveralls
- [ ] Per-package coverage badges
- [ ] Coverage heatmap visualization

## Support

For issues or questions:
- Check workflow logs in Actions tab
- Review script output for errors
- Ensure all dependencies are installed
- Verify file permissions (`chmod +x scripts/*.sh`)

---

**Last Updated**: November 22, 2025
