package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/palzino/vidanalyser/internal/datatypes"
)

var DB *sql.DB

func InitDatabase(dbPath string) {
	var err error
	DB, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Error opening database: %s\n", err)
	}

	// Create the files table
	filesTableQuery := `
	CREATE TABLE IF NOT EXISTS files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		location TEXT NOT NULL,
		full_file_path TEXT NOT NULL UNIQUE,
		size INTEGER NOT NULL,
		width INTEGER,
		height INTEGER,
		length INTEGER,
		framerate REAL,
		frames INTEGER,
		bitrate INTEGER,
		file_extension TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	_, err = DB.Exec(filesTableQuery)
	if err != nil {
		log.Fatalf("Error creating files table: %s\n", err)
	}

	TranscodesTableQuery := `
	CREATE TABLE IF NOT EXISTS transcodes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		OriginalVideo TEXT NOT NULL,
		Transcoded TEXT NOT NULL,
		OldExtension TEXT NOT NULL,
		NewExtension TEXT NOT NULL,
		OldSize INTEGER NOT NULL,
		NewSize INTEGER NOT NULL,
		OriginalRes TEXT NOT NULL,
		NewRes TEXT NOT NULL,
		OldBitrate INTEGER NOT NULL,
		NewBitrate INTEGER NOT NULL,
		TimeTaken INTEGER NOT NULL,
	
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	_, err = DB.Exec(TranscodesTableQuery)
	if err != nil {
		log.Fatalf("Error creating files table: %s\n", err)
	}

	fmt.Println("Database initialized successfully.")
}

func InsertVideo(video datatypes.VideoObject) error {
	query := `
	INSERT INTO files (name, location, full_file_path, size, width, height, length, framerate, frames, bitrate, file_extension)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`
	_, err := DB.Exec(query, video.Name, video.Location, video.FullFilePath, video.Size, video.Width,
		video.Height, video.Length, video.Framerate, video.Frames, video.Bitrate, video.FileExtension)
	return err
}

func InsertTranscode(t datatypes.TranscodedVideo) error {
	query := `
	INSERT INTO transcodes (OriginalVideo, Transcoded, OldExtension, NewExtension, OldSize, NewSize, OriginalRes, NewRes, OldBitrate, NewBitrate, TimeTaken)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`
	_, err := DB.Exec(query, t.OriginalVideoPath, t.TranscodedPath, t.OldExtension, t.NewExtension, t.OldSize,
		t.NewSize, t.OriginalRES, t.NewRES, t.OldBitrate, t.NewBitrate, t.TimeTaken)
	return err
}

func DeleteVideo(filePath string) error {
	query := `DELETE FROM files WHERE full_file_path = ?`
	result, err := DB.Exec(query, filePath)
	if err != nil {
		return fmt.Errorf("error deleting video %s: %w", filePath, err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		fmt.Printf("No database entry found for %s to delete.\n", filePath)
	}

	return nil
}

func UpdateVideo(video datatypes.VideoObject) error {
	query := `
		UPDATE files SET
			name = ?, location = ?, size = ?, width = ?, height = ?, length = ?, framerate = ?, frames = ?, bitrate = ?
		WHERE full_file_path = ?
	`
	_, err := DB.Exec(query,
		video.Name,
		video.Location,
		video.Size,
		video.Width,
		video.Height,
		video.Length,
		video.Framerate,
		video.Frames,
		video.Bitrate,
		video.FullFilePath,
	)
	if err != nil {
		return fmt.Errorf("error updating video: %w", err)
	}
	return nil
}
func QueryVideoByPath(filePath string) (*datatypes.VideoObject, error) {
	query := `SELECT name, location, full_file_path, size, width, height, length, framerate, frames, bitrate FROM files WHERE full_file_path = ?`
	row := DB.QueryRow(query, filePath)

	var video datatypes.VideoObject
	err := row.Scan(
		&video.Name,
		&video.Location,
		&video.FullFilePath,
		&video.Size,
		&video.Width,
		&video.Height,
		&video.Length,
		&video.Framerate,
		&video.Frames,
		&video.Bitrate,
	)
	if err == sql.ErrNoRows {
		return nil, nil // No matching video
	} else if err != nil {
		return nil, fmt.Errorf("error querying video: %w", err)
	}
	return &video, nil
}
func QueryVideos(directory string, minSize float64) ([]datatypes.VideoObject, error) {
	query := `
	SELECT name, location, full_file_path, size, width, height, length, framerate, frames, bitrate
	FROM files
	WHERE location LIKE ? AND size >= ?;
	`

	rows, err := DB.Query(query, directory+"%", int(minSize*1024*1024*1024))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var videos []datatypes.VideoObject
	for rows.Next() {
		var video datatypes.VideoObject
		err := rows.Scan(&video.Name, &video.Location, &video.FullFilePath, &video.Size, &video.Width,
			&video.Height, &video.Length, &video.Framerate, &video.Frames, &video.Bitrate)
		if err != nil {
			return nil, err
		}
		videos = append(videos, video)
	}
	return videos, nil
}

func QueryAllVideos() ([]datatypes.VideoObject, error) {
	query := `
	SELECT name, location, full_file_path, size, width, height, length, framerate, frames, bitrate
	FROM files;
	`
	rows, err := DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("error querying all videos: %w", err)
	}
	defer rows.Close()

	var videos []datatypes.VideoObject
	for rows.Next() {
		var video datatypes.VideoObject
		err := rows.Scan(&video.Name, &video.Location, &video.FullFilePath, &video.Size, &video.Width,
			&video.Height, &video.Length, &video.Framerate, &video.Frames, &video.Bitrate)
		if err != nil {
			return nil, fmt.Errorf("error scanning video row: %w", err)
		}
		videos = append(videos, video)
	}

	return videos, nil
}

