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

func checkBucketACL(bucketURL string) (bool, bool, error) {
	// Check GET (read) permission
	respGet, err := http.Get(bucketURL)
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

	client := &http.Client{}
	respPut, err := client.Do(req)
	if err != nil {
		return canRead, false, err
	}
	defer respPut.Body.Close()
	canWrite := respPut.StatusCode == http.StatusOK

	return canRead, canWrite, nil
}

func downloadBucket(bucketName string) error {
	// Create directory
	err := os.MkdirAll(bucketName, 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	// Use AWS CLI to download recursively
	fmt.Printf("[+] Downloading bucket contents to directory: %s\n", bucketName)
	cmd := exec.Command("aws", "s3", "sync", "s3://"+bucketName, bucketName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func analyzeBucket(bucketName string, quietMode bool) {
	if !quietMode {
		fmt.Printf("\n[+] Checking s3://%s\n", bucketName)
	}

	bucketURL := fmt.Sprintf("http://%s.s3.amazonaws.com", bucketName)

	// Check if bucket exists
	resp, err := http.Head(bucketURL)
	if err != nil {
		if !quietMode {
			fmt.Printf("[-] Error: %v\n", err)
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		if !quietMode {
			fmt.Println("[-] Bucket does not exist")
		}
		return
	}

	// Check permissions
	canRead, canWrite, _ := checkBucketACL(bucketURL)
	if !canRead && !canWrite {
		if !quietMode {
			fmt.Println("[-] Bucket exists but is not public")
		}
		return
	}

	if quietMode {
		// In quiet mode, just output in format: domain,read,write
		fmt.Printf("%s,%v,%v\n", bucketName, canRead, canWrite)
		return
	}

	fmt.Printf("[+] Public Read:  %v\n", canRead)
	fmt.Printf("[+] Public Write: %v\n", canWrite)

	if !canRead {
		return
	}

	// Only proceed with listing and download option if not in quiet mode
	if !quietMode {
		// List bucket contents
		resp, err = http.Get(bucketURL)
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

		// Ask to download
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
	// Remove any protocol and get just the domain
	domain = strings.TrimPrefix(strings.TrimPrefix(domain, "http://"), "https://")
	// Remove any path
	if idx := strings.Index(domain, "/"); idx != -1 {
		domain = domain[:idx]
	}
	return strings.TrimSpace(domain)
}

func main() {
	// Parse flags
	quietMode := flag.Bool("q", false, "Quiet mode - only output CSV format: domain,read,write")
	flag.Parse()

	// Check if we have input from pipe
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		// Reading from pipe
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			domain := cleanDomain(scanner.Text())
			if domain != "" {
				analyzeBucket(domain, *quietMode)
			}
		}
	} else {
		// Normal command line argument
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
			analyzeBucket(domain, *quietMode)
		}
	}
}
