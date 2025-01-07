package tree

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/palzino/vidanalyser/internal/datatypes"
)

type DirectoryNode struct {
	Name     string
	Path     string
	Children map[string]*DirectoryNode
	Files    []datatypes.VideoObject
}

// NewDirectoryNode creates a new directory tree from the base directory
func NewDirectoryNode(baseDir string) *DirectoryNode {
	return &DirectoryNode{
		Name:     filepath.Base(baseDir),
		Path:     baseDir,
		Children: make(map[string]*DirectoryNode),
		Files:    make([]datatypes.VideoObject, 0),
	}
}

// AddVideo adds a video to the appropriate location in the tree
func (n *DirectoryNode) AddVideo(video datatypes.VideoObject) {
	// Get relative path from the base directory
	relPath, err := filepath.Rel(n.Path, video.Location)
	if err != nil {
		fmt.Printf("Error getting relative path for %s: %v\n", video.FullFilePath, err)
		return
	}

	if relPath == "." {
		// Video belongs in current directory
		n.Files = append(n.Files, video)
		return
	}

	// Split path into components
	parts := strings.Split(relPath, string(filepath.Separator))
	current := n

	// Create or traverse path
	for _, part := range parts {
		if part == "" {
			continue
		}

		child, exists := current.Children[part]
		if !exists {
			child = &DirectoryNode{
				Name:     part,
				Path:     filepath.Join(current.Path, part),
				Children: make(map[string]*DirectoryNode),
				Files:    make([]datatypes.VideoObject, 0),
			}
			current.Children[part] = child
		}
		current = child
	}

	// Add video to final directory
	current.Files = append(current.Files, video)
}

// GetSubDirectory returns a subdirectory node given a path
func (n *DirectoryNode) GetSubDirectory(path string) *DirectoryNode {
	if path == n.Path {
		return n
	}

	relPath, err := filepath.Rel(n.Path, path)
	if err != nil {
		return nil
	}

	current := n
	parts := strings.Split(relPath, string(filepath.Separator))

	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}

		child, exists := current.Children[part]
		if !exists {
			return nil
		}
		current = child
	}

	return current
}

// GetAllFiles returns all files in this directory and optionally its subdirectories
func (n *DirectoryNode) GetAllFiles(recursive bool) []datatypes.VideoObject {
	files := make([]datatypes.VideoObject, len(n.Files))
	copy(files, n.Files)

	if recursive {
		for _, child := range n.Children {
			files = append(files, child.GetAllFiles(true)...)
		}
	}

	return files
}

// FilterFiles returns files that match the given filter function
func (n *DirectoryNode) FilterFiles(filter func(datatypes.VideoObject) bool, recursive bool) []datatypes.VideoObject {
	var result []datatypes.VideoObject

	for _, file := range n.Files {
		if filter(file) {
			result = append(result, file)
		}
	}

	if recursive {
		for _, child := range n.Children {
			result = append(result, child.FilterFiles(filter, true)...)
		}
	}

	return result
}
