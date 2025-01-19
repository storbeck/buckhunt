package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

const (
	gray = "\033[38;5;242m" // Dark gray color
	reset = "\033[0m"       // Reset color
)

type Stats struct {
	total       int
	found       int
	notFound    int
	withRead    int
	withWrite   int
	withAwsRead int
	mu          sync.Mutex
}

func (s *Stats) increment(found bool, canRead, canWrite, awsRead bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.total++
	if !found {
		s.notFound++
		return
	}
	s.found++
	if canRead {
		s.withRead++
	}
	if canWrite {
		s.withWrite++
	}
	if awsRead {
		s.withAwsRead++
	}
}

type Result struct {
	domain   string
	found    bool
	canRead  bool
	canWrite bool
	awsRead  bool
}

func analyzeBucket(domain string) Result {
	result := Result{
		domain: domain,
	}

	// Try AWS CLI access first
	cmd := exec.Command("aws", "s3", "ls", "s3://"+domain)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()

	// If we can list, it's readable
	if err == nil {
		result.found = true
		result.canRead = true
		result.awsRead = true
		return result
	}

	// If it's a NoSuchBucket error, bucket doesn't exist
	if strings.Contains(stderr.String(), "NoSuchBucket") {
		return result
	}

	// If we get an error but not NoSuchBucket, the bucket exists but we can't list it
	if strings.Contains(stderr.String(), "AccessDenied") || strings.Contains(stderr.String(), "AllAccessDisabled") {
		result.found = true
	}
	return result
}

func updateLogWindow(lines []string) {
	// Clear previous lines
	fmt.Fprint(os.Stderr, "\033[2K\r")  // Clear current line
	for i := 0; i < 3; i++ {
		fmt.Fprint(os.Stderr, "\033[A\033[2K\r")  // Move up and clear line
	}
	
	// Print empty lines to fill window
	for i := 0; i < 3-len(lines); i++ {
		fmt.Fprintln(os.Stderr)
	}
	
	// Print actual lines
	for _, line := range lines {
		fmt.Fprintf(os.Stderr, "%s%s%s\n", gray, line, reset)
	}
}

func worker(jobs <-chan string, results chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()
	for domain := range jobs {
		results <- analyzeBucket(domain)
	}
}

func main() {
	quietMode := flag.Bool("q", false, "Quiet mode - only output CSV format: domain,read,write,aws")
	workers := flag.Int("w", 20, "Number of concurrent workers")
	flag.Parse()

	if *workers < 1 {
		*workers = 1
	} else if *workers > 100 {
		*workers = 100
	}

	stats := &Stats{}
	var lastThree []string
	var logMutex sync.Mutex

	stat, _ := os.Stdin.Stat()
	isPipe := (stat.Mode()&os.ModeCharDevice) == 0

	if isPipe {
		if !*quietMode {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr)
		}

		// Create channels
		jobs := make(chan string, *workers)
		results := make(chan Result, *workers)

		// Start workers
		var wg sync.WaitGroup
		for i := 0; i < *workers; i++ {
			wg.Add(1)
			go worker(jobs, results, &wg)
		}

		// Process results in the background
		doneChan := make(chan struct{})
		go func() {
			wg.Wait()
			close(results)
			close(doneChan)
		}()

		// Start scanner in a goroutine
		go func() {
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				domain := cleanDomain(scanner.Text())
				if domain != "" {
					jobs <- domain
				}
			}
			if err := scanner.Err(); err != nil {
				fmt.Fprintf(os.Stderr, "Error reading from stdin: %v\n", err)
			}
			close(jobs)
		}()

		// Process results as they come in
		for {
			select {
			case result, ok := <-results:
				if !ok {
					// Results channel closed, we're done
					if !*quietMode {
						fmt.Fprintf(os.Stderr, "\n# Summary: %d tested, %d found (%d readable, %d writable, %d aws), %d not found\n",
							stats.total, stats.found, stats.withRead, stats.withWrite, stats.withAwsRead, stats.notFound)
					}
					return
				}
				
				stats.increment(result.found, result.canRead, result.canWrite, result.awsRead)
				
				if !*quietMode {
					logMutex.Lock()
					lastThree = append(lastThree, fmt.Sprintf("Testing: %s", result.domain))
					if len(lastThree) > 3 {
						lastThree = lastThree[1:]
					}
					updateLogWindow(lastThree)
					logMutex.Unlock()
				}

				if result.found && (result.canRead || result.awsRead) {
					fmt.Printf("%s,%v,%v,%v\n", result.domain, result.canRead, result.canWrite, result.awsRead)
				}
			}
		}
	}

	args := flag.Args()
	if len(args) == 0 {
		fmt.Println("Usage:")
		fmt.Println("  Single domain:  buckhunt [-q] <domain>")
		fmt.Println("  Multiple domains: cat domains.txt | buckhunt [-q] [-w workers]")
		fmt.Println("\nFlags:")
		fmt.Println("  -q    Quiet mode - only output CSV format: domain,read,write,aws")
		fmt.Println("  -w    Number of concurrent workers (default: 20, max: 100)")
		fmt.Println("\nExample:")
		fmt.Println("  ./buckhunt flaws.cloud")
		fmt.Println("  cat domains.txt | ./buckhunt -q -w 50")
		return
	}

	domain := cleanDomain(args[0])
	if domain != "" {
		if !*quietMode {
			fmt.Fprintf(os.Stderr, "Testing: %s\n", domain)
		}
		result := analyzeBucket(domain)
		stats.increment(result.found, result.canRead, result.canWrite, result.awsRead)
		if result.found && (result.canRead || result.awsRead) {
			fmt.Printf("%s,%v,%v,%v\n", result.domain, result.canRead, result.canWrite, result.awsRead)
		}
		if !*quietMode {
			fmt.Fprintf(os.Stderr, "\n# Summary: %d tested, %d found (%d readable, %d writable, %d aws), %d not found\n",
				stats.total, stats.found, stats.withRead, stats.withWrite, stats.withAwsRead, stats.notFound)
		}
	}
}

func cleanDomain(domain string) string {
	domain = strings.TrimSpace(domain)
	domain = strings.ToLower(domain)
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "s3://")
	domain = strings.TrimSuffix(domain, "/")
	return domain
}
