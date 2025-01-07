package transcoder

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/palzino/vidanalyser/internal/datatypes"
	"github.com/palzino/vidanalyser/internal/scanner"

	"github.com/palzino/vidanalyser/internal/db"
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

//define a list of servers here

func StartInteractiveTranscoding(background bool) {
	if background {
		// Create a log file for output
		logFile, err := os.OpenFile("transcode.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Printf("Error creating log file: %s\n", err)
			return
		}
		defer logFile.Close()

		// Redirect stdout and stderr to the log file
		log.SetOutput(logFile)
		os.Stdout = logFile
		os.Stderr = logFile

		// Detach from the terminal
		if os.Getppid() != 1 {
			args := append([]string{os.Args[0]}, os.Args[1:]...)
			cmd := exec.Command("/usr/bin/nohup", args...)
			cmd.Start()
			fmt.Println("Transcoding process started in background. Check transcode.log for progress.")
			os.Exit(0)
		}
	}

	// Query all videos from the database
	videos, err := db.QueryAllVideos()
	if err != nil {
		log.Printf("Error querying videos: %s\n", err)
		return
	}

	// Build the directory tree from the database
	directoryTree, err := db.BuildDirectoryTree()
	if err != nil {
		fmt.Printf("Error building directory tree: %s\n", err)
		return
	}
	fmt.Printf("Starting from base directory: %s\n", directoryTree.Path)

	// Ask user for output resolution, bitrate, and auto-delete preference
	var resolution string
	var maxConcurrent int
	var outputResolution string
	var outputBitrate int
	var autoDelete bool
	var minSize float64
	fmt.Print("Enter desired input resolution (e.g., 720p,1080p,4k): ")
	fmt.Scanln(&resolution)
	fmt.Print("Enter desired minimum filesize for transcoding: ")
	fmt.Scanln(&minSize)
	fmt.Print("Enter desired concurrent transcodes: ")
	fmt.Scanln(&maxConcurrent)
	fmt.Print("Enter desired output resolution (e.g., 1280x720): ")
	fmt.Scanln(&outputResolution)
	fmt.Print("Enter desired output bitrate in kbps (e.g., 3500): ")
	fmt.Scanln(&outputBitrate)
	fmt.Println("Auto delete original files after transcoding? (true/false)")
	fmt.Scanln(&autoDelete)

	// Create a filter function for eligible files
	fileFilter := func(video datatypes.VideoObject) bool {
		return float64(video.Size)/(1024*1024*1024) >= minSize && shouldTranscode(video.Width, video.Height, resolution)
	}

	// Navigate the directory tree and select files for transcoding
	selectedNode, recursive := displayDirectoryAndGetSelection(directoryTree)
	if selectedNode == nil {
		return
	}
	selectedFiles := selectedNode.FilterFiles(fileFilter, recursive)

	if len(selectedFiles) == 0 {
		fmt.Println("No files found matching the criteria")
		return
	}
	fmt.Printf("Found %d files to transcode\n", len(selectedFiles))

	// Start progress display
	go DisplayProgress(background)

	// Transcoding logic
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrent)

	for _, video := range videos {
		if containsVideo(selectedFiles, video) && fileFilter(video) {
			wg.Add(1)
			sem <- struct{}{}
			go func(video datatypes.VideoObject) {
				defer wg.Done()
				TranscodeAndRenameVideo(video, outputResolution, outputBitrate, autoDelete)
				<-sem
			}(video)
		}
	}

	wg.Wait()
	fmt.Println("All selected videos have been transcoded.")
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
		return resolutionRegex.ReplaceAllString(originalName, "zinoCoded")
	}
	ext := filepath.Ext(originalName)
	base := strings.TrimSuffix(originalName, ext)
	return fmt.Sprintf("%s_ZinoCoded%s", base, ext)
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
	if resolution == "1080p" && width == 1920 && height == 1080 {
		return true
	}
	if resolution == "720p" && width == 1280 && height == 720 {
		return true
	}
	return false
}

