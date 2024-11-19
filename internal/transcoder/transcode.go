package transcoder

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/palzino/vidanalyser/internal/datatypes"
	"github.com/palzino/vidanalyser/internal/utils"
)

type RenamedFile struct {
	OriginalName string `json:"original_name"`
	NewName      string `json:"new_name"`
	OriginalSize int64  `json:"original_size"`
	NewSize      int64  `json:"new_size"`
}
type Progress struct {
	Percentage float64
	Elapsed    time.Duration
	Remaining  time.Duration
}

var progressMap = make(map[string]*Progress)
var progressKeys []string
var progressMutex sync.Mutex

var renamedFiles []RenamedFile
var renamedFilesMutex sync.Mutex
var totalSpaceSaved int64
var spaceSavedMutex sync.Mutex

// BuildDirectoryTree creates a nested map representing the directory structure from the video metadata.

// StartInteractiveTranscoding handles the transcoding process based on user selections.
func StartInteractiveTranscoding(jsonPath string, minSize float64, resolution string, maxConcurrent int) {
	file, _ := os.Open(jsonPath)
	defer file.Close()

	var videos datatypes.VideoObjects
	json.NewDecoder(file).Decode(&videos)

	// Ask user for output resolution and bitrate
	var outputResolution string
	var outputBitrate int
	fmt.Print("Enter desired output resolution (e.g., 1280x720): ")
	fmt.Scanln(&outputResolution)
	fmt.Print("Enter desired output bitrate in kbps (e.g., 3500): ")
	fmt.Scanln(&outputBitrate)

	// Determine the base directory automatically from the JSON metadata
	baseDir := FindCommonBaseDir(videos)
	fmt.Printf("Starting from base directory: %s\n", baseDir)

	// Build the directory tree from metadata and start from the determined base directory
	directoryTree := utils.BuildDirectoryTree(videos)

	// Create a filter function for eligible files
	fileFilter := func(video datatypes.VideoObject) bool {
		return float64(video.Size)/(1024*1024*1024) >= minSize && shouldTranscode(video.Width, video.Height, resolution)
	}

	// Use the fileFilter in DisplayDirectoryTree
	selectedDirs, selectedFiles, recursive := utils.DisplayDirectoryTree(directoryTree, baseDir, videos, fileFilter)

	// Start progress display
	go DisplayProgress()

	// Transcoding logic
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrent)

	for _, video := range videos.Object {
		if (IsInSelectedDirectory(video.Location, selectedDirs, recursive) || containsVideo(selectedFiles, video)) &&
			fileFilter(video) {

			wg.Add(1)
			sem <- struct{}{}
			go func(video datatypes.VideoObject) {
				defer wg.Done()
				TranscodeAndRenameVideo(video, outputResolution, outputBitrate)
				<-sem
			}(video)
		}
	}

	wg.Wait()
	saveRenamedFilesToJSON("renamed_files.json")
}
func FindCommonBaseDir(videos datatypes.VideoObjects) string {
	if len(videos.Object) == 0 {
		return "/"
	}

	// Start with the first video's directory
	commonBaseDir := filepath.Dir(videos.Object[0].FullFilePath)

	// Iterate over all videos to find the common base directory
	for _, video := range videos.Object {
		videoDir := filepath.Dir(video.FullFilePath)
		for !strings.HasPrefix(videoDir, commonBaseDir) {
			// Move up one level in the common base directory
			commonBaseDir = filepath.Dir(commonBaseDir)
			if commonBaseDir == "/" {
				return commonBaseDir
			}
		}
	}

	return commonBaseDir
}
func containsVideo(selectedFiles []datatypes.VideoObject, video datatypes.VideoObject) bool {
	for _, v := range selectedFiles {
		if v.FullFilePath == video.FullFilePath {
			return true
		}
	}
	return false
}

func generateNewName(originalName string) string {
	resolutionRegex := regexp.MustCompile(`(?i)(4k|2160p|1080p|720p)`)
	if resolutionRegex.MatchString(originalName) {
		return resolutionRegex.ReplaceAllString(originalName, "720p")
	}
	ext := filepath.Ext(originalName)
	base := strings.TrimSuffix(originalName, ext)
	return fmt.Sprintf("%s_720p%s", base, ext)
}

func IsInSelectedDirectory(location string, selectedDirs []string, recursive bool) bool {
	for _, dir := range selectedDirs {
		if recursive {
			if strings.HasPrefix(location, dir) {
				return true
			}
		} else {
			if filepath.Dir(location) == dir {
				return true
			}
		}
	}
	return false
}

func shouldTranscode(width, height int, resolution string) bool {
	if resolution == "4k" && width >= 3840 && height >= 2160 {
		return true
	}
	if resolution == "1080p" && width >= 1920 && height >= 1080 {
		return true
	}
	return false
}

