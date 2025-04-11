package transcoder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/palzino/vidanalyser/internal/datatypes"
	"github.com/palzino/vidanalyser/internal/db"
	"github.com/palzino/vidanalyser/internal/scanner"
	"github.com/palzino/vidanalyser/internal/utils"
)

// Request payload structure
type TranscodeRequest struct {
	Video       datatypes.VideoObject `json:"video"`
	Resolution  string                `json:"resolution"`
	Bitrate     int                   `json:"bitrate"`
	AutoDelete  bool                  `json:"autoDelete"`
	CallbackURL string                `json:"callbackURL"` // The URL to notify on completion
}

// Handle the transcoding request
func handleTranscode(w http.ResponseWriter, r *http.Request) {
	// Only allow POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method. Only POST is allowed.", http.StatusMethodNotAllowed)
		return
	}

	// Parse the JSON body
	var req TranscodeRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request body: %s", err), http.StatusBadRequest)
		return
	}

	// Validate the input
	if req.Resolution == "" || req.Bitrate <= 0 || req.Video.FullFilePath == "" {
		http.Error(w, "Invalid input parameters.", http.StatusBadRequest)
		return
	}

	// Perform transcoding
	go func() {
		APITranscode(req.Video, req.Resolution, req.Bitrate, req.AutoDelete, req.CallbackURL)
	}()

	// Respond to the client
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("Transcoding job accepted and started."))
}

// Handle cleanup of old transcoded file entries
func handleCleanTranscoded(w http.ResponseWriter, r *http.Request) {
	// Only allow POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method. Only POST is allowed.", http.StatusMethodNotAllowed)
		return
	}

	// Clean up transcoded entries from the database
	count, err := db.CleanTranscodedEntries()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error cleaning transcoded entries: %s", err), http.StatusInternalServerError)
		return
	}

	// Return success response
	response := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Successfully removed %d transcoded file entries from the database.", count),
		"count":   count,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func TranscodeServer() {
	// Define the route for the transcoding endpoint
	http.HandleFunc("/transcode", handleTranscode)

	// Add the endpoint for cleaning transcoded entries
	http.HandleFunc("/clean-transcoded", handleCleanTranscoded)

	// Add a simple dashboard
	http.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		html := `
		<!DOCTYPE html>
		<html>
		<head>
			<title>Video Analyser Dashboard</title>
			<style>
				body {
					font-family: Arial, sans-serif;
					max-width: 800px;
					margin: 0 auto;
					padding: 20px;
				}
				.button {
					background-color: #4CAF50;
					border: none;
					color: white;
					padding: 10px 20px;
					text-align: center;
					text-decoration: none;
					display: inline-block;
					font-size: 16px;
					margin: 10px 2px;
					cursor: pointer;
					border-radius: 4px;
				}
				.info {
					background-color: #f0f0f0;
					padding: 15px;
					border-radius: 4px;
					margin-bottom: 20px;
				}
				#result {
					margin-top: 20px;
					padding: 10px;
					background-color: #f9f9f9;
					border-radius: 4px;
					display: none;
				}
			</style>
		</head>
		<body>
			<h1>Video Analyser Dashboard</h1>
			
			<div class="info">
				<p>Use this dashboard to manage your transcoded videos and database.</p>
			</div>
			
			<h2>Database Maintenance</h2>
			<p>After transcoding videos, you can clean up the database by removing entries for original files that have been transcoded to ZinoCoded versions.</p>
			<button id="cleanBtn" class="button">Clean Transcoded Files from Database</button>
			
			<div id="result"></div>
			
			<script>
				document.getElementById('cleanBtn').addEventListener('click', function() {
					fetch('/clean-transcoded', {
						method: 'POST',
					})
					.then(response => response.json())
					.then(data => {
						const resultDiv = document.getElementById('result');
						resultDiv.style.display = 'block';
						resultDiv.innerHTML = '<strong>' + data.message + '</strong>';
					})
					.catch(error => {
						document.getElementById('result').innerHTML = 'Error: ' + error;
						document.getElementById('result').style.display = 'block';
					});
				});
			</script>
		</body>
		</html>
		`
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	})

	// Start the HTTP server
	port := 8080
	fmt.Printf("Starting server on port %d...\n", port)
	fmt.Printf("Dashboard available at http://localhost:%d/dashboard\n", port)
	err := http.ListenAndServe(":"+strconv.Itoa(port), nil)
	if err != nil {
		fmt.Printf("Error starting server: %s\n", err)
	}
}

func APITranscode(video datatypes.VideoObject, resolution string, bitrate int, autoDelete bool, callbackURL string) {
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
		if callbackURL != "" {
			sendCallback(callbackURL, map[string]interface{}{
				"status": "failed",
				"error":  message,
				"video":  video,
			})
		}

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
	if callbackURL != "" {
		sendCallback(callbackURL, map[string]interface{}{
			"status":     "success",
			"new_object": newObj,
		})
	}

	// Display total space saved
	displaySpaceSaved() // CLI notification

	if autoDelete {
		err := os.Remove(video.FullFilePath)
		if err != nil {
			fmt.Println("Error deleting file", video.FullFilePath)
		}
		fmt.Println("file has been deleted: ", video.FullFilePath)
	}
	completionMessage := fmt.Sprintf("Transcoding completed: %s -> %s\nSpace saved for this file: %.2f GB\nTotal space saved so far: %.2f GB",
		video.FullFilePath, outputPath, float64(spaceSaved)/(1024*1024*1024), float64(totalSpaceSaved)/(1024*1024*1024))
	utils.SendTelegramMessage(completionMessage)
}

func sendCallback(callbackURL string, payload map[string]interface{}) {
	// Serialize the payload to JSON
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Error marshalling callback payload: %s\n", err)
		return
	}

	// Send POST request to the callback URL
	resp, err := http.Post(callbackURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		fmt.Printf("Error sending callback to %s: %s\n", callbackURL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Callback to %s returned status: %s\n", callbackURL, resp.Status)
	}
}
