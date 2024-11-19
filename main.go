package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/palzino/vidanalyser/internal/analyser"
	"github.com/palzino/vidanalyser/internal/scanner"
	"github.com/palzino/vidanalyser/internal/transcoder"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <command> <path>")
		return
	}

	command := os.Args[1]
	path := os.Args[2]

	switch command {
	case "scan":
		wg := scanner.ProcessMasterDirectory(path)
		wg.Wait()
		fmt.Printf("Total video files: %d\n", scanner.GetTotalVideos())
		scanner.SaveToJSON("video_metadata.json")
		fmt.Println("Metadata saved to video_metadata.json")

	case "analyse":
		analyser.AnalyzeJSONWithDirectoryTraversal(path)

	case "transcode":
		minSize, _ := strconv.ParseFloat(os.Args[3], 64)
		resolution := os.Args[4]
		maxConcurrent, _ := strconv.Atoi(os.Args[5])
		transcoder.StartInteractiveTranscoding("video_metadata.json", minSize, resolution, maxConcurrent)

	default:
		fmt.Println("Unknown command. Use 'scan', 'analyse', or 'transcode'.")
	}
}