func TranscodeAndRenameVideo(video datatypes.VideoObject, resolution string, bitrate int) {
	newName := generateNewName(video.Name)
	outputPath := filepath.Join(video.Location, newName)

	// Get the original file size
	originalSize, err := getFileSize(video.FullFilePath)
	if err != nil {
		fmt.Printf("Error getting file size for %s: %s\n", video.FullFilePath, err)
		return
	}

	// Prepare FFmpeg command with user-specified resolution and bitrate
	ffmpegCmd := []string{
		"ffmpeg", "-y", "-hwaccel", "cuda", "-hwaccel_output_format", "cuda",
		"-i", video.FullFilePath, "-vf", fmt.Sprintf("scale_npp=%s", resolution), "-c:a", "copy",
		"-c:v", "h264_nvenc", "-b:v", fmt.Sprintf("%dk", bitrate), "-nostats", "-progress", "pipe:2", outputPath,
	}
	cmd := exec.Command(ffmpegCmd[0], ffmpegCmd[1:]...)

	// Print the FFmpeg command for debugging
	fmt.Printf("Running FFmpeg command: %s\n", strings.Join(ffmpegCmd, " "))

	// Capture stderr for progress updates
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Printf("Error capturing FFmpeg stderr: %s\n", err)
		return
	}

	// Initialize progress tracking
	progressKey := video.FullFilePath
	progressMutex.Lock()
	if _, exists := progressMap[progressKey]; !exists {
		progressMap[progressKey] = &Progress{}
		progressKeys = append(progressKeys, progressKey) // Maintain order
	}
	progressMutex.Unlock()

	// Start the FFmpeg process
	if err := cmd.Start(); err != nil {
		fmt.Printf("Error starting FFmpeg process: %s\n", err)
		return
	}

	// Goroutine to parse progress
	go parseProgress(stderr, video.Length, time.Now(), progressKey)

	// Wait for FFmpeg to finish
	if err := cmd.Wait(); err != nil {
		fmt.Printf("\nError during transcoding: %s\n", err)
		return
	}

	// Remove progress tracking entry after completion
	progressMutex.Lock()
	delete(progressMap, progressKey)
	progressMutex.Unlock()

	// Get the new file size
	newSize, err := getFileSize(outputPath)
	if err != nil {
		fmt.Printf("Error getting file size for %s: %s\n", outputPath, err)
		return
	}

	// Calculate space saved
	spaceSaved := originalSize - newSize

	// Update the total space saved
	spaceSavedMutex.Lock()
	totalSpaceSaved += spaceSaved
	spaceSavedMutex.Unlock()

	// Record the renamed file
	renamedFilesMutex.Lock()
	renamedFiles = append(renamedFiles, RenamedFile{
		OriginalName: video.FullFilePath,
		NewName:      outputPath,
		OriginalSize: originalSize,
		NewSize:      newSize,
	})
	renamedFilesMutex.Unlock()

	// Display individual file completion and updated total space saved
	fmt.Printf("\nTranscoding completed: %s -> %s\n", video.FullFilePath, outputPath)
	fmt.Printf("Space saved for this file: %.2f GB\n", float64(spaceSaved)/(1024*1024*1024))
	displaySpaceSaved() // Recalculate and display the total space saved
	fmt.Printf("Done! Transcode completed successfully.\n")
}

func parseProgress(stderr io.ReadCloser, totalDuration int, startTime time.Time, key string) {
	progressRegex := regexp.MustCompile(`out_time=(\d+:\d+:\d+\.\d+)`)

	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()

		if matches := progressRegex.FindStringSubmatch(line); matches != nil {
			currentTimeStr := matches[1]
			currentTime := parseTimestamp(currentTimeStr)

			// Calculate progress percentage
			progress := float64(currentTime) / float64(totalDuration) * 100

			// Calculate elapsed time and remaining time
			elapsed := time.Since(startTime)
			remaining := time.Duration(float64(elapsed) * (100/progress - 1))

			// Update progress map
			progressMutex.Lock()
			progressMap[key] = &Progress{
				Percentage: progress,
				Elapsed:    elapsed,
				Remaining:  remaining,
			}
			progressMutex.Unlock()
		}
	}
}

func DisplayProgress() {
	for {
		time.Sleep(1 * time.Second)
		progressMutex.Lock()
		fmt.Print("\033[H\033[2J") // Clear the terminal
		fmt.Println("Current Transcoding Progress:")

		// Iterate over progressKeys to maintain a fixed order
		for _, key := range progressKeys {
			if progress, exists := progressMap[key]; exists {
				fmt.Printf("%s | Progress: %.2f%% | Elapsed: %s | Remaining: %s\n",
					key, progress.Percentage, progress.Elapsed.Truncate(time.Second), progress.Remaining.Truncate(time.Second))
			}
		}

		progressMutex.Unlock()
	}
}

func parseTimestamp(timestamp string) int {
	parts := strings.Split(timestamp, ":")
	if len(parts) != 3 {
		return 0
	}

	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])
	seconds, _ := strconv.ParseFloat(parts[2], 64)

	return int(hours*3600 + minutes*60 + int(seconds))
}

func getFileSize(filePath string) (int64, error) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return 0, err
	}
	return fileInfo.Size(), nil
}

func saveRenamedFilesToJSON(filename string) {
	renamedFilesMutex.Lock()
	defer renamedFilesMutex.Unlock()

	file, err := os.Create(filename)
	if err != nil {
		fmt.Printf("Error creating JSON file: %s\n", err)
		return
	}
	defer file.Close()

	json.NewEncoder(file).Encode(renamedFiles)
	fmt.Printf("Renamed files saved to %s\n", filename)
}

// displaySpaceSaved displays the total space saved
func displaySpaceSaved() {
	spaceSavedMutex.Lock()
	defer spaceSavedMutex.Unlock()

	savedGB := float64(totalSpaceSaved) / (1024 * 1024 * 1024)
	fmt.Printf("Total space saved so far: %.2f GB\n", savedGB)
}
func StartTranscodingFromAnalysis(videos datatypes.VideoObjects, selectedDirs []string, selectedFiles []datatypes.VideoObject, recursive bool, resolution string, bitrate int) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3) // Example: max concurrent jobs = 3

	for _, video := range videos.Object {
		if IsInSelectedDirectory(video.Location, selectedDirs, recursive) || containsVideo(selectedFiles, video) {
			wg.Add(1)
			sem <- struct{}{}
			go func(video datatypes.VideoObject) {
				defer wg.Done()
				TranscodeAndRenameVideo(video, resolution, bitrate)
				<-sem
			}(video)
		}
	}

	wg.Wait()
	fmt.Println("All selected files have been transcoded.")
}
