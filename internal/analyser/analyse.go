package analyser

import (
	"fmt"
	"strings"

	"github.com/palzino/vidanalyser/internal/datatypes"
	"github.com/palzino/vidanalyser/internal/db"
	"github.com/palzino/vidanalyser/internal/tree"
)

// formatTime converts total seconds into days, hours, minutes, and seconds
func formatTime(totalSeconds int) (int, int, int, int) {
	days := totalSeconds / (24 * 3600)
	hours := (totalSeconds % (24 * 3600)) / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	return days, hours, minutes, seconds
}

// estimateSize estimates the file size after converting based on the specified bitrate and audio bitrate
func estimateSize(length int, videoBitrateKbps int, audioBitrateKbps int) int64 {
	videoBitrate := int64(videoBitrateKbps * 1024 / 8) // Convert kbps to bytes per second
	audioBitrate := int64(audioBitrateKbps * 1024 / 8) // Convert kbps to bytes per second
	totalBitrate := videoBitrate + audioBitrate
	return int64(length) * totalBitrate
}

// shouldTranscode determines if a video should be transcoded based on the target resolution
func shouldTranscode(width, height int, targetResolution string) bool {
	targetResolution = strings.ToLower(targetResolution)
	switch targetResolution {
	case "720p":
		return width > 1280 || height > 720
	case "1080p":
		return width > 1920 || height > 1080
	case "4k":
		return width >= 3840 || height >= 2160
	default:
		return false
	}
}

func AnalyzeDatabase() {
	// Get user input for filters
	filters := getUserFilters()

	// Build directory tree
	directoryTree, err := db.BuildDirectoryTree()
	if err != nil {
		fmt.Printf("Error building directory tree: %s\n", err)
		return
	}

	// Create filter function
	fileFilter := createFileFilter(filters)

	for {
		// Display current directory and get user selection
		selectedNode, recursive := displayDirectoryAndGetSelection(directoryTree)
		if selectedNode == nil {
			return
		}

		// Get filtered files
		selectedFiles := selectedNode.FilterFiles(fileFilter, recursive)

		// Analyze selected files
		analyzeFiles(selectedFiles, filters.targetBitrate)

		if !promptContinue() {
			break
		}
	}
}

type AnalysisFilters struct {
	minSize       float64
	resolution    string
	minDuration   int
	targetBitrate int64
}

func getUserFilters() AnalysisFilters {
	var f AnalysisFilters
	fmt.Print("Enter minimum file size in GB (or 0 for all sizes): ")
	fmt.Scanln(&f.minSize)
	fmt.Print("Enter resolution to analyse (e.g., 1920x1080, or '0' for all resolutions): ")
	fmt.Scanln(&f.resolution)
	fmt.Print("Enter minimum duration in seconds (or 0 for all durations): ")
	fmt.Scanln(&f.minDuration)
	fmt.Print("Enter desired bitrate savings estimation: ")
	fmt.Scanln(&f.targetBitrate)
	return f
}

func createFileFilter(f AnalysisFilters) func(datatypes.VideoObject) bool {
	return func(video datatypes.VideoObject) bool {
		if f.minSize > 0 && float64(video.Size)/(1024*1024*1024) < f.minSize {
			return false
		}
		if f.resolution != "0" {
			res := fmt.Sprintf("%dx%d", video.Width, video.Height)
			if res != f.resolution {
				return false
			}
		}
		if f.minDuration > 0 && video.Length < f.minDuration {
			return false
		}
		return true
	}
}

func analyzeFiles(selectedFiles []datatypes.VideoObject, targetBitrate int64) {
	totalLength := 0
	totalSize := int64(0)
	totalEstimatedSize := int64(0)
	totalSavings := int64(0)

	videos, err := db.QueryAllVideos()
	if err != nil {
		fmt.Printf("Error querying videos: %s\n", err)
		return
	}

	for _, video := range videos {
		if containsVideo(selectedFiles, video) {
			totalLength += video.Length
			totalSize += int64(video.Size)

			// Estimate transcoded size for 720p, 1.5 Mbps video + 160 kbps audio
			videoBitrate := int64(targetBitrate * 1024 * 1024 / 8) // 1.5 Mbps to bytes per second
			const audioBitrate = int64(160 * 1024 / 8)             // 160 kbps to bytes per second
			totalBitrate := videoBitrate + audioBitrate
			estimatedSize := int64(video.Length) * totalBitrate

			totalEstimatedSize += estimatedSize
			totalSavings += int64(video.Size) - estimatedSize
		}
	}

	totalSizeGB := float64(totalSize) / (1024 * 1024 * 1024)
	totalEstimatedSizeGB := float64(totalEstimatedSize) / (1024 * 1024 * 1024)
	totalSavingsGB := float64(totalSavings) / (1024 * 1024 * 1024)

	fmt.Printf("Total Selected Video Length: %d seconds\n", totalLength)
	fmt.Printf("Total Original File Size: %.2f GB\n", totalSizeGB)
	fmt.Printf("Estimated Transcoded Size: %.2f GB\n", totalEstimatedSizeGB)
	fmt.Printf("Estimated Savings: %.2f GB\n", totalSavingsGB)
}

// containsVideo checks if a video is in the selected files
func containsVideo(selectedFiles []datatypes.VideoObject, video datatypes.VideoObject) bool {
	for _, v := range selectedFiles {
		if v.FullFilePath == video.FullFilePath {
			return true
		}
	}
	return false
}

func promptContinue() bool {
	var response string
	fmt.Print("Would you like to analyze another directory? (yes/no): ")
	fmt.Scanln(&response)
	return strings.ToLower(response) == "yes"
}

func displayDirectoryAndGetSelection(tree *tree.DirectoryNode) (*tree.DirectoryNode, bool) {
	fmt.Printf("\nCurrent directory: %s\n", tree.Path)
	fmt.Println("[1] Select files in this directory only")
	fmt.Println("[2] Select files in this directory and subdirectories")
	fmt.Println("[q] Quit")

	var input string
	fmt.Print("Enter choice: ")
	fmt.Scanln(&input)

	if input == "q" {
		return nil, false
	}
	if input == "1" {
		return tree, false
	}
	if input == "2" {
		return tree, true
	}

	return tree, false
}
