package main

import (
	"bufio"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
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
	total     int
	found     int
	notFound  int
	withRead  int
	withWrite int
}

func checkBucketACL(bucketURL string) (bool, bool, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Check GET (read) permission
	respGet, err := client.Get(bucketURL)
	if err != nil {
		return false, false, err
	}
	defer respGet.Body.Close()
	canRead := respGet.StatusCode == http.StatusOK

	// Check PUT (write) permission
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

func analyzeBucket(bucketName string, quietMode bool, stats *Stats) {
	stats.total++

	if !quietMode {
		fmt.Printf("\n[+] Checking s3://%s\n", bucketName)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	bucketURL := fmt.Sprintf("http://%s.s3.amazonaws.com", bucketName)

	// Check if bucket exists
	resp, err := client.Head(bucketURL)
	if err != nil {
		stats.notFound++
		if !quietMode {
			fmt.Printf("[-] Error: %v\n", err)
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		stats.notFound++
		if !quietMode {
			fmt.Println("[-] Bucket does not exist")
		}
		return
	}

	stats.found++

	// Check permissions
	canRead, canWrite, err := checkBucketACL(bucketURL)
	if err != nil {
		if !quietMode {
			fmt.Printf("[-] Error checking permissions: %v\n", err)
		}
		return
	}

	if canRead {
		stats.withRead++
	}
	if canWrite {
		stats.withWrite++
	}

	if !canRead && !canWrite {
		if !quietMode {
			fmt.Println("[-] Bucket exists but is not public")
		}
		return
	}

	if quietMode {
		// Only output buckets we can access
		fmt.Printf("%s,%v,%v\n", bucketName, canRead, canWrite)
		return
	}

	fmt.Printf("[+] Public Read:  %v\n", canRead)
	fmt.Printf("[+] Public Write: %v\n", canWrite)

	if !canRead {
		return
	}

	if !quietMode {
		resp, err = client.Get(bucketURL)
		if err != nil {
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return
		}

		var result S3ListBucketResult
		err = xml.Unmarshal(body, &result)
		if err != nil {
			return
		}

		fmt.Println("\nFiles:")
		for _, item := range result.Contents {
			fmt.Printf("%s\t%d bytes\t%s\n",
				item.LastModified.Format("2006-01-02 15:04:05"),
				item.Size,
				item.Key)
		}

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
}

func cleanDomain(domain string) string {
	domain = strings.TrimPrefix(strings.TrimPrefix(domain, "http://"), "https://")
	if idx := strings.Index(domain, "/"); idx != -1 {
		domain = domain[:idx]
	}
	return strings.TrimSpace(domain)
}

func main() {
	quietMode := flag.Bool("q", false, "Quiet mode - only output CSV format: domain,read,write")
	flag.Parse()

	stats := &Stats{}

	stat, _ := os.Stdin.Stat()
	isPipe := (stat.Mode() & os.ModeCharDevice) == 0

	if isPipe {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			domain := cleanDomain(scanner.Text())
			if domain != "" {
				analyzeBucket(domain, *quietMode, stats)
			}
		}

		if err := scanner.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "Error reading from stdin: %v\n", err)
			os.Exit(1)
		}

		if *quietMode {
			fmt.Printf("# Summary: %d tested, %d found (%d readable, %d writable), %d not found\n", 
				stats.total, stats.found, stats.withRead, stats.withWrite, stats.notFound)
		}
	} else {
		args := flag.Args()
		if len(args) != 1 {
			fmt.Println("Usage:")
			fmt.Println("  Single domain:  buckhunt [-q] <domain>")
			fmt.Println("  Multiple domains: cat domains.txt | buckhunt [-q]")
			fmt.Println("\nFlags:")
			fmt.Println("  -q    Quiet mode - only output CSV format: domain,read,write")
			fmt.Println("\nExample:")
			fmt.Println("  ./buckhunt flaws.cloud")
			fmt.Println("  cat domains.txt | ./buckhunt -q")
			os.Exit(1)
		}

		domain := cleanDomain(args[0])
		if domain != "" {
			analyzeBucket(domain, *quietMode, stats)
		}
	}
}
