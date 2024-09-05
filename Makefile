run:
	go run main.go
build:
	go build -ldflags "-w -s" -o wave main.go
fmt:
	go fmt
