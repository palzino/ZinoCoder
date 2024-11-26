# Video-File-Analyser
 A Linux based software tool written in golang, using FFProbe to scan video directories and store metadata for analysis.

To build the project use 
``` go build -o cmd/main.go ```
Then to scan a directory: 
```./main scan "/path/to/dir"```
To analyse the data collected 
```./main analyse```
To transcode 
```./main transcode #minimum_file_size_as_INT "1080p" #Number_of_simultanious_transcodes```
e.g: ```./main transcode 3 "1080p" 3``` for all files above 3gb, 1080p or higher and 3 concurrent transcodes