func QueryVideosByDirectory(directory string) ([]datatypes.VideoObject, error) {
	query := `
		SELECT * FROM files WHERE location LIKE ?
	`
	rows, err := DB.Query(query, directory+"%")
	if err != nil {
		return nil, fmt.Errorf("error querying videos by directory: %w", err)
	}
	defer rows.Close()

	videos := []datatypes.VideoObject{}
	for rows.Next() {
		var video datatypes.VideoObject
		if err := rows.Scan(&video.Name, &video.Location, &video.FullFilePath, &video.Size, &video.Width, &video.Height, &video.Length, &video.Framerate, &video.Frames, &video.Bitrate); err != nil {
			return nil, fmt.Errorf("error scanning video row: %w", err)
		}
		videos = append(videos, video)
	}
	return videos, nil
}

func UpdateVideoAfterTranscode(originalPath, newPath string, newSize int64) error {
	query := `
		UPDATE files SET full_file_path = ?, size = ? WHERE full_file_path = ?
	`
	_, err := DB.Exec(query, newPath, newSize, originalPath)
	if err != nil {
		return fmt.Errorf("error updating video after transcode: %w", err)
	}
	return nil
}

func CleanDatabase() error {
	// Query the database for all file paths
	query := `SELECT full_file_path FROM files`
	rows, err := DB.Query(query)
	if err != nil {
		return fmt.Errorf("error querying database for cleanup: %w", err)
	}
	defer rows.Close()

	var nonExistentFiles []string
	var totalFiles int

	for rows.Next() {
		var filePath string
		if err := rows.Scan(&filePath); err != nil {
			fmt.Printf("Error scanning file path: %s\n", err)
			continue
		}

		totalFiles++
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			nonExistentFiles = append(nonExistentFiles, filePath)
		} else if err != nil {
			// Handle unexpected errors during file system checks
			fmt.Printf("Error checking file %s: %s\n", filePath, err)
		}
	}

	// Log database scan results
	fmt.Printf("Total files scanned in database: %d\n", totalFiles)
	fmt.Printf("Files marked for removal: %d\n", len(nonExistentFiles))

	// Remove non-existent files from the database
	for _, filePath := range nonExistentFiles {
		if err := DeleteVideo(filePath); err != nil {
			fmt.Printf("Error removing entry for %s: %s\n", filePath, err)
		} else {
			fmt.Printf("Removed database entry for missing file: %s\n", filePath)
		}
	}

	if len(nonExistentFiles) == 0 {
		fmt.Println("No missing files found in the database.")
	} else {
		fmt.Printf("Cleaned %d entries from the database.\n", len(nonExistentFiles))
	}

	return nil
}

func BuildDirectoryTreeFromDatabase() (map[string]interface{}, string, error) {
	// Query all videos from the database
	videos, err := QueryAllVideos()
	if err != nil {
		return nil, "", fmt.Errorf("error querying videos for directory tree: %w", err)
	}

	if len(videos) == 0 {
		return nil, "/", nil // No videos, default to root
	}

	// Determine the common base directory
	baseDir := findCommonPrefix(videos)
	fmt.Printf("Common base directory: %s\n", baseDir)

	// Initialize the tree structure
	tree := make(map[string]interface{})

	// Build the directory tree
	for _, video := range videos {
		dirPath := filepath.Clean(filepath.Dir(video.FullFilePath))
		relativePath := strings.TrimPrefix(dirPath, baseDir)
		relativePath = strings.TrimPrefix(relativePath, string(filepath.Separator)) // Remove leading slash if present

		// Split the relative path into parts
		parts := strings.Split(relativePath, string(filepath.Separator))
		current := tree

		// Traverse or create nodes for each part
		for _, part := range parts {
			if part == "" {
				continue
			}
			if _, exists := current[part]; !exists {
				current[part] = make(map[string]interface{})
			}
			current = current[part].(map[string]interface{})
		}
	}

	return tree, baseDir, nil
}

// findCommonPrefix finds the common prefix between two paths
func findCommonPrefix(videos []datatypes.VideoObject) string {
	if len(videos) == 0 {
		return "/"
	}

	// Start with the first video's directory
	commonBaseDir := filepath.Dir(videos[0].FullFilePath)

	// Iterate over all videos to find the common base directory
	for _, video := range videos {
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

func IsInSelectedDirectory(location string, selectedDirs []string, recursive bool) bool {
	for _, dir := range selectedDirs {
		if recursive {
			// Check if the video's location is within the directory or any of its subdirectories
			if strings.HasPrefix(location, dir) {
				return true
			}
		} else {
			// Check if the video's location matches the directory exactly
			if location == dir {
				return true
			}
		}
	}
	return false
}
