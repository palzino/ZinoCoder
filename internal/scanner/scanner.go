package scanner

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/palzino/vidanalyser/internal/datatypes"
)

var videoExtensions = map[string]bool{
	".mp4":  true,
	".mkv":  true,
	".avi":  true,
	".mov":  true,
	".m4v":  true,
	".webm": true,
}

var videoObjects datatypes.VideoObjects
var totalVideos int
var mu sync.Mutex
var wg sync.WaitGroup

// checkExtension checks if the file has a video extension
func checkExtension(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return videoExtensions[ext]
}

// getFileSize returns the size of the file in bytes
func getFileSize(filePath string) int64 {
	info, err := os.Stat(filePath)
	if err != nil {
		fmt.Println("Error getting file size:", err)
		return 0
	}
	return info.Size()
}
func getVideoMetadata(filePath string) (int, int, int, float64, int, int) {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".mp4", ".mov", ".m4v", ".avi":
		return getMP4Metadata(filePath)
	case ".mkv":
		return getMKVMetadata(filePath)
	default:
		fmt.Println("Unsupported file type:", ext)
		return 0, 0, 0, 0.0, 0, 0
	}
}

// getMP4Metadata uses ffprobe to extract video metadata
func getMP4Metadata(filePath string) (int, int, int, float64, int, int) {
	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height,avg_frame_rate,nb_frames,bit_rate,duration",
		"-of", "csv=p=0", filePath)
	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		fmt.Println("Error running ffprobe:", err)
		return 0, 0, 0, 0.0, 0, 0
	}

	width, height, duration := 0, 0, 0
	framerate := 0.0
	frames, bitrate := 0, 0

	scanner := bufio.NewScanner(&out)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ",")
		if len(parts) >= 6 {
			width, _ = strconv.Atoi(parts[0])
			height, _ = strconv.Atoi(parts[1])
			framerate = parseFramerate(parts[2]) // Handle framerate as a fraction
			durationFloat, _ := strconv.ParseFloat(parts[3], 64)
			duration = int(durationFloat)
			frames, _ = strconv.Atoi(parts[4])
			bitrate, _ = strconv.Atoi(parts[5])
		}
	}
	return width, height, duration, framerate, frames, bitrate
}

// getMKVMetadata extracts metadata for MKV files
func getMKVMetadata(filePath string) (int, int, int, float64, int, int) {
	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height,avg_frame_rate",
		"-show_entries", "format=duration,bit_rate", "-of", "csv=p=0", filePath)
	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		fmt.Println("Error running ffprobe for MKV:", err)
		return 0, 0, 0, 0.0, 0, 0
	}

	width, height := 0, 0
	duration := 0
	framerate := 0.0
	bitrate := 0

	scanner := bufio.NewScanner(&out)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ",")
		if len(parts) >= 5 {
			width, _ = strconv.Atoi(parts[0])
			height, _ = strconv.Atoi(parts[1])
			framerate = parseFramerate(parts[2])

			// Handle duration (from format section)
			if parts[3] != "N/A" {
				durationFloat, _ := strconv.ParseFloat(parts[3], 64)
				duration = int(durationFloat)
			}

			// Handle bitrate (from format section)
			if parts[4] != "N/A" {
				bitrate, _ = strconv.Atoi(parts[4])
			}
		}
	}
	return width, height, duration, framerate, 0, bitrate // MKV does not reliably provide nb_frames
}

// parseFramerate converts a fraction string like "30000/1001" to a float
func parseFramerate(fps string) float64 {
	parts := strings.Split(fps, "/")
	if len(parts) == 2 {
		numerator, err1 := strconv.ParseFloat(parts[0], 64)
		denominator, err2 := strconv.ParseFloat(parts[1], 64)
		if err1 == nil && err2 == nil && denominator != 0 {
			return numerator / denominator
		}
	}
	// If it's not a fraction, attempt to parse as a float
	if framerate, err := strconv.ParseFloat(fps, 64); err == nil {
		return framerate
	}
	return 0.0
}

// processFile extracts metadata from a video file and adds it to the list
func processFile(filePath string) {
	fileSize := getFileSize(filePath)
	width, height, length, framerate, frames, bitrate := getVideoMetadata(filePath)

	mu.Lock()
	defer mu.Unlock()
	totalVideos++

	obj := datatypes.VideoObject{
		Name:         filepath.Base(filePath),
		Location:     filepath.Dir(filePath),
		FullFilePath: filePath,
		Size:         int(fileSize),
		Width:        width,
		Height:       height,
		Length:       length,
		Framerate:    framerate,
		Frames:       frames,
		Bitrate:      bitrate,
	}
	videoObjects.Object = append(videoObjects.Object, obj)
}

// processDirectory scans a directory for video files
func ProcessDirectory(directory string) {
	defer wg.Done()
	err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println("Error walking path:", err)
			return err
		}
		if !info.IsDir() && checkExtension(info.Name()) {
			processFile(path)
		}
		return nil
	})
	if err != nil {
		fmt.Println("Error processing directory:", err)
	}
}

// GetTotalVideos returns the total number of processed videos
func GetTotalVideos() int {
	mu.Lock()
	defer mu.Unlock()
	return totalVideos
}

// ProcessMasterDirectory now returns a WaitGroup for synchronization
func ProcessMasterDirectory(masterFolder string) *sync.WaitGroup {
	var wg sync.WaitGroup

	files, err := os.ReadDir(masterFolder)
	if err != nil {
		fmt.Println("Error reading master folder:", err)
		return &wg
	}

	// Process files in master directory
	for _, file := range files {
		if !file.IsDir() && checkExtension(file.Name()) {
			filePath := filepath.Join(masterFolder, file.Name())
			processFile(filePath)
		}
	}

	// Process subdirectories
	for _, subdir := range files {
		if subdir.IsDir() {
			wg.Add(1)
			go ProcessDirectory(filepath.Join(masterFolder, subdir.Name()))
		}
	}

	return &wg
}

// saveToJSON saves the video metadata to a JSON file
func SaveToJSON(filename string) {
	file, err := os.Create(filename)
	if err != nil {
		fmt.Println("Error creating JSON file:", err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(videoObjects)
	if err != nil {
		fmt.Println("Error writing JSON data:", err)
	}
}
