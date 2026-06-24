package main

import (
	"bufio"        // Untuk membaca input dari terminal
	"bytes"        // Untuk membuat io.Reader dari string body
	"fmt"          // Untuk mencetak output
	"io"           // Untuk io.Reader dan io.ReadAll
	"net/http"     // Untuk membuat permintaan HTTP
	"os"           // Untuk os.Stdin, os.Stdout
	"strconv"      // Untuk konversi string ke integer
	"strings"      // Untuk manipulasi string (TrimSpace, ToUpper)
	"sync"         // Untuk WaitGroup dan Mutex
	"sync/atomic"  // Untuk operasi atomic pada counter
	"time"         // Untuk durasi, sleep, dan timestamp
)

// LoadTestStats menyimpan statistik pengujian beban
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

// getUserInput meminta input string dari pengguna dengan nilai default.
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

// getUserSecondsInput meminta input angka bulat (dalam detik) dan mengembalikannya sebagai time.Duration.
func getUserSecondsInput(prompt string, defaultValue time.Duration) time.Duration {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s (default: %.0fs): ", prompt, defaultValue.Seconds())
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("Error reading input: %v\n", err)
		return defaultValue
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}

	seconds, err := strconv.Atoi(input)
	if err != nil {
		fmt.Printf("Invalid input '%s'. Please enter a whole number of seconds. Using default: %.0fs\n", input, defaultValue.Seconds())
		return defaultValue
	}

	return time.Duration(seconds) * time.Second
}

// getUserIntInput meminta input integer dari pengguna dengan nilai default.
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

// getUserPositiveIntInput meminta input integer positif dari pengguna.
func getUserPositiveIntInput(prompt string, defaultValue int) int {
	val := getUserIntInput(prompt, defaultValue)
	if val <= 0 {
		fmt.Printf("Value must be positive. Using default: %d\n", defaultValue)
		return defaultValue
	}
	return val
}

// getStatusCodeColor mengembalikan kode warna ANSI berdasarkan status code HTTP.
func getStatusCodeColor(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return colorGreen
	case statusCode >= 300 && statusCode < 400:
		return colorYellow
	case statusCode >= 400 || statusCode >= 500:
		return colorRed
	default:
		return colorGray // Status tidak dikenal atau error koneksi
	}
}

func main() {
	// --- Ambil Input Parameter dari Pengguna secara Interaktif ---

	debugModeInput := getUserInput("Enable debug mode? (yes/no)", "no")
	debugMode := strings.ToLower(debugModeInput) == "yes"

	targetURL := getUserInput("Enter target URL", "http://localhost:8080")

	concurrency := getUserPositiveIntInput("Enter number of concurrent requests", 10)

	duration := getUserSecondsInput("Enter test duration (in seconds)", 30*time.Second)

	timeout := getUserSecondsInput("Enter request timeout (in seconds)", 10*time.Second)

	method := getUserInput("Enter HTTP method (GET, POST, PUT, DELETE, etc.)", "GET")
	method = strings.ToUpper(method)

	var requestBody string = ""
	var contentType string = ""

	if method == "POST" || method == "PUT" || method == "PATCH" {
		requestBody = getUserInput("Enter request body (leave empty for none)", "")
		if requestBody != "" {
			contentType = getUserInput("Enter Content-Type header (e.g., application/json, application/x-www-form-urlencoded)", "application/json")
		}
	}

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
	fmt.Printf("Debug Mode: %t\n", debugMode)
	fmt.Printf("Target URL: %s\n", targetURL)
	fmt.Printf("Concurrency: %d\n", concurrency)
	fmt.Printf("Duration: %v\n", duration)
	fmt.Printf("Timeout: %v\n", timeout)
	fmt.Printf("Method: %s\n", method)
	if requestBody != "" {
		fmt.Printf("Content-Type: %s\n", contentType)
		if debugMode && len(requestBody) < 200 {
			fmt.Printf("Request Body: %s\n", requestBody)
		} else if debugMode && len(requestBody) >= 200 {
			fmt.Printf("Request Body: [Too long to display]\n")
		}
	}
	fmt.Println()

	stats := &LoadTestStats{
		MinDuration: time.Hour,
	}
	var mu sync.Mutex

	client := &http.Client{
		Timeout: timeout,
	}

	stopChan := make(chan struct{})
	var wg sync.WaitGroup

	startTime := time.Now()

	go func() {
		time.Sleep(duration)
		close(stopChan)
	}()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker(client, targetURL, method, requestBody, contentType, stopChan, stats, &mu, debugMode)
		}()
	}

	wg.Wait()

	stats.TotalDuration = time.Since(startTime)
	if stats.TotalRequests > 0 {
		stats.AvgDuration = time.Duration(int64(stats.TotalDuration) / stats.TotalRequests)
	}

	printStats(stats)
}

