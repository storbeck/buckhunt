package main

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type S3ListBucketResult struct {
	XMLName  xml.Name `xml:"ListBucketResult"`
	Contents []struct {
		Key          string    `xml:"Key"`
		LastModified time.Time `xml:"LastModified"`
		Size         int64     `xml:"Size"`
	} `xml:"Contents"`
}

type Stats struct {
	total        int32
	found        int32
	notFound     int32
	withRead     int32
	withWrite    int32
	withAwsRead  int32
}

func (s *Stats) increment(found bool, canRead, canWrite, awsRead bool) {
	atomic.AddInt32(&s.total, 1)
	if !found {
		atomic.AddInt32(&s.notFound, 1)
		return
	}
	atomic.AddInt32(&s.found, 1)
	if canRead {
		atomic.AddInt32(&s.withRead, 1)
	}
	if canWrite {
		atomic.AddInt32(&s.withWrite, 1)
	}
	if awsRead {
		atomic.AddInt32(&s.withAwsRead, 1)
	}
}

type Result struct {
	domain    string
	found     bool
	canRead   bool
	canWrite  bool
	awsRead   bool
	files     []S3File
}

type S3File struct {
	Name         string
	Size         int64
	LastModified time.Time
}

func checkBucketACL(bucketURL string) (bool, bool, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	respGet, err := client.Get(bucketURL)
	if err != nil {
		return false, false, err
	}
	defer respGet.Body.Close()
	canRead := respGet.StatusCode == http.StatusOK

	testFile := "permission_test_" + fmt.Sprint(time.Now().Unix())
	req, err := http.NewRequest(http.MethodPut, bucketURL+"/"+testFile, strings.NewReader("test"))
	if err != nil {
		return canRead, false, err
	}

	respPut, err := client.Do(req)
	if err != nil {
		return canRead, false, err
	}
	defer respPut.Body.Close()
	canWrite := respPut.StatusCode == http.StatusOK

	return canRead, canWrite, nil
}

func checkAwsAccess(bucketName string) (bool, []S3File, error) {
	cmd := exec.Command("aws", "s3", "ls", bucketName)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	if err != nil {
		return false, nil, fmt.Errorf("aws error: %v - %s", err, stderr.String())
	}

	// Parse the ls output
	var files []S3File
	scanner := bufio.NewScanner(bytes.NewReader(stdout.Bytes()))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) >= 4 {
			// AWS S3 LS format: 2023-01-17 10:00:00 SIZE filename
			dateStr := parts[0] + " " + parts[1]
			lastMod, _ := time.Parse("2006-01-02 15:04:05", dateStr)
			size, _ := strconv.Atoi(parts[2])
			name := strings.Join(parts[3:], " ")
			files = append(files, S3File{
				Name:         name,
				Size:         int64(size),
				LastModified: lastMod,
			})
		}
	}

	return true, files, nil
}

