package main

import (
	"bufio"    // Diperlukan untuk membaca input dari terminal
	"fmt"
	"io"
	"net/http"
	"os"       // Diperlukan untuk os.Stdin
	"strings"  // Diperlukan untuk memanipulasi string input
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
	colorCyan  = "\033[36m"
	colorReset = "\033[0m"
)

// Fungsi helper untuk meminta input dari pengguna dengan prompt
func getUserInput(prompt string, defaultValue string) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s (default: %s): ", prompt, defaultValue)
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("Error reading input: %v\n", err)
		return defaultValue // Kembali ke default jika ada error
	}
	// Hapus karakter newline dari input
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue // Gunakan default jika input kosong
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
	val, err := strconv.Atoi(input) // Gunakan strconv untuk konversi ke integer
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


func main() {
	// --- Ambil Input Parameter dari Pengguna secara Interaktif ---

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
	// Konversi ke uppercase untuk konsistensi
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
	fmt.Printf("Target URL: %s\n", targetURL)
	fmt.Printf("Concurrency: %d\n", concurrency)
	fmt.Printf("Duration: %v\n", duration)
	fmt.Printf("Timeout: %v\n", timeout)
	fmt.Printf("Method: %s\n\n", method)

	// Initialize stats
	stats := &LoadTestStats{
		MinDuration: time.Hour, // Inisialisasi MinDuration ke nilai yang sangat besar
	}
	var mu sync.Mutex // Mutex untuk melindungi akses ke stats

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: timeout, // Gunakan timeout manual
	}

	// Channel to control test duration
	stopChan := make(chan struct{})
	var wg sync.WaitGroup

	// Start timer
	startTime := time.Now()

	// Schedule stop after duration
	go func() {
		time.Sleep(duration) // Gunakan duration manual
		close(stopChan)      // Kirim sinyal untuk menghentikan worker
	}()

	// Launch concurrent workers
	for i := 0; i < concurrency; i++ { // Gunakan concurrency manual
		wg.Add(1)
		go func() {
			defer wg.Done() // Pastikan wg.Done() dipanggil saat goroutine selesai
			worker(client, targetURL, method, stopChan, stats, &mu)
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

// Fungsi worker melakukan satu request HTTP (sama seperti sebelumnya)
func worker(client *http.Client, targetURL, method string, stopChan chan struct{}, stats *LoadTestStats, mu *sync.Mutex) {
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
				mu.Unlock()
				continue
			}
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
			defer resp.Body.Close()

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				stats.FailedRequests++
			} else {
				stats.SuccessRequests++
				body, _ := io.ReadAll(resp.Body)
				stats.TotalBytes += int64(len(body))

				if duration < stats.MinDuration {
					stats.MinDuration = duration
				}
				if duration > stats.MaxDuration {
					stats.MaxDuration = duration
				}
			}
			mu.Unlock()
		}
	}
}

// Fungsi printStats untuk menampilkan hasil (sama seperti sebelumnya)
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
