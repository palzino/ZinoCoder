package main

import (
	"fmt"
	"os"

	"github.com/palzino/vidanalyser/internal/analyser"
	"github.com/palzino/vidanalyser/internal/scanner"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <command> <path>")
		return
	}

	command := os.Args[0]
	path := os.Args[1]

	switch command {
	case "scan":
		// Process the directory and get WaitGroup
		wg := scanner.ProcessMasterDirectory(path)

		// Wait for all goroutines to complete
		wg.Wait()

		// Get and print total videos
		fmt.Printf("Total video files: %d\n", scanner.GetTotalVideos())

		// Save video objects to JSON file
		scanner.SaveToJSON("video_metadata.json")
		fmt.Println("Metadata saved to video_metadata.json")

	case "analyze":
		analyser.AnalyzeJSON(path)

	default:
		fmt.Println("Unknown command. Use 'scan' or 'analyze'.")
	}
}
