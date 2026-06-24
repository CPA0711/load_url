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
	// --- Input User ---
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

	// --- Banner Baru ---
	bannerArt := `
╭─╴╭─╮╭─╮   ╭─╴╷  ╭─╮╭─╮╶┬╮
│  ├─╯├─┤   ├╴ │  │ ││ │ ││
╰─╴╵  ╵ ╵   ╵  ╰─╴╰─╯╰─╯╶┴╯
`
	fmt.Printf("%s%s%s", colorCyan, bannerArt, colorReset)
	// ----------------------------------------------

	fmt.Printf("=== CPA FLOOD Started ===\n")
	fmt.Printf("Target: %s\n", targetURL)
	fmt.Printf("Method: %s, Concurrency: %d, Duration: %v, Timeout: %v\n", method, concurrency, duration, timeout)
	if requestBody != "" {
		fmt.Printf("Body provided (Content-Type: %s)\n", contentType)
	}
	fmt.Println() // Baris kosong untuk pemisah

	// --- Setup Pengujian ---
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

	// --- Launch Workers ---
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker(client, targetURL, method, requestBody, contentType, stopChan, stats, &mu, debugMode)
		}()
	}

	wg.Wait() // Tunggu semua worker selesai

	// --- Statistik Akhir ---
	stats.TotalDuration = time.Since(startTime)
	if stats.TotalRequests > 0 {
		stats.AvgDuration = time.Duration(int64(stats.TotalDuration) / stats.TotalRequests)
	}

	printSimplifiedStats(stats) // Gunakan fungsi statistik yang disederhanakan
}

// worker melakukan satu request HTTP.
// Debug output di sini disederhanakan.
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
					debugMsg := fmt.Sprintf("[DEBUG] Err creating req: %v", err)
					fmt.Fprintln(os.Stdout, debugMsg)
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
					debugMsg := fmt.Sprintf("[DEBUG] Req %s failed: %v", targetURL, err)
					fmt.Fprintln(os.Stdout, debugMsg)
				}
			} else {
				statusCode := resp.StatusCode
				defer resp.Body.Close()

				if statusCode < 200 || statusCode >= 300 {
					stats.FailedRequests++
					if debugMode {
						statusCodeColor := getStatusCodeColor(statusCode)
						debugMsg := fmt.Sprintf("[DEBUG] Req %s status %s%d%s: %v", targetURL, statusCodeColor, statusCode, colorReset, duration)
						fmt.Fprintln(os.Stdout, debugMsg)
					}
				} else {
					stats.SuccessRequests++
					if debugMode {
						bodyBytes, _ := io.ReadAll(resp.Body) // Baca body hanya untuk menghitung byte
						stats.TotalBytes += int64(len(bodyBytes))

						if duration < stats.MinDuration {
							stats.MinDuration = duration
						}
						if duration > stats.MaxDuration {
							stats.MaxDuration = duration
						}

						statusCodeColor := getStatusCodeColor(statusCode)
						debugMsg := fmt.Sprintf("[DEBUG] Req %s status %s%d%s: %v (Bytes: %d)", targetURL, statusCodeColor, statusCode, colorReset, duration, len(bodyBytes))
						fmt.Fprintln(os.Stdout, debugMsg)
					} else {
						// Jika tidak debug, tetap perlu mengukur durasi untuk min/max
						if duration < stats.MinDuration {
							stats.MinDuration = duration
						}
						if duration > stats.MaxDuration {
							stats.MaxDuration = duration
						}
						// Opsional: Tampilkan progres singkat tanpa debug
						// fmt.Print(".")
					}
				}
			}
			mu.Unlock()
		}
	}
}

// printSimplifiedStats menampilkan hasil pengujian beban dengan format yang lebih ringkas.
func printSimplifiedStats(stats *LoadTestStats) {
	fmt.Printf("\n=== Load Testing Results ===\n")
	fmt.Printf("Total Requests: %d | Successful: %d | Failed: %d\n", stats.TotalRequests, stats.SuccessRequests, stats.FailedRequests)
	fmt.Printf("Duration: %v | Avg Response Time: %v\n", stats.TotalDuration, stats.AvgDuration)
	fmt.Printf("Min Response Time: %v | Max Response Time: %v\n", stats.MinDuration, stats.MaxDuration)
	fmt.Printf("Total Bytes Transferred: %d\n", stats.TotalBytes)

	if stats.TotalDuration.Seconds() > 0 {
		fmt.Printf("Throughput: %.2f req/s | %.2f KB/s\n",
			float64(stats.TotalRequests)/stats.TotalDuration.Seconds(),
			float64(stats.TotalBytes)/stats.TotalDuration.Seconds()/1024)
	} else {
		fmt.Printf("Throughput: N/A\n")
	}

	if stats.TotalRequests > 0 {
		successRate := (float64(stats.SuccessRequests) / float64(stats.TotalRequests)) * 100
		successColor := colorGreen
		if successRate < 50 {
			successColor = colorRed
		} else if successRate < 90 {
			successColor = colorYellow
		}
		fmt.Printf("Success Rate: %s%.2f%%%s\n", successColor, successRate, colorReset)
	}
}

