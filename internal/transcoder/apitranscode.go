package transcoder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/palzino/vidanalyser/internal/datatypes"
	"github.com/palzino/vidanalyser/internal/db"
	"github.com/palzino/vidanalyser/internal/utils"
)

type Server struct {
	name       string
	addr       string
	concurrent int
}
type Servers struct {
	servers []Server
}

func sendToTranscodingServer(server Server, video datatypes.VideoObject, resolution string, bitrate int, autoDelete bool) error {
	// Construct the server's transcoding URL
	url := fmt.Sprintf("http://%s/transcode", server.addr)

	// Client's callback URL
	callbackURL := "http://<client_ip>:<client_port>/callback"

	// Payload with video and callback URL
	payload := map[string]interface{}{
		"file_path":    video.FullFilePath,
		"resolution":   resolution,
		"bitrate":      bitrate,
		"auto_delete":  autoDelete,
		"callback_url": callbackURL,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error creating payload: %w", err)
	}

	// Send request to server
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("error sending request to server: %w", err)
	}
	defer resp.Body.Close()

	// Handle server response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server %s responded with status: %d", server.name, resp.StatusCode)
	}

	return nil
}

func startCallbackServer(serverSemaphores map[string]chan struct{}, numVids *int) {
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			ServerName string                    `json:"server_name"`
			NewObject  datatypes.TranscodedVideo `json:"new_object"`
		}

		// Parse callback payload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid payload", http.StatusBadRequest)
			return
		}

		db.InsertTranscode(payload.NewObject)

		*numVids--
		fmt.Printf("Files remaining: %d\n", *numVids)

		// Find the corresponding server semaphore and release it
		if sem, exists := serverSemaphores[payload.ServerName]; exists {
			select {
			case sem <- struct{}{}:
				// Semaphore slot freed
				fmt.Printf("Server %s is now available.\n", payload.ServerName)
			default:
				fmt.Printf("Server %s was already available.\n", payload.ServerName)
			}
		}

		// Acknowledge the callback
		w.WriteHeader(http.StatusOK)
	})

	// Start the callback server
	go func() {
		fmt.Println("Starting callback server on :8080")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			fmt.Printf("Error starting callback server: %v\n", err)
		}
	}()
}

func StartAPITranscoding() {
	Servers := Servers{
		servers: []Server{
			{name: "Server1", addr: "", concurrent: 2},
			{name: "Server2", addr: "", concurrent: 3},
			{name: "Server3", addr: "", concurrent: 3},
		},
	}
	// Query all videos from the database
	videos, err := db.QueryAllVideos()
	if err != nil {
		fmt.Printf("Error querying videos: %s\n", err)
		return
	}

	// Build the directory tree from the database
	directoryTree, baseDir, err := db.BuildDirectoryTreeFromDatabase()
	if err != nil {
		fmt.Printf("Error building directory tree: %s\n", err)
		return
	}
	fmt.Printf("Starting from base directory: %s\n", baseDir)

	// Ask user for input preferences
	var resolution string
	var minSize float64
	var outputResolution string
	var outputBitrate int
	var autoDelete bool

	fmt.Print("Enter desired input resolution (e.g., 720p,1080p,4k): ")
	fmt.Scanln(&resolution)
	fmt.Print("Enter desired minimum filesize for transcoding (GB): ")
	fmt.Scanln(&minSize)
	fmt.Print("Enter desired output resolution (e.g., 1280x720): ")
	fmt.Scanln(&outputResolution)
	fmt.Print("Enter desired output bitrate in kbps (e.g., 3500): ")
	fmt.Scanln(&outputBitrate)
	fmt.Println("Auto delete original files after transcoding? (true/false): ")
	fmt.Scanln(&autoDelete)

	// Create a filter function for eligible files
	fileFilter := func(video datatypes.VideoObject) bool {
		return float64(video.Size)/(1024*1024*1024) >= minSize && shouldTranscode(video.Width, video.Height, resolution)
	}

	// Navigate the directory tree and select files for transcoding
	selectedDirs, selectedFiles, recursive := utils.DisplayDirectoryTree(directoryTree, baseDir, baseDir, datatypes.VideoObjects{Object: videos}, fileFilter)

	// Prepare server-specific semaphores
	serverSemaphores := make(map[string]chan struct{})
	for _, server := range Servers.servers {
		serverSemaphores[server.name] = make(chan struct{}, server.concurrent)

		// Initially, fill semaphore slots to max capacity
		for i := 0; i < server.concurrent; i++ {
			serverSemaphores[server.name] <- struct{}{}
		}
	}

	// Start the callback server
	numVids := len(videos)
	startCallbackServer(serverSemaphores, &numVids)

	var wg sync.WaitGroup

	utils.SendTelegramMessage(fmt.Sprintf("Starting transcoding of %d videos", numVids))

	for _, video := range videos {
		if (IsInSelectedDirectory(video.Location, selectedDirs, recursive) || containsVideo(selectedFiles, video)) &&
			fileFilter(video) {

			// Find an available server
			for _, server := range Servers.servers {
				select {
				case <-serverSemaphores[server.name]: // Wait for server to become available
					wg.Add(1)
					go func(server Server, video datatypes.VideoObject) {
						defer wg.Done()

						err := sendToTranscodingServer(server, video, outputResolution, outputBitrate, autoDelete)
						if err != nil {
							fmt.Printf("Error transcoding video on server %s: %v\n", server.name, err)
							serverSemaphores[server.name] <- struct{}{} // Retry semaphore release on error
						}
					}(server, video)
					break
				default:
					// All servers at capacity; wait for callback
					continue
				}
			}
		}
	}

	wg.Wait()
	fmt.Println("All selected videos have been transcoded.")
}
