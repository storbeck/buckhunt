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
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	styleFound = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // Green
	styleWrite = lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // Yellow
	styleAWS   = lipgloss.NewStyle().Foreground(lipgloss.Color("14")) // Cyan
	styleDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))  // Gray
)

type Result struct {
	domain   string
	found    bool
	canRead  bool
	canWrite bool
	awsRead  bool
}

type model struct {
	testing      string
	foundBuckets []string
	stats        Stats
	done         bool
	err          error
	processing   bool
	debug        string
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.done = true
			return m, tea.Quit
		}
	case Result:
		m.stats.increment(msg.found, msg.canRead, msg.canWrite, msg.awsRead)
		if msg.found && (msg.canRead || msg.awsRead) {
			badges := ""
			if msg.canRead {
				badges += " " + styleFound.Render("READ")
			}
			if msg.canWrite {
				badges += " " + styleWrite.Render("WRITE")
			}
			if msg.awsRead {
				badges += " " + styleAWS.Render("AWS")
			}
			m.foundBuckets = append(m.foundBuckets, fmt.Sprintf("%s%s", msg.domain, badges))
		}
		m.testing = msg.domain
		return m, nil
	case string: // debug message
		m.debug = msg
		return m, nil
	case bool: // completion signal
		m.processing = false
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m model) View() string {
	var s strings.Builder

	s.WriteString(styleFound.Render("⚡ Buck Hunter") + "\n")
	s.WriteString(styleDim.Render("Press 'q' to quit\n"))

	// Found buckets
	if len(m.foundBuckets) > 0 {
		s.WriteString("\nFound:\n")
		for _, bucket := range m.foundBuckets {
			s.WriteString(bucket + "\n")
		}
	}

	// Testing status
	if m.processing && m.testing != "" {
		s.WriteString("\n" + styleDim.Render(fmt.Sprintf("Testing: %s", m.testing)))
	}

	// Debug info
	if m.debug != "" {
		s.WriteString("\n" + styleDim.Render(m.debug))
	}

	// Summary when done
	if m.done {
		s.WriteString(fmt.Sprintf("\nSummary: %d tested, %d found (%d readable, %d writable, %d aws), %d not found\n",
			m.stats.total, m.stats.found, m.stats.withRead, m.stats.withWrite, m.stats.withAwsRead, m.stats.notFound))
	}

	return s.String()
}

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

func analyzeBucket(domain string) Result {
	result := Result{
		domain: domain,
	}

	cmd := exec.Command("aws", "s3", "ls", "s3://"+domain)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()

	if err == nil {
		result.found = true
		result.canRead = true
		result.awsRead = true
		return result
	}

	if strings.Contains(stderr.String(), "NoSuchBucket") {
		return result
	}

	if strings.Contains(stderr.String(), "AccessDenied") || strings.Contains(stderr.String(), "AllAccessDisabled") {
		result.found = true
	}
	return result
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

	stat, _ := os.Stdin.Stat()
	isPipe := (stat.Mode() & os.ModeCharDevice) == 0

	if isPipe {
		if *quietMode {
			// Process in quiet mode (CSV output)
			jobs := make(chan string, *workers)
			results := make(chan Result, *workers)
			var wg sync.WaitGroup
			stats := &Stats{}

			// Start workers
			for i := 0; i < *workers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for domain := range jobs {
						results <- analyzeBucket(domain)
					}
				}()
			}

			// Process results
			go func() {
				wg.Wait()
				close(results)
			}()

			// Read domains
			go func() {
				scanner := bufio.NewScanner(os.Stdin)
				for scanner.Scan() {
					domain := strings.TrimSpace(scanner.Text())
					if domain != "" {
						jobs <- domain
					}
				}
				close(jobs)
			}()

			// Print CSV results
			for result := range results {
				stats.increment(result.found, result.canRead, result.canWrite, result.awsRead)
				if result.found && (result.canRead || result.awsRead) {
					fmt.Printf("%s,%v,%v,%v\n", result.domain, result.canRead, result.canWrite, result.awsRead)
				}
			}
			return
		}

		// Interactive mode with TUI
		p := tea.NewProgram(model{processing: true})

		jobs := make(chan string, *workers*2) // Increase buffer size
		results := make(chan Result, *workers*2)
		done := make(chan struct{})
		var wg sync.WaitGroup
		var jobCount atomic.Int32

		// Start workers
		for i := 0; i < *workers; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for domain := range jobs {
					select {
					case <-done:
						return
					default:
						result := analyzeBucket(domain)
						select {
						case results <- result:
							jobCount.Add(1)
						case <-done:
							return
						}
					}
				}
				p.Send(fmt.Sprintf("Debug: Worker %d finished", id))
			}(i)
		}

		// Process results and update UI
		go func() {
			for result := range results {
				select {
				case <-done:
					return
				default:
					p.Send(result)
				}
			}
			p.Send(fmt.Sprintf("Debug: Processed %d results", jobCount.Load()))
			p.Send(false) // signal completion
		}()

		// Process completion
		go func() {
			wg.Wait()
			p.Send("Debug: All workers finished")
			close(results)
			close(done)
		}()

		// Read domains
		go func() {
			defer func() {
				close(jobs)
				p.Send("Debug: Jobs channel closed")
			}()

			scanner := bufio.NewScanner(os.Stdin)
			count := 0
			skipped := 0
			for scanner.Scan() {
				select {
				case <-done:
					p.Send(fmt.Sprintf("Debug: Scanner stopped at %d domains (%d skipped)", count, skipped))
					return
				default:
					domain := strings.TrimSpace(scanner.Text())
					if domain != "" && !strings.HasPrefix(domain, "*") {
						count++
						select {
						case jobs <- domain:
							if count%10 == 0 {
								p.Send(fmt.Sprintf("Debug: Queued %d domains (skipped %d wildcards)", count, skipped))
							}
						case <-done:
							p.Send(fmt.Sprintf("Debug: Stopped at %d domains (%d skipped)", count, skipped))
							return
						}
					} else if strings.HasPrefix(domain, "*") {
						skipped++
					}
				}
			}

			if err := scanner.Err(); err != nil {
				p.Send(fmt.Sprintf("Debug: Scanner error: %v", err))
				return
			}
			p.Send(fmt.Sprintf("Debug: Total domains queued: %d (skipped %d wildcards)", count, skipped))
		}()

		if _, err := p.Run(); err != nil {
			fmt.Printf("Error running program: %v\n", err)
			os.Exit(1)
		}
		return
	}

	args := flag.Args()
	if len(args) == 0 {
		fmt.Println("Usage:")
		fmt.Println("  Single domain:  buckhunt [-q] <domain>")
		fmt.Println("  Multiple domains via stdin:  cat domains.txt | buckhunt [-q]")
		os.Exit(1)
	}

	// Handle single domain case
	result := analyzeBucket(args[0])
	if *quietMode {
		fmt.Printf("%s,%v,%v,%v\n", result.domain, result.canRead, result.canWrite, result.awsRead)
	} else {
		if result.found {
			fmt.Printf("Found bucket %s:\n", result.domain)
			fmt.Printf("  Read:  %v\n", result.canRead)
			fmt.Printf("  Write: %v\n", result.canWrite)
			fmt.Printf("  AWS:   %v\n", result.awsRead)
		} else {
			fmt.Printf("Bucket %s not found\n", result.domain)
		}
	}
}
