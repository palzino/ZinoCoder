package deleter

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/palzino/vidanalyser/internal/utils"
)

type RenamedFile struct {
	OriginalName string `json:"original_name"`
	NewName      string `json:"new_name"`
	OriginalSize int64  `json:"original_size"`
	NewSize      int64  `json:"new_size"`
}

// DeleteOriginalFiles reads a JSON file containing renamed file mappings and deletes the original files
func DeleteOriginalFiles(jsonPath string) error {
	file, err := os.Open(jsonPath)
	if err != nil {
		utils.SendTelegramMessage(fmt.Sprintf("Error opening JSON file: %s", err))
		return err
	}
	defer file.Close()

	var renamedFiles []RenamedFile
	err = json.NewDecoder(file).Decode(&renamedFiles)
	if err != nil {
		utils.SendTelegramMessage(fmt.Sprintf("Error decoding JSON data: %s", err))
		return err
	}

	queueLength := len(renamedFiles)
	for _, renamedFile := range renamedFiles {
		err := os.Remove(renamedFile.OriginalName)
		if err != nil {
			utils.SendTelegramMessage(fmt.Sprintf("Error deleting file %s: %s", renamedFile.OriginalName, err))
		} else {
			utils.SendTelegramMessage(fmt.Sprintf("Deleted original file: %s", renamedFile.OriginalName))
		}

		// Notify remaining items in the queue
		queueLength--
		utils.SendTelegramMessage(fmt.Sprintf("Items left in queue: %d", queueLength))
	}

	// Notify when deletion is complete
	utils.SendTelegramMessage("All original files have been deleted.")
	return nil
}
