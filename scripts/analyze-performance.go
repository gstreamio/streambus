package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// PerformanceIssue represents a detected performance problem
type PerformanceIssue struct {
	Severity    string // "critical", "warning", "info"
	Category    string // "memory", "cpu", "allocation", "gc"
	Description string
	Impact      string
	Suggestion  string
}

func main() {
	profileFile := flag.String("profile", "", "Path to benchmark results file")
	outputFormat := flag.String("format", "text", "Output format: text, json, markdown")
	flag.Parse()

	if *profileFile == "" {
		fmt.Println("Usage: analyze-performance -profile <file>")
		os.Exit(1)
	}

	issues := analyzeProfile(*profileFile)

	switch *outputFormat {
	case "json":
		printJSON(issues)
	case "markdown":
		printMarkdown(issues)
	default:
		printText(issues)
	}

	// Exit code based on severity
	for _, issue := range issues {
		if issue.Severity == "critical" {
			os.Exit(1)
		}
	}
}

func analyzeProfile(filename string) []PerformanceIssue {
	file, err := os.Open(filename)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	var issues []PerformanceIssue
	scanner := bufio.NewScanner(file)

	// Parse benchmark results
	var (
		allocsPerOp      float64
		bytesPerOp       float64
		nsPerOp          float64
		benchmarkName    string
		hasThroughput    bool
		throughputMBps   float64
		messagesPerSec   float64
		hasLatency       bool
		p99Latency       float64
	)

	allocPattern := regexp.MustCompile(`(\d+)\s+allocs/op`)
	bytesPattern := regexp.MustCompile(`(\d+)\s+B/op`)
	nsPattern := regexp.MustCompile(`(\d+)\s+ns/op`)
	namePattern := regexp.MustCompile(`Benchmark(\w+)`)
	throughputPattern := regexp.MustCompile(`([\d.]+)\s+MB/s`)
	msgRatePattern := regexp.MustCompile(`([\d.]+)\s+msgs/s`)
	p99Pattern := regexp.MustCompile(`p99_[µm]s:([\d.]+)`)

	for scanner.Scan() {
		line := scanner.Text()

		// Extract benchmark name
		if matches := namePattern.FindStringSubmatch(line); len(matches) > 1 {
			benchmarkName = matches[1]
		}

		// Extract allocation metrics
		if matches := allocPattern.FindStringSubmatch(line); len(matches) > 1 {
			allocsPerOp, _ = strconv.ParseFloat(matches[1], 64)
		}

		if matches := bytesPattern.FindStringSubmatch(line); len(matches) > 1 {
			bytesPerOp, _ = strconv.ParseFloat(matches[1], 64)
		}

		if matches := nsPattern.FindStringSubmatch(line); len(matches) > 1 {
			nsPerOp, _ = strconv.ParseFloat(matches[1], 64)
		}

		// Extract throughput metrics
		if matches := throughputPattern.FindStringSubmatch(line); len(matches) > 1 {
			throughputMBps, _ = strconv.ParseFloat(matches[1], 64)
			hasThroughput = true
		}

		if matches := msgRatePattern.FindStringSubmatch(line); len(matches) > 1 {
			messagesPerSec, _ = strconv.ParseFloat(matches[1], 64)
		}

		// Extract latency metrics
		if matches := p99Pattern.FindStringSubmatch(line); len(matches) > 1 {
			p99Latency, _ = strconv.ParseFloat(matches[1], 64)
			hasLatency = true
		}

		// Analyze at end of each benchmark section
		if strings.HasPrefix(line, "Benchmark") && benchmarkName != "" {
			// Check for high allocation rates
			if allocsPerOp > 1000 {
				issues = append(issues, PerformanceIssue{
					Severity:    "critical",
					Category:    "allocation",
					Description: fmt.Sprintf("%s: Excessive allocations (%dallocs/op)", benchmarkName, int(allocsPerOp)),
					Impact:      "High GC pressure, reduced throughput",
					Suggestion:  "Review code for unnecessary allocations. Consider object pooling or reusing buffers.",
				})
			} else if allocsPerOp > 100 {
				issues = append(issues, PerformanceIssue{
					Severity:    "warning",
					Category:    "allocation",
					Description: fmt.Sprintf("%s: High allocations (%d allocs/op)", benchmarkName, int(allocsPerOp)),
					Impact:      "Moderate GC pressure",
					Suggestion:  "Look for opportunities to reduce allocations.",
				})
			}

			// Check for large memory allocations
			if bytesPerOp > 1024*1024 {
				issues = append(issues, PerformanceIssue{
					Severity:    "warning",
					Category:    "memory",
					Description: fmt.Sprintf("%s: Large memory allocations (%.1f MB/op)", benchmarkName, bytesPerOp/(1024*1024)),
					Impact:      "High memory usage",
					Suggestion:  "Consider streaming or chunking large data structures.",
				})
			}

			// Check throughput performance
			if hasThroughput {
				if throughputMBps < 10 {
					issues = append(issues, PerformanceIssue{
						Severity:    "critical",
						Category:    "throughput",
						Description: fmt.Sprintf("%s: Low throughput (%.1f MB/s)", benchmarkName, throughputMBps),
						Impact:      "Poor performance for production workloads",
						Suggestion:  "Profile for CPU hotspots. Check for blocking I/O or lock contention.",
					})
				} else if throughputMBps < 50 {
					issues = append(issues, PerformanceIssue{
						Severity:    "warning",
						Category:    "throughput",
						Description: fmt.Sprintf("%s: Moderate throughput (%.1f MB/s)", benchmarkName, throughputMBps),
						Impact:      "May not meet high-throughput requirements",
						Suggestion:  "Consider batching, buffering, or concurrent processing.",
					})
				}

				if messagesPerSec < 1000 {
					issues = append(issues, PerformanceIssue{
						Severity:    "warning",
						Category:    "throughput",
						Description: fmt.Sprintf("%s: Low message rate (%.0f msgs/s)", benchmarkName, messagesPerSec),
						Impact:      "Limited message processing capacity",
						Suggestion:  "Optimize hot paths and reduce per-message overhead.",
					})
				}
			}

			// Check latency performance
			if hasLatency {
				if p99Latency > 100 {
					issues = append(issues, PerformanceIssue{
						Severity:    "warning",
						Category:    "latency",
						Description: fmt.Sprintf("%s: High p99 latency (%.1fms)", benchmarkName, p99Latency),
						Impact:      "Poor tail latency for latency-sensitive applications",
						Suggestion:  "Investigate tail latency causes. Check for GC pauses, lock contention, or slow paths.",
					})
				}
			}

			// Reset for next benchmark
			benchmarkName = ""
			hasThroughput = false
			hasLatency = false
		}
	}

	// Sort by severity
	sort.Slice(issues, func(i, j int) bool {
		severityOrder := map[string]int{"critical": 0, "warning": 1, "info": 2}
		return severityOrder[issues[i].Severity] < severityOrder[issues[j].Severity]
	})

	return issues
}

