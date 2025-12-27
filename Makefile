.PHONY: rhythm all

rhythm:
	go get ./...
	go build -o rhythm ./game/main

vet:
	go vet ./...
