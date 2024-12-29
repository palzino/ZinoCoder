package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/palzino/vidanalyser/internal/config"
	"github.com/palzino/vidanalyser/internal/datatypes"
)

// BuildDirectoryTree creates a nested map representing the directory structure from video metadata
func BuildDirectoryTree(videos datatypes.VideoObjects) map[string]interface{} {
	tree := make(map[string]interface{})
	for _, video := range videos.Object {
		parts := strings.Split(filepath.Dir(video.FullFilePath), string(filepath.Separator))
		current := tree
		for _, part := range parts {
			// Skip empty parts (e.g., from leading/trailing slashes)
			if part == "" {
				continue
			}
			if _, exists := current[part]; !exists {
				current[part] = make(map[string]interface{})
			}
			current = current[part].(map[string]interface{})
		}
	}
	return tree
}

// DisplayDirectoryTree shows an interactive tree for user navigation
func DisplayDirectoryTree(tree map[string]interface{}, currentPath string, baseDir string, videos datatypes.VideoObjects, fileFilter func(datatypes.VideoObject) bool) (selectedDirs []string, selectedFiles []datatypes.VideoObject, recursive bool) {
	for {
		// Display the full current path
		fmt.Printf("\nCurrent Path: %s\n", currentPath)
		fmt.Println("[0] Select files in this directory only (no subdirectories)")
		fmt.Println("[1] Select files in this directory and all subdirectories (recursive)")
		fmt.Println("[2] Go up a level")
		fmt.Println("[q] Quit")

		// Calculate the relative path for tree navigation
		relativePath := strings.TrimPrefix(currentPath, baseDir)
		relativePath = strings.TrimPrefix(relativePath, string(filepath.Separator)) // Remove leading slash if present

		// Get the subtree for the relative path
		subTree := getSubTree(tree, relativePath)
		if subTree == nil {
			fmt.Printf("Error: Unable to retrieve subdirectory tree for path '%s'.\n", relativePath)
			return nil, nil, false
		}

		// List subdirectories at the current level
		i := 3
		subDirs := []string{}
		for dir := range subTree {
			subDirs = append(subDirs, dir)
			fmt.Printf("[%d] %s (directory)\n", i, dir)
			i++
		}

		// Check for eligible video files in the current directory
		eligibleFiles := []datatypes.VideoObject{}
		for _, video := range videos.Object {
			if video.Location == currentPath && (fileFilter == nil || fileFilter(video)) {
				eligibleFiles = append(eligibleFiles, video)
			}
		}

		if len(eligibleFiles) > 0 {
			fmt.Printf("\nFound %d video files in the current directory that meet the criteria.\n", len(eligibleFiles))
		} else {
			fmt.Println("No eligible video files found in this directory.")
		}

		// Get user input
		var input string
		fmt.Print("Enter your choice: ")
		fmt.Scanln(&input)

		// Handle quitting
		if input == "q" {
			fmt.Println("Exiting directory tree navigation.")
			return nil, nil, false
		}

		// Handle numeric choices
		choice, err := strconv.Atoi(input)
		if err != nil {
			fmt.Println("Invalid input. Please enter a valid option.")
			continue
		}

		switch choice {
		case 0:
			// Select files in the current directory only
			selectedDirs = append(selectedDirs, currentPath)
			selectedFiles = eligibleFiles
			recursive = false
			return selectedDirs, selectedFiles, recursive

		case 1:
			// Select files in the current directory and all subdirectories
			selectedDirs = append(selectedDirs, currentPath)
			selectedFiles = eligibleFiles
			recursive = true
			return selectedDirs, selectedFiles, recursive

		case 2:
			// Go up one level
			parentPath := filepath.Dir(currentPath)
			if parentPath == baseDir || parentPath == currentPath {
				// Prevent going above the base directory
				fmt.Println("Already at the base directory.")
				currentPath = baseDir
			} else {
				currentPath = parentPath // Update the current path
			}

		default:
			// Navigate into a selected subdirectory
			subDirIndex := choice - 3
			if subDirIndex >= 0 && subDirIndex < len(subDirs) {
				currentPath = filepath.Join(currentPath, subDirs[subDirIndex]) // Update the current path
			} else {
				fmt.Println("Invalid choice. Please try again.")
			}
		}
	}
}

// getSubTree retrieves the subtree for the given path
func getSubTree(tree map[string]interface{}, relativePath string) map[string]interface{} {
	parts := strings.Split(relativePath, string(filepath.Separator))
	current := tree
	for _, part := range parts {
		if part == "" {
			continue
		}
		subTree, exists := current[part]
		if !exists {
			fmt.Printf("Path part '%s' not found in tree at level: %v\n", part, current)
			return nil
		}
		current = subTree.(map[string]interface{})
	}
	return current
}
func SendTelegramMessage(message string) {
	botToken := config.GetTelegramBotToken()
	chatID := config.GetTelegramChatID()
	if botToken == "" || chatID == "" {
		fmt.Println("Telegram bot token or chat ID not set. Skipping message sending.")
		return
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	body := map[string]string{
		"chat_id": chatID,
		"text":    message,
	}
	jsonBody, _ := json.Marshal(body)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		fmt.Printf("Error sending Telegram message: %s\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Failed to send Telegram message: %s\n", resp.Status)
	}
}
