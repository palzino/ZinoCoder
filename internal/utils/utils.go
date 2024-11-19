package utils

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

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
func DisplayDirectoryTree(tree map[string]interface{}, currentPath string, videos datatypes.VideoObjects, fileFilter func(datatypes.VideoObject) bool) (selectedDirs []string, selectedFiles []datatypes.VideoObject, recursive bool) {
	for {
		fmt.Printf("\nCurrent Path: %s\n", currentPath)
		fmt.Println("[0] Select files in this directory only (no subdirectories)")
		fmt.Println("[1] Select files in this directory and all subdirectories (recursive)")
		fmt.Println("[2] Go up a level")
		fmt.Println("[q] Quit")

		// Get the subtree for the current path
		subTree := getSubTree(tree, currentPath)
		if subTree == nil {
			fmt.Println("Error: Unable to retrieve subdirectory tree.")
			return nil, nil, false
		}

		// List subdirectories
		i := 3
		subDirs := []string{}
		for dir := range subTree {
			if dir == "" {
				continue
			}
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
			if filepath.Dir(currentPath) == currentPath {
				fmt.Println("Already at the base directory.")
			} else {
				currentPath = filepath.Dir(currentPath)
			}

		default:
			// Navigate into a selected subdirectory
			subDirIndex := choice - 3
			if subDirIndex >= 0 && subDirIndex < len(subDirs) {
				currentPath = filepath.Join(currentPath, subDirs[subDirIndex])
			} else {
				fmt.Println("Invalid choice. Please try again.")
			}
		}
	}
}

// getSubTree retrieves the subtree for the given path
func getSubTree(tree map[string]interface{}, path string) map[string]interface{} {
	parts := strings.Split(path, string(filepath.Separator))
	current := tree
	for _, part := range parts {
		if part == "" {
			continue
		}
		subTree, exists := current[part]
		if !exists {
			return nil
		}
		current = subTree.(map[string]interface{})
	}
	return current
}
