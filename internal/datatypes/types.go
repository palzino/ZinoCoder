package datatypes

type VideoObject struct {
	Name         string  `json:"name"`
	Location     string  `json:"location"`
	FullFilePath string  `json:"full_file_path"`
	Size         int     `json:"size"`
	Width        int     `json:"width"`
	Height       int     `json:"height"`
	Length       int     `json:"length"`    // Length of the video in seconds
	Framerate    float64 `json:"framerate"` // Framerate of the video
	Frames       int     `json:"frames"`    // Total number of frames
	Bitrate      int     `json:"bitrate"`   // Bitrate of the video in bits per second
}

type VideoObjects struct {
	Object []VideoObject `json:"videos"`
}

type SmallVideos struct {
	vid []VideoObject
}
