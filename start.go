package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
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

// Konstanta untuk kode warna ANSI
const (
	colorCyan    = "\033[36m"
	colorReset   = "\033[0m"
	colorGreen   = "\033[32m" // Untuk status 2xx
	colorYellow  = "\033[33m" // Untuk status 3xx
	colorRed     = "\033[31m" // Untuk status 4xx, 5xx
	colorGray    = "\033[90m" // Untuk debug output
	colorBold    = "\033[1m"
	colorUnbold  = "\033[22m"
)

// Fungsi helper untuk meminta input dari pengguna dengan prompt
func getUserInput(prompt string, defaultValue string) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s (default: %s): ", prompt, defaultValue)
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("Error reading input: %v\n", err)
		return defaultValue
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}
	return input
}

// Fungsi helper untuk meminta input durasi dari pengguna
func getUserDurationInput(prompt string, defaultValue time.Duration) time.Duration {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s (default: %v): ", prompt, defaultValue)
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("Error reading input: %v\n", err)
		return defaultValue
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}
	duration, err := time.ParseDuration(input)
	if err != nil {
		fmt.Printf("Invalid duration format '%s'. Using default: %v\n", input, defaultValue)
		return defaultValue
	}
	return duration
}

// Fungsi helper untuk meminta input integer dari pengguna
func getUserIntInput(prompt string, defaultValue int) int {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s (default: %d): ", prompt, defaultValue)
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("Error reading input: %v\n", err)
		return defaultValue
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}
	val, err := strconv.Atoi(input)
	if err != nil {
		fmt.Printf("Invalid integer format '%s'. Using default: %d\n", input, defaultValue)
		return defaultValue
	}
	return val
}

// Fungsi helper untuk meminta input integer positif dari pengguna
func getUserPositiveIntInput(prompt string, defaultValue int) int {
	val := getUserIntInput(prompt, defaultValue)
	if val <= 0 {
		fmt.Printf("Value must be positive. Using default: %d\n", defaultValue)
		return defaultValue
	}
	return val
}

// Fungsi untuk mendapatkan kode warna berdasarkan status code HTTP
func getStatusCodeColor(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return colorGreen
	case statusCode >= 300 && statusCode < 400:
		return colorYellow
	case statusCode >= 400 || statusCode >= 500:
		return colorRed
	default:
		return colorGray // Status tidak dikenal
	}
}

func main() {
	// --- Ambil Input Parameter dari Pengguna secara Interaktif ---

	// Mode Debug
	debugModeInput := getUserInput("Enable debug mode? (yes/no)", "no")
	debugMode := strings.ToLower(debugModeInput) == "yes"

	// Meminta URL
	targetURL := getUserInput("Enter target URL", "http://localhost:8080")

	// Meminta Concurrency
	concurrency := getUserPositiveIntInput("Enter number of concurrent requests", 10)

	// Meminta Duration
	duration := getUserDurationInput("Enter test duration", 30*time.Second)

	// Meminta Timeout
	timeout := getUserDurationInput("Enter request timeout", 10*time.Second)

	// Meminta Method
	method := getUserInput("Enter HTTP method (GET, POST, etc.)", "GET")
	method = strings.ToUpper(method)

	// -------------------------------------------------------------

	// --- Banner CPA dengan Warna Cyan ---
	bannerCPA := `
   ______      ________
  / ____/___  / ____/ /______
 / / __/ __ \/ / __/ __/ ___/
/ /_/ / /_/ / /_/ / /_/ /
\____/\____/\____/\__/_/
`
	fmt.Printf("%s%s%s", colorCyan, bannerCPA, colorReset)
	// ----------------------------------------------

	fmt.Printf("=== Load Testing Started ===\n")
	fmt.Printf("Debug Mode: %t\n", debugMode) // Tampilkan status debug mode
	fmt.Printf("Target URL: %s\n", targetURL)
	fmt.Printf("Concurrency: %d\n", concurrency)
	fmt.Printf("Duration: %v\n", duration)
	fmt.Printf("Timeout: %v\n", timeout)
	fmt.Printf("Method: %s\n\n", method)

	// Initialize stats
	stats := &LoadTestStats{
		MinDuration: time.Hour,
	}
	var mu sync.Mutex

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: timeout,
	}

	// Channel to control test duration
	stopChan := make(chan struct{})
	var wg sync.WaitGroup

	// Start timer
	startTime := time.Now()

	// Schedule stop after duration
	go func() {
		time.Sleep(duration)
		close(stopChan)
	}()

	// Launch concurrent workers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker(client, targetURL, method, stopChan, stats, &mu, debugMode) // Teruskan debugMode ke worker
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

