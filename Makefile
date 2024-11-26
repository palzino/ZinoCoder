build:
	go build -o cmd/main

re:
	rm cmd/main
	go build -o cmd/main