// worker melakukan satu request HTTP
func worker(client *http.Client, targetURL, method, requestBody, contentType string, stopChan chan struct{}, stats *LoadTestStats, mu *sync.Mutex, debugMode bool) {
	for {
		select {
		case <-stopChan:
			return
		default:
			startTime := time.Now()
			var reqBodyReader io.Reader = nil
			if requestBody != "" {
				reqBodyReader = bytes.NewBufferString(requestBody)
			}

			req, err := http.NewRequest(method, targetURL, reqBodyReader)
			if err != nil {
				mu.Lock()
				atomic.AddInt64(&stats.TotalRequests, 1)
				stats.FailedRequests++
				if debugMode {
					// Menggunakan fmt.Sprintln untuk mencetak satu baris pesan debug yang sudah diformat
					debugMsg := fmt.Sprintf("%s[DEBUG] Error creating request: %v (Duration: %v)%s", colorGray, err, time.Since(startTime), colorReset)
					fmt.Fprintln(os.Stdout, debugMsg) // Fprintln akan menambahkan newline secara otomatis
				}
				mu.Unlock()
				continue
			}

			if requestBody != "" && contentType != "" {
				req.Header.Set("Content-Type", contentType)
			}
			req.Header.Set("User-Agent", "Go-Load-Tester/1.0")

			resp, err := client.Do(req)
			duration := time.Since(startTime)

			mu.Lock()
			atomic.AddInt64(&stats.TotalRequests, 1)

			if err != nil {
				stats.FailedRequests++
				if debugMode {
					debugMsg := fmt.Sprintf("%s[DEBUG] Request to %s failed: %v (Duration: %v)%s", colorGray, targetURL, err, duration, colorReset)
					fmt.Fprintln(os.Stdout, debugMsg)
				}
			} else {
				statusCode := resp.StatusCode
				defer resp.Body.Close()

				if statusCode < 200 || statusCode >= 300 {
					stats.FailedRequests++
					if debugMode {
						bodyBytes, _ := io.ReadAll(resp.Body)
						respBody := string(bodyBytes)
						statusCodeColor := getStatusCodeColor(statusCode)
						debugMsg := fmt.Sprintf("%s[DEBUG] Request to %s failed with status %s%d%s (Duration: %v) - Body: %s%s",
							colorGray, targetURL, statusCodeColor, statusCode, colorReset, duration, respBody, colorGray, colorReset)
						fmt.Fprintln(os.Stdout, debugMsg)
					}
				} else {
					stats.SuccessRequests++
					bodyBytes, _ := io.ReadAll(resp.Body)
					stats.TotalBytes += int64(len(bodyBytes))

					if duration < stats.MinDuration {
						stats.MinDuration = duration
					}
					if duration > stats.MaxDuration {
						stats.MaxDuration = duration
					}
					if debugMode {
						statusCodeColor := getStatusCodeColor(statusCode)
						debugMsg := fmt.Sprintf("%s[DEBUG] Request to %s succeeded (Status: %s%d%s, Duration: %v, Bytes: %d)%s",
							colorGray, targetURL, statusCodeColor, statusCode, colorReset, duration, len(bodyBytes), colorGray, colorReset)
						fmt.Fprintln(os.Stdout, debugMsg)
					}
				}
			}
			mu.Unlock()
		}
	}
}

// printStats menampilkan hasil pengujian beban.
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
		successColor := colorGreen
		if successRate < 50 {
			successColor = colorRed
		} else if successRate < 90 {
			successColor = colorYellow
		}
		fmt.Printf("Success Rate:       %s%.2f%%%s\n", successColor, successRate, colorReset)
	}
}
