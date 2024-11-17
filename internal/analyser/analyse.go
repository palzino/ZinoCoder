package analyser

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/palzino/vidanalyser/internal/datatypes"
)

// formatTime converts total seconds into days, hours, minutes, and seconds
func formatTime(totalSeconds int) (int, int, int, int) {
	days := totalSeconds / (24 * 3600)
	hours := (totalSeconds % (24 * 3600)) / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	return days, hours, minutes, seconds
}

// estimate720pSize estimates the file size after converting to 720p (1.5 Mbps video + 160 kbps audio)
func estimate720pSize(length int) int64 {
	const videoBitrate = int64(1.5 * 1024 * 1024 / 8) // 1.5 Mbps to bytes per second
	const audioBitrate = int64(160 * 1024 / 8)        // 160 kbps to bytes per second
	totalBitrate := videoBitrate + audioBitrate
	return int64(length) * totalBitrate
}

func AnalyzeJSON(filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println("Error opening JSON file:", err)
		return
	}
	defer file.Close()

	var videoObjects datatypes.VideoObjects
	err = json.NewDecoder(file).Decode(&videoObjects)
	if err != nil {
		fmt.Println("Error decoding JSON data:", err)
		return
	}

	totalVideos := len(videoObjects.Object)
	totalLength := 0
	smallvid := int64(0)
	smallsize := int64(0)
	totalSize := int64(0)
	totalEstimatedSize := int64(0)
	totalSavings := int64(0)
	resolutionCount := make(map[string]int)

	// Analyze video data
	for _, video := range videoObjects.Object {
		totalLength += video.Length
		totalSize += int64(video.Size)

		// Create resolution string (e.g., "1920x1080")
		resolution := fmt.Sprintf("%dx%d", video.Width, video.Height)
		resolutionCount[resolution]++
		if video.Length < 15 {
			smallvid = smallvid + 1
			smallsize += int64(video.Size)
		}

		// Check if the video is 4K or 1080p and above 2.7 GB (2.7 * 1024 * 1024 * 1024 bytes)
		if (video.Width >= 3840 || (video.Width >= 1920 && video.Width < 3840)) && int64(video.Size) > int64(2700*1024*1024) {
			estimatedSize := estimate720pSize(video.Length)
			totalEstimatedSize += estimatedSize
			savings := int64(video.Size) - estimatedSize
			totalSavings += savings
		}
	}
	// Convert total size to gigabytes
	totalSizeGB := float64(totalSize) / (1024 * 1024 * 1024)
	smallGB := float64(smallsize) / (1024 * 1024 * 1024)
	totalEstimatedSizeGB := float64(totalEstimatedSize) / (1024 * 1024 * 1024)
	totalSavingsGB := float64(totalSavings) / (1024 * 1024 * 1024)

	// Format total video length
	days, hours, minutes, seconds := formatTime(totalLength)

	// Display results
	fmt.Printf("Total Videos: %d\n", totalVideos)
	fmt.Printf("Total Video Length: %d days, %d hours, %d minutes, %d seconds\n", days, hours, minutes, seconds)
	fmt.Printf("Total File Size: %.2f GB\n", totalSizeGB)
	fmt.Printf("Total SMALL File Size: %.2f GB\n", smallGB)
	fmt.Printf("Estimated Transcoded Size: %.2f GB\n", totalEstimatedSizeGB)
	fmt.Printf("Total Estimated Savings: %.2f GB\n", totalSavingsGB)
	fmt.Printf("small videos: %d", smallvid)
	// Display resolution count
	//fmt.Println("\nResolution Count:")
	//for resolution, count := range resolutionCount {
	//      fmt.Printf("%s: %d videos\n", resolution, count)
	//}
}