func printText(issues []PerformanceIssue) {
	if len(issues) == 0 {
		fmt.Println("✓ No performance issues detected!")
		return
	}

	fmt.Printf("Found %d performance issue(s):\n\n", len(issues))

	for i, issue := range issues {
		icon := "⚠"
		if issue.Severity == "critical" {
			icon = "✗"
		} else if issue.Severity == "info" {
			icon = "ℹ"
		}

		fmt.Printf("%s [%s] %s\n", icon, strings.ToUpper(issue.Severity), issue.Description)
		fmt.Printf("  Category: %s\n", issue.Category)
		fmt.Printf("  Impact: %s\n", issue.Impact)
		fmt.Printf("  Suggestion: %s\n", issue.Suggestion)

		if i < len(issues)-1 {
			fmt.Println()
		}
	}
}

func printJSON(issues []PerformanceIssue) {
	fmt.Println("{")
	fmt.Printf("  \"issue_count\": %d,\n", len(issues))
	fmt.Println("  \"issues\": [")

	for i, issue := range issues {
		fmt.Println("    {")
		fmt.Printf("      \"severity\": \"%s\",\n", issue.Severity)
		fmt.Printf("      \"category\": \"%s\",\n", issue.Category)
		fmt.Printf("      \"description\": \"%s\",\n", issue.Description)
		fmt.Printf("      \"impact\": \"%s\",\n", issue.Impact)
		fmt.Printf("      \"suggestion\": \"%s\"\n", issue.Suggestion)

		if i < len(issues)-1 {
			fmt.Println("    },")
		} else {
			fmt.Println("    }")
		}
	}

	fmt.Println("  ]")
	fmt.Println("}")
}

func printMarkdown(issues []PerformanceIssue) {
	if len(issues) == 0 {
		fmt.Println("## Performance Analysis\n\n✓ No performance issues detected!")
		return
	}

	fmt.Printf("## Performance Analysis\n\nFound %d issue(s):\n\n", len(issues))

	critical := 0
	warnings := 0

	for _, issue := range issues {
		if issue.Severity == "critical" {
			critical++
		} else if issue.Severity == "warning" {
			warnings++
		}
	}

	fmt.Printf("- **Critical**: %d\n", critical)
	fmt.Printf("- **Warnings**: %d\n\n", warnings)

	fmt.Println("### Issues\n")

	for _, issue := range issues {
		icon := "⚠️"
		if issue.Severity == "critical" {
			icon = "🔴"
		} else if issue.Severity == "info" {
			icon = "ℹ️"
		}

		fmt.Printf("#### %s [%s] %s\n\n", icon, strings.ToUpper(issue.Severity), issue.Description)
		fmt.Printf("- **Category**: %s\n", issue.Category)
		fmt.Printf("- **Impact**: %s\n", issue.Impact)
		fmt.Printf("- **Suggestion**: %s\n\n", issue.Suggestion)
	}
}
