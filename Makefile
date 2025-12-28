.PHONY: rhythm all

rhythm:
	go mod tidy
	go build -o rhythm ./game/main

vet:
	go vet ./...
