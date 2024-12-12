# Video-File-Analyser
 A Linux based software tool written in golang, using FFProbe to scan video directories and store metadata for analysis.

## To build the project use
```make build``` OR
``` go build -o cmd/main ```
## To scan a directory: 
```cd cmd && ./main scan "/path/to/dir"```
## To analyse the data collected 
```./main analyse```
## To transcode 
```./main transcode```
