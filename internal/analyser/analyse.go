package analyser

import (
	"fmt"
	"strings"

	"github.com/palzino/vidanalyser/internal/datatypes"
	"github.com/palzino/vidanalyser/internal/db"
	"github.com/palzino/vidanalyser/internal/transcoder"
	"github.com/palzino/vidanalyser/internal/utils"
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
	// Ask the user for filtering options
	var minSize float64
	var resolution string
	var minDuration int

	fmt.Print("Enter minimum file size in GB (or 0 for all sizes): ")
	fmt.Scanln(&minSize)
	fmt.Print("Enter desired resolution (e.g., 1920x1080, or 'all' for all resolutions): ")
	fmt.Scanln(&resolution)
	fmt.Print("Enter minimum duration in seconds (or 0 for all durations): ")
	fmt.Scanln(&minDuration)

	directoryTree, baseDir, err := db.BuildDirectoryTreeFromDatabase()
	if err != nil {
		fmt.Printf("Error building directory tree: %s\n", err)
		return
	}
	fmt.Printf("Starting from base directory: %s\n", baseDir)

	currentPath := baseDir

	rawVideos, err := db.QueryAllVideos()
	if err != nil {
		fmt.Printf("Error querying all videos: %s\n", err)
		return
	}

	// Wrap the raw videos into the VideoObjects struct
	videos := datatypes.VideoObjects{Object: rawVideos}

	// Determine if any filter has been applied
	filterApplied := minSize > 0 || resolution != "all" || minDuration > 0

	// Define a filter function based on user input
	fileFilter := func(video datatypes.VideoObject) bool {
		if minSize > 0 && float64(video.Size)/(1024*1024*1024) < minSize {
			return false
		}
		if resolution != "all" {
			res := fmt.Sprintf("%dx%d", video.Width, video.Height)
			if res != resolution {
				return false
			}
		}
		if minDuration > 0 && video.Length < minDuration {
			return false
		}
		return true
	}

	// Traverse directories and select files for analysis

	for {
		selectedDirs, selectedFiles, recursive := utils.DisplayDirectoryTree(directoryTree, currentPath, baseDir, videos, fileFilter)

		// If the user chose to quit
		if selectedDirs == nil && selectedFiles == nil && !recursive {
			fmt.Println("Exiting analysis.")
			return
		}

		// Perform analysis on selected files
		analyzeSelectedFilesFromDatabase(selectedDirs, selectedFiles, recursive)

		// Option to transcode
		fmt.Print("Would you like to transcode the analyzed files? (yes/no): ")
		var choice string
		fmt.Scanln(&choice)

		if choice == "yes" {
			var transcodeResolution string
			var transcodeBitrate int
			var autoDelete bool
			fmt.Print("Enter desired resolution for transcoding (e.g., 1280x720): ")
			fmt.Scanln(&transcodeResolution)
			fmt.Print("Enter desired bitrate in kbps (e.g., 3500): ")
			fmt.Scanln(&transcodeBitrate)
			fmt.Println("Auto delete? true/false")
			fmt.Scanln(&autoDelete)

			transcoder.StartTranscodingFromAnalysis(videos, selectedDirs, selectedFiles, recursive, transcodeResolution, transcodeBitrate, autoDelete)
			break
		} else if choice == "no" {
			if !filterApplied {
				fmt.Print("Would you like to perform another analysis? (yes/no): ")
				fmt.Scanln(&choice)
				if choice != "yes" {
					fmt.Println("Exiting analysis.")
					return
				}
			} else {
				fmt.Println("Exiting analysis.")
				return
			}
		} else {
			fmt.Println("Invalid input. Please enter 'yes' or 'no'.")
		}
	}
}

func analyzeSelectedFilesFromDatabase(selectedDirs []string, selectedFiles []datatypes.VideoObject, recursive bool) {
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
		if db.IsInSelectedDirectory(video.Location, selectedDirs, recursive) || containsVideo(selectedFiles, video) {
			totalLength += video.Length
			totalSize += int64(video.Size)

			// Estimate transcoded size for 720p, 1.5 Mbps video + 160 kbps audio
			const videoBitrate = int64(1.5 * 1024 * 1024 / 8) // 1.5 Mbps to bytes per second
			const audioBitrate = int64(160 * 1024 / 8)        // 160 kbps to bytes per second
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