// Fungsi worker melakukan satu request HTTP
func worker(client *http.Client, targetURL, method string, stopChan chan struct{}, stats *LoadTestStats, mu *sync.Mutex, debugMode bool) {
	for {
		select {
		case <-stopChan:
			return
		default:
			startTime := time.Now()
			req, err := http.NewRequest(method, targetURL, nil)
			if err != nil {
				mu.Lock()
				atomic.AddInt64(&stats.TotalRequests, 1)
				stats.FailedRequests++
				if debugMode {
					fmt.Printf("%s[DEBUG] Error creating request: %v%s\n", colorGray, err, colorReset)
				}
				mu.Unlock()
				continue
			}
			req.Header.Set("User-Agent", "Go-Load-Tester/1.0")

			resp, err := client.Do(req)
			duration := time.Since(startTime)

			mu.Lock()
			atomic.AddInt64(&stats.TotalRequests, 1)

			var statusCode int = -1 // Default jika err != nil
			var respBody string = "" // Untuk menyimpan body respons jika debug

			if err != nil {
				stats.FailedRequests++
				if debugMode {
					fmt.Printf("%s[DEBUG] Request to %s failed: %v (Duration: %v)%s\n", colorGray, targetURL, err, duration, colorReset)
				}
			} else {
				statusCode = resp.StatusCode
				defer resp.Body.Close()

				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					stats.FailedRequests++
					if debugMode {
						// Baca body untuk debug jika status tidak sukses
						bodyBytes, _ := io.ReadAll(resp.Body)
						respBody = string(bodyBytes)
						fmt.Printf("%s[DEBUG] Request to %s failed with status %d (Duration: %v) - Body: %s%s\n", colorGray, targetURL, resp.StatusCode, duration, respBody, colorReset)
					}
				} else {
					stats.SuccessRequests++
					bodyBytes, _ := io.ReadAll(resp.Body)
					respBody = string(bodyBytes) // Simpan body untuk debug
					stats.TotalBytes += int64(len(bodyBytes))

					if duration < stats.MinDuration {
						stats.MinDuration = duration
					}
					if duration > stats.MaxDuration {
						stats.MaxDuration = duration
					}
					if debugMode {
						fmt.Printf("%s[DEBUG] Request to %s succeeded (Status: %d, Duration: %v, Bytes: %d)%s\n", colorGray, targetURL, resp.StatusCode, duration, len(bodyBytes), colorReset)
					}
				}
			}
			mu.Unlock()
		}
	}
}

// Fungsi printStats untuk menampilkan hasil
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

	if stats.TotalDuration.Seconds() > 0 {
		fmt.Printf("Requests/sec:       %.2f\n", float64(stats.TotalRequests)/stats.TotalDuration.Seconds())
		fmt.Printf("Throughput (KB/s):  %.2f\n", float64(stats.TotalBytes)/stats.TotalDuration.Seconds()/1024)
	} else {
		fmt.Printf("Requests/sec:       N/A (Total duration is zero)\n")
		fmt.Printf("Throughput (KB/s):  N/A (Total duration is zero)\n")
	}

	if stats.TotalRequests > 0 {
		successRate := (float64(stats.SuccessRequests) / float64(stats.TotalRequests)) * 100
		fmt.Printf("Success Rate:       %.2f%%\n", successRate)
	}
}

