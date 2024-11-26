package datatypes

import "time"

type VideoObject struct {
	Name          string  `json:"name"`
	Location      string  `json:"location"`
	FullFilePath  string  `json:"full_file_path"`
	Size          int     `json:"size"`
	Width         int     `json:"width"`
	Height        int     `json:"height"`
	Length        int     `json:"length"`    // Length of the video in seconds
	Framerate     float64 `json:"framerate"` // Framerate of the video
	Frames        int     `json:"frames"`    // Total number of frames
	Bitrate       int     `json:"bitrate"`   // Bitrate of the video in bits per second
	FileExtension string  `json:"file_extension"`
}

type TranscodedVideo struct {
	OriginalVideoPath string `json:"original_video"`
	TranscodedPath    string `json:"transcoded"`
	OldExtension      string `json:"old_extension"`
	NewExtension      string `json:"new_extension"`
	OldSize           int    `json:"old_size"`
	NewSize           int    `json:"new_size"`
	OriginalRES       string `json:"original_res"`
	NewRES            string `json:"new_res"`
	OldBitrate        int    `json:"old_bitrate"`
	NewBitrate        int    `json:"new_bitrate"`
	TimeTaken         int    `json:"time_taken"`
}

type VideoObjects struct {
	Object []VideoObject `json:"videos"`
}

type SmallVideos struct {
	vid []VideoObject
}

type Video struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	FileID    int       `json:"file_id"`
	ViewCount int       `json:"view_count"`
	CreatedAt time.Time `json:"created_at"`
}
