package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"
	"xivi/backend/pkg/utils"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Benchmark settings
const (
	ServerHost = "localhost"
	ServerPort = 8080
	TempDir    = "./temp"
)

// Sample data for benchmarking
var sampleChannelNames = []string{
	"CNN International", "ESPN", "HBO", "Discovery Channel", "National Geographic",
	"BBC News", "Comedy Central", "Disney Channel", "MTV", "Nickelodeon",
	"Food Network", "History Channel", "Animal Planet", "Cartoon Network", "HGTV",
	"Syfy", "TLC", "Travel Channel", "USA Network", "A&E",
}

// Stubs for initialization functions that might not be exported
func init() {
	// Configure zerolog
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Create temp directory for benchmarks
	if err := os.MkdirAll(TempDir, 0755); err != nil {
		log.Error().Err(err).Msg("Failed to create temp directory")
	}

	// Initialize mock functions
	initMocks()
}

// Setup mocks for database interactions
func initMocks() {
	// Mock functions would initialize here
}

func main() {
	fmt.Println("Starting benchmark suite...")

	// Set random seed
	rand.Seed(time.Now().UnixNano())

	// Run various benchmarks
	benchmarkSingleVectorization()
	benchmarkBatchVectorization()
	benchmarkCachedVectorization()
	// benchmarkPlaylistProcessing() // Requires database
	// benchmarkM3UOperations()      // Requires real settings

	fmt.Println("Benchmark suite completed.")
}

// Benchmark single string vectorization
func benchmarkSingleVectorization() {
	fmt.Println("\n=== Benchmarking Single Vectorization ===")

	iterations := 20
	totalTime := time.Duration(0)

	// Generate random indices
	indices := make([]int, iterations)
	for i := 0; i < iterations; i++ {
		indices[i] = rand.Intn(len(sampleChannelNames))
	}

	// Run benchmarks
	for i := 0; i < iterations; i++ {
		name := sampleChannelNames[indices[i]]
		start := time.Now()

		vector, err := utils.VectorizeStringSingle(name)
		if err != nil {
			log.Error().Err(err).Msg("Error during vectorization")
			continue
		}

		elapsed := time.Since(start)
		totalTime += elapsed

		fmt.Printf("Vectorized '%s' in %v, vector size: %d\n", name, elapsed, len(vector))
	}

	fmt.Printf("Average time for single vectorization: %v\n", totalTime/time.Duration(iterations))
}

// Benchmark batch vectorization
func benchmarkBatchVectorization() {
	fmt.Println("\n=== Benchmarking Batch Vectorization ===")

	batchSizes := []int{5, 10, 20}

	for _, batchSize := range batchSizes {
		// Create batch of strings
		batch := make([]string, batchSize)
		for i := 0; i < batchSize; i++ {
			batch[i] = sampleChannelNames[rand.Intn(len(sampleChannelNames))]
		}

		// Measure batch processing time
		start := time.Now()
		vectors, err := utils.VectorizeStringBatch(batch)
		if err != nil {
			log.Error().Err(err).Msg("Error during batch vectorization")
			continue
		}
		elapsed := time.Since(start)

		fmt.Printf("Batch size %d: processed in %v (avg: %v per item)\n",
			batchSize, elapsed, elapsed/time.Duration(batchSize))

		// Compare with sequential processing
		sequentialStart := time.Now()
		for _, text := range batch {
			_, err := utils.VectorizeStringSingle(text)
			if err != nil {
				log.Error().Err(err).Msg("Error during sequential vectorization")
			}
		}
		sequentialElapsed := time.Since(sequentialStart)

		fmt.Printf("Sequential processing of %d items: %v (avg: %v per item)\n",
			batchSize, sequentialElapsed, sequentialElapsed/time.Duration(batchSize))

		fmt.Printf("Speedup: %.2fx\n", float64(sequentialElapsed)/float64(elapsed))

		// Verify the vectors are correctly shaped
		fmt.Printf("Received %d vectors\n", len(vectors))
	}
}

// Benchmark cached vectorization
func benchmarkCachedVectorization() {
	fmt.Println("\n=== Benchmarking Cached Vectorization ===")

	// First run - should be cached
	totalFirst := time.Duration(0)
	for i := 0; i < 10; i++ {
		text := sampleChannelNames[i%len(sampleChannelNames)]
		start := time.Now()
		_, err := utils.VectorizeString(text)
		if err != nil {
			log.Error().Err(err).Msg("Error during first vectorization")
			continue
		}
		elapsed := time.Since(start)
		totalFirst += elapsed
		fmt.Printf("First vectorization of '%s': %v\n", text, elapsed)
	}

	// Second run - should use cache
	totalSecond := time.Duration(0)
	for i := 0; i < 10; i++ {
		text := sampleChannelNames[i%len(sampleChannelNames)]
		start := time.Now()
		_, err := utils.VectorizeString(text)
		if err != nil {
			log.Error().Err(err).Msg("Error during second vectorization")
			continue
		}
		elapsed := time.Since(start)
		totalSecond += elapsed
		fmt.Printf("Second vectorization of '%s': %v\n", text, elapsed)
	}

	fmt.Printf("Average first run: %v\n", totalFirst/10)
	fmt.Printf("Average cached run: %v\n", totalSecond/10)
	fmt.Printf("Cache speedup: %.2fx\n", float64(totalFirst)/float64(totalSecond))
}
