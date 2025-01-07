package main

import (
	"fmt"
	"os"

	"github.com/palzino/vidanalyser/internal/analyser"
	"github.com/palzino/vidanalyser/internal/config"
	"github.com/palzino/vidanalyser/internal/db"
	"github.com/palzino/vidanalyser/internal/deleter"
	"github.com/palzino/vidanalyser/internal/scanner"
	"github.com/palzino/vidanalyser/internal/transcoder"
)

func main() {

	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <command> <path>")
		return
	}

	db.InitDatabase("video_metadata.db")

	config.LoadConfig()

	command := os.Args[1]

	switch command {
	case "scan":
		if len(os.Args) < 3 {
			fmt.Println("Usage: go run main.go scan <path>")
			return
		}
		path := os.Args[2]
		wg := scanner.ProcessMasterDirectory(path)
		wg.Wait()
		fmt.Printf("Total video files: %d\n", scanner.GetTotalVideos())

	case "analyse":
		analyser.AnalyzeDatabase()

	case "transcode":
		if len(os.Args) < 3 {
			fmt.Println("Usage: go run main.go transcode [background|foreground]")
			return
		}
		mode := os.Args[2]
		switch mode {
		case "background":
			transcoder.StartBackgroundTranscoding()
		case "foreground":
			transcoder.StartInteractiveTranscoding(false)
		default:
			fmt.Println("Invalid mode. Use 'background' or 'foreground'")
		}

	case "clean":
		db.CleanDatabase()

	case "del-og":
		renamedFilesJSON := "renamed_files.json"
		err := deleter.DeleteOriginalFiles(renamedFilesJSON)
		if err != nil {
			fmt.Printf("Error deleting original files: %s\n", err)
		} else {
			fmt.Println("All original files have been successfully deleted.")
		}

	default:
		fmt.Println("Unknown command. Use 'scan', 'analyse', 'transcode', or 'del-og'.")
	}

}
