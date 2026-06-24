package main

import (
	"fmt"     // fmt tetap dibutuhkan untuk Printf
	"io"      // io tetap dibutuhkan untuk ReadAll
	"net/http" // net/http tetap dibutuhkan untuk http.Client dan http.NewRequest
	// "net/url" // Tidak digunakan dalam kode ini, jadi bisa dihapus atau tetap jika untuk validasi di masa depan.
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

func main() {
	// --- Input Parameter Manual untuk SEMUA Parameter ---
	targetURL := "http://localhost:8080"      // Ganti dengan URL target Anda
	concurrency := 10                         // Jumlah permintaan bersamaan
	duration := 30 * time.Second              // Durasi pengujian
	timeout := 10 * time.Second               // Batas waktu permintaan
	method := "GET"                           // Metode HTTP (GET, POST, dll.)
	// ----------------------------------------------------

	// --- Banner CPA dengan Warna Cyan ---
	// Gunakan tanda kutip terbalik (`) untuk string multi-baris
	bannerCPA := `
   ______      ________
  / ____/___  / ____/ /______
 / / __/ __ \/ / __/ __/ ___/
/ /_/ / /_/ / /_/ / /_/ /
\____/\____/\____/\__/_/
`
	// Cetak banner
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
		MinDuration: time.Hour,
	}
	var mu sync.Mutex

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: timeout, // Gunakan variabel timeout manual
	}

	// Channel to control test duration
	stopChan := make(chan struct{})
	var wg sync.WaitGroup

	// Start timer
	startTime := time.Now()

	// Schedule stop after duration
	go func() {
		time.Sleep(duration) // Gunakan variabel duration manual
		close(stopChan)
	}()

	// Launch concurrent workers
	for i := 0; i < concurrency; i++ { // Gunakan variabel concurrency manual
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Panggil worker dengan semua parameter yang sudah didefinisikan manual
			// Pastikan parameter yang diteruskan sesuai dengan definisi fungsi worker
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

func worker(client *http.Client, targetURL, method string, stopChan chan struct{}, stats *LoadTestStats, mu *sync.Mutex) {
	for {
		select {
		case <-stopChan:
			return
		default:
			// Perform request
			startTime := time.Now()
			req, _ := http.NewRequest(method, targetURL, nil) // Menggunakan method dan targetURL manual
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