func downloadBucket(bucketName string) error {
	err := os.MkdirAll(bucketName, 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	fmt.Printf("[+] Downloading bucket contents to directory: %s\n", bucketName)
	cmd := exec.Command("aws", "s3", "sync", "s3://"+bucketName, bucketName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func analyzeBucket(bucketName string, quietMode bool) Result {
	result := Result{
		domain: bucketName,
	}

	if !quietMode {
		fmt.Printf("\n[+] Checking s3://%s\n", bucketName)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	bucketURL := fmt.Sprintf("http://%s.s3.amazonaws.com", bucketName)

	// First check AWS access
	awsRead, files, _ := checkAwsAccess(bucketName)
	if awsRead {
		result.found = true
		result.awsRead = true
		result.files = files
	}

	// Then check public access
	resp, err := client.Head(bucketURL)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			result.found = true
			canRead, canWrite, _ := checkBucketACL(bucketURL)
			result.canRead = canRead
			result.canWrite = canWrite
		}
	}

	if !quietMode {
		if result.found {
			fmt.Printf("[+] Public Read:  %v\n", result.canRead)
			fmt.Printf("[+] Public Write: %v\n", result.canWrite)
			fmt.Printf("[+] AWS Read:     %v\n", result.awsRead)

			if result.awsRead {
				fmt.Println("\nFiles (via AWS):")
				for _, file := range result.files {
					fmt.Printf("%s\t%d bytes\t%s\n",
						file.LastModified.Format("2006-01-02 15:04:05"),
						file.Size,
						file.Name)
				}
			} else if result.canRead {
				resp, err := client.Get(bucketURL)
				if err == nil {
					defer resp.Body.Close()
					body, err := io.ReadAll(resp.Body)
					if err == nil {
						var listResult S3ListBucketResult
						if err := xml.Unmarshal(body, &listResult); err == nil {
							fmt.Println("\nFiles (via HTTP):")
							for _, item := range listResult.Contents {
								fmt.Printf("%s\t%d bytes\t%s\n",
									item.LastModified.Format("2006-01-02 15:04:05"),
									item.Size,
									item.Key)
							}
						}
					}
				}
			}

			if result.canRead || result.awsRead {
				fmt.Print("\nDownload bucket contents? [y/N]: ")
				reader := bufio.NewReader(os.Stdin)
				response, _ := reader.ReadString('\n')
				response = strings.ToLower(strings.TrimSpace(response))

				if response == "y" || response == "yes" {
					err := downloadBucket(bucketName)
					if err != nil {
						fmt.Printf("[-] Download failed: %v\n", err)
					} else {
						fmt.Println("[+] Download completed successfully")
					}
				}
			}
		} else {
			fmt.Println("[-] Bucket does not exist")
		}
	}

	return result
}

func worker(jobs <-chan string, results chan<- Result, quietMode bool, wg *sync.WaitGroup) {
	defer wg.Done()
	for domain := range jobs {
		results <- analyzeBucket(domain, quietMode)
	}
}

func cleanDomain(domain string) string {
	domain = strings.TrimPrefix(strings.TrimPrefix(domain, "http://"), "https://")
	if idx := strings.Index(domain, "/"); idx != -1 {
		domain = domain[:idx]
	}
	return strings.TrimSpace(domain)
}

func processResults(results <-chan Result, stats *Stats, quietMode bool, total int) {
	processed := 0
	for result := range results {
		stats.increment(result.found, result.canRead, result.canWrite, result.awsRead)
		
		if quietMode && (result.canRead || result.canWrite || result.awsRead) {
			fmt.Printf("%s,%v,%v,%v\n", result.domain, result.canRead, result.canWrite, result.awsRead)
		}
		
		processed++
		if processed == total {
			break
		}
	}
}

func main() {
	quietMode := flag.Bool("q", false, "Quiet mode - only output CSV format: domain,read,write,aws")
	workers := flag.Int("w", 10, "Number of concurrent workers")
	flag.Parse()

	if *workers < 1 {
		*workers = 1
	} else if *workers > 100 {
		*workers = 100
	}

	stats := &Stats{}

	stat, _ := os.Stdin.Stat()
	isPipe := (stat.Mode() & os.ModeCharDevice) == 0

	if isPipe {
		var domains []string
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			if domain := cleanDomain(scanner.Text()); domain != "" {
				domains = append(domains, domain)
			}
		}

		if err := scanner.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "Error reading from stdin: %v\n", err)
			os.Exit(1)
		}

		jobs := make(chan string, len(domains))
		results := make(chan Result, len(domains))

		var wg sync.WaitGroup
		workerCount := min(*workers, len(domains))
		wg.Add(workerCount)
		for i := 0; i < workerCount; i++ {
			go worker(jobs, results, *quietMode, &wg)
		}

		for _, domain := range domains {
			jobs <- domain
		}
		close(jobs)

		go processResults(results, stats, *quietMode, len(domains))

		wg.Wait()
		close(results)

		if *quietMode {
			fmt.Printf("# Summary: %d tested, %d found (%d readable, %d writable, %d aws), %d not found\n",
				stats.total, stats.found, stats.withRead, stats.withWrite, stats.withAwsRead, stats.notFound)
		}
	} else {
		args := flag.Args()
		if len(args) != 1 {
			fmt.Println("Usage:")
			fmt.Println("  Single domain:  buckhunt [-q] <domain>")
			fmt.Println("  Multiple domains: cat domains.txt | buckhunt [-q] [-w workers]")
			fmt.Println("\nFlags:")
			fmt.Println("  -q    Quiet mode - only output CSV format: domain,read,write,aws")
			fmt.Println("  -w    Number of concurrent workers (default: 10, max: 100)")
			fmt.Println("\nExample:")
			fmt.Println("  ./buckhunt flaws.cloud")
			fmt.Println("  cat domains.txt | ./buckhunt -q -w 20")
			os.Exit(1)
		}

		domain := cleanDomain(args[0])
		if domain != "" {
			result := analyzeBucket(domain, *quietMode)
			stats.increment(result.found, result.canRead, result.canWrite, result.awsRead)
		}
	}
}