func TranscodeAndRenameVideo(video datatypes.VideoObject, resolution string, bitrate int, autoDelete bool) {
	newName := generateNewName(video.Name)
	outputPath := filepath.Join(video.Location, newName)

	// Get the original file size
	originalSize, err := getFileSize(video.FullFilePath)
	if err != nil {
		message := fmt.Sprintf("Error getting file size for %s: %s", video.FullFilePath, err)
		fmt.Println(message)
		utils.SendTelegramMessage(message)
		return
	}

	// Determine the encoding method based on hardware support
	var encoder string
	var scaleFilter string
	hardware := detectHardware()

	switch hardware {
	case "nvidia":
		encoder = "h264_nvenc"
		scaleFilter = fmt.Sprintf("scale_npp=%s", resolution)
	case "intel":
		encoder = "h264_qsv"
		scaleFilter = fmt.Sprintf("scale=%s", resolution) // QSV uses standard scaling
	default:
		encoder = "libx264"
		scaleFilter = fmt.Sprintf("scale=%s", resolution) // CPU uses standard scaling
	}

	// Prepare FFmpeg command with selected encoder
	ffmpegCmd := []string{
		"ffmpeg", "-y", "-i", video.FullFilePath, "-vf", scaleFilter, "-c:a", "copy",
		"-c:v", encoder, "-b:v", fmt.Sprintf("%dk", bitrate), "-nostats", "-progress", "pipe:2", outputPath,
	}

	// Add hardware acceleration flags if supported
	if hardware == "nvidia" {
		ffmpegCmd = append([]string{"ffmpeg", "-y", "-hwaccel", "cuda", "-hwaccel_output_format", "cuda"}, ffmpegCmd[2:]...)
	} else if hardware == "intel" {
		ffmpegCmd = append([]string{"ffmpeg", "-y", "-hwaccel", "qsv"}, ffmpegCmd[2:]...)
	}

	cmd := exec.Command(ffmpegCmd[0], ffmpegCmd[1:]...)

	// Print the FFmpeg command for debugging
	commandMessage := fmt.Sprintf("Running FFmpeg command: %s", strings.Join(ffmpegCmd, " "))
	fmt.Println(commandMessage)
	utils.SendTelegramMessage(commandMessage)

	// Capture stderr for progress updates
	stderr, err := cmd.StderrPipe()
	if err != nil {
		message := fmt.Sprintf("Error capturing FFmpeg stderr: %s", err)
		fmt.Println(message)
		utils.SendTelegramMessage(message)
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
	timer := time.Now()
	if err := cmd.Start(); err != nil {
		message := fmt.Sprintf("Error starting FFmpeg process: %s", err)
		fmt.Println(message)
		utils.SendTelegramMessage(message)
		return
	}

	// Goroutine to parse progress
	go parseProgress(stderr, video.Length, time.Now(), progressKey)

	// Wait for FFmpeg to finish
	if err := cmd.Wait(); err != nil {
		message := fmt.Sprintf("Error during transcoding: %s", err)
		fmt.Println(message)
		utils.SendTelegramMessage(message)
		return
	}
	timeTaken := time.Since(timer)

	// Remove progress tracking entry after completion
	progressMutex.Lock()
	delete(progressMap, progressKey)
	progressMutex.Unlock()

	// Get the new file size
	newSize, err := getFileSize(outputPath)
	if err != nil {
		message := fmt.Sprintf("Error getting file size for %s: %s", outputPath, err)
		fmt.Println(message)
		utils.SendTelegramMessage(message)
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
	scanner.ProcessFile(outputPath)
	renamedFilesMutex.Unlock()

	// Display individual file completion and updated total space saved

	newObj := datatypes.TranscodedVideo{
		OriginalVideoPath: video.FullFilePath,
		TranscodedPath:    outputPath,
		OldExtension:      filepath.Ext(video.FullFilePath),
		NewExtension:      filepath.Ext(outputPath),
		OldSize:           int(originalSize),
		NewSize:           int(newSize),
		OriginalRES:       fmt.Sprintf("%dx%d", video.Width, video.Height),
		NewRES:            resolution,
		OldBitrate:        video.Bitrate,
		NewBitrate:        bitrate,
		TimeTaken:         int(timeTaken.Seconds()),
	}
	db.InsertTranscode(newObj)

	// Display total space saved
	displaySpaceSaved() // CLI notification

	if autoDelete {
		err := os.Remove(video.FullFilePath)
		if err != nil {
			fmt.Println("Error deleting file", video.FullFilePath)
		}
		fmt.Println("file has been deleted: ", video.FullFilePath)
	}
	completionMessage := fmt.Sprintf("Transcoding completed: %s -> %s\nSpace saved for this file: %.2f GB",
		video.FullFilePath, outputPath, float64(spaceSaved)/(1024*1024*1024), "Total space saved so far: %.2f GB", float64(totalSpaceSaved)/(1024*1024*1024))
	utils.SendTelegramMessage(completionMessage)
}

func detectHardware() string {
	// Check for NVIDIA GPU support
	cmd := exec.Command("nvidia-smi")
	if err := cmd.Run(); err == nil {
		fmt.Println("NVIDIA GPU detected.")
		return "nvidia"
	}

	// Check for Intel Quick Sync Video (QSV) support
	cmd = exec.Command("vainfo")
	output, err := cmd.Output()
	if err == nil && strings.Contains(string(output), "Intel") {
		fmt.Println("Intel QSV detected.")
		return "intel"
	}

	// Default to CPU-based encoding
	fmt.Println("No hardware acceleration detected. Using CPU encoding.")
	return "cpu"
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

func DisplayProgress(background bool) {
	for {
		time.Sleep(1 * time.Second)
		progressMutex.Lock()

		if background {
			// Write progress to log
			log.Println("\n--- Current Transcoding Progress ---")
			for _, key := range progressKeys {
				if progress, exists := progressMap[key]; exists {
					log.Printf("%s | Progress: %.2f%% | Elapsed: %s | Remaining: %s\n",
						key, progress.Percentage, progress.Elapsed.Truncate(time.Second), progress.Remaining.Truncate(time.Second))
				}
			}
		} else {
			// Clear terminal and show progress
			fmt.Print("\033[H\033[2J")
			fmt.Println("Current Transcoding Progress:")
			for _, key := range progressKeys {
				if progress, exists := progressMap[key]; exists {
					fmt.Printf("%s | Progress: %.2f%% | Elapsed: %s | Remaining: %s\n",
						key, progress.Percentage, progress.Elapsed.Truncate(time.Second), progress.Remaining.Truncate(time.Second))
				}
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

// displaySpaceSaved displays the total space saved
func displaySpaceSaved() {
	spaceSavedMutex.Lock()
	defer spaceSavedMutex.Unlock()

	savedGB := float64(totalSpaceSaved) / (1024 * 1024 * 1024)
	fmt.Printf("Total space saved so far: %.2f GB\n", savedGB)
}

func StartTranscodingFromAnalysis(videos datatypes.VideoObjects, selectedDirs []string, selectedFiles []datatypes.VideoObject, recursive bool, resolution string, bitrate int, autoDelete bool) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3) // Example: max concurrent jobs = 3

	for _, video := range videos.Object {
		if IsInSelectedDirectory(video.Location, selectedDirs, recursive) || containsVideo(selectedFiles, video) {
			wg.Add(1)
			sem <- struct{}{}
			go func(video datatypes.VideoObject) {
				defer wg.Done()
				TranscodeAndRenameVideo(video, resolution, bitrate, autoDelete)
				<-sem
			}(video)
		}
	}

	wg.Wait()
	fmt.Println("All selected files have been transcoded.")
}

func NonInteractiveTranscodingByDirectory(
	directory string, minSize float64, resolution string, bitrate int, maxConcurrent int, autoDelete bool,
) error {
	// Query the database for videos
	videos, err := db.QueryVideosByDirectory(directory)
	if err != nil {
		return fmt.Errorf("error querying videos from the database: %s", err)
	}

	// Filter videos that match the requirements
	filteredVideos := []datatypes.VideoObject{}
	for _, video := range videos {
		if float64(video.Size)/(1024*1024*1024) >= minSize && // Meets size requirement
			shouldTranscode(video.Width, video.Height, resolution) { // Matches resolution
			filteredVideos = append(filteredVideos, video)
		}
	}

	if len(filteredVideos) == 0 {
		fmt.Printf("No videos found in the directory %s matching the criteria.\n", directory)
		return nil
	}

	fmt.Printf("Found %d video(s) in directory %s matching the criteria.\n", len(filteredVideos), directory)

	// Run transcoding in the background
	go func() {
		var wg sync.WaitGroup
		sem := make(chan struct{}, maxConcurrent) // Semaphore to limit concurrency

		for _, video := range filteredVideos {
			wg.Add(1)
			sem <- struct{}{}
			go func(video datatypes.VideoObject) {
				defer wg.Done()
				TranscodeAndRenameVideo(video, resolution, bitrate, autoDelete)

				// Update the database after transcoding
				newName := generateNewName(video.Name)
				outputPath := filepath.Join(video.Location, newName)
				newSize, _ := getFileSize(outputPath)

				// Update or delete video entry in the database
				if autoDelete {
					if err := db.DeleteVideo(video.FullFilePath); err != nil {
						fmt.Printf("Error deleting video %s from database: %s\n", video.FullFilePath, err)
					}
				} else {
					if err := db.UpdateVideoAfterTranscode(video.FullFilePath, outputPath, newSize); err != nil {
						fmt.Printf("Error updating video %s in database: %s\n", video.FullFilePath, err)
					}
				}
			}(video)
			<-sem
		}

		wg.Wait()
		fmt.Println("All non-interactive transcoding jobs completed successfully.")
	}()

	fmt.Println("Non-interactive transcoding job has started. Logs and progress will be saved.")
	return nil
}

func StartBackgroundTranscoding() {
	StartInteractiveTranscoding(true)
}
