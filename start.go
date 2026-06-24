package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

type LoadTestStats struct {
	TotalRequests   int64
	SuccessRequests int64
	FailedRequests  int64
	TotalDuration   time.Duration
	MinDuration     time.Duration
	MaxDuration     time.Duration
	AvgDuration     time.Duration
	TotalBytes      int64
}

func main() {
	// Parse command line flags
	url := flag.String("url", "http://localhost:8080", "Target URL to test")
	concurrency := flag.Int("concurrency", 10, "Number of concurrent requests")
	duration := flag.Duration("duration", 30*time.Second, "Duration of the test")
	timeout := flag.Duration("timeout", 10*time.Second, "Request timeout")
	method := flag.String("method", "GET", "HTTP method (GET, POST, etc)")
	flag.Parse()

	fmt.Printf("=== Load Testing Started ===\n")
	fmt.Printf("Target URL: %s\n", *url)
	fmt.Printf("Concurrency: %d\n", *concurrency)
	fmt.Printf("Duration: %v\n", *duration)
	fmt.Printf("Timeout: %v\n", *timeout)
	fmt.Printf("Method: %s\n\n", *method)

	// Initialize stats
	stats := &LoadTestStats{
		MinDuration: time.Hour,
	}
	var mu sync.Mutex

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: *timeout,
	}

	// Channel to control test duration
	stopChan := make(chan struct{})
	var wg sync.WaitGroup

	// Start timer
	startTime := time.Now()

	// Schedule stop after duration
	go func() {
		time.Sleep(*duration)
		close(stopChan)
	}()

	// Launch concurrent workers
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker(client, *url, *method, stopChan, stats, &mu)
		}()
	}

	// Wait for all workers to complete
	wg.Wait()

	// Calculate final stats
	stats.TotalDuration = time.Since(startTime)
	if stats.TotalRequests > 0 {
		stats.AvgDuration = time.Duration(int64(stats.TotalDuration) / stats.TotalRequests)
	}

	// Print results
	printStats(stats)
}

func worker(client *http.Client, targetURL, method string, stopChan chan struct{}, stats *LoadTestStats, mu *sync.Mutex) {
	for {
		select {
		case <-stopChan:
			return
		default:
			// Perform request
			startTime := time.Now()
			req, _ := http.NewRequest(method, targetURL, nil)
			req.Header.Set("User-Agent", "Go-Load-Tester/1.0")

			resp, err := client.Do(req)
			duration := time.Since(startTime)

			mu.Lock()
			atomic.AddInt64(&stats.TotalRequests, 1)

			if err != nil {
				stats.FailedRequests++
				mu.Unlock()
				continue
			}

			// Read response body
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			stats.SuccessRequests++
			stats.TotalBytes += int64(len(body))

			// Update min/max duration
			if duration < stats.MinDuration {
				stats.MinDuration = duration
			}
			if duration > stats.MaxDuration {
				stats.MaxDuration = duration
			}

			mu.Unlock()
		}
	}
}

func printStats(stats *LoadTestStats) {
	fmt.Printf("\n=== Load Testing Results ===\n")
	fmt.Printf("Total Requests:     %d\n", stats.TotalRequests)
	fmt.Printf("Successful:         %d\n", stats.SuccessRequests)
	fmt.Printf("Failed:             %d\n", stats.FailedRequests)
	fmt.Printf("Total Duration:     %v\n", stats.TotalDuration)
	fmt.Printf("Min Response Time:  %v\n", stats.MinDuration)
	fmt.Printf("Max Response Time:  %v\n", stats.MaxDuration)
	fmt.Printf("Avg Response Time:  %v\n", stats.AvgDuration)
	fmt.Printf("Total Bytes:        %d\n", stats.TotalBytes)
	fmt.Printf("Requests/sec:       %.2f\n", float64(stats.TotalRequests)/stats.TotalDuration.Seconds())
	fmt.Printf("Throughput (KB/s):  %.2f\n", float64(stats.TotalBytes)/stats.TotalDuration.Seconds()/1024)

	if stats.TotalRequests > 0 {
		successRate := (float64(stats.SuccessRequests) / float64(stats.TotalRequests)) * 100
		fmt.Printf("Success Rate:       %.2f%%\n", successRate)
	}
}
