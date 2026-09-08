.PHONY: build test race lint run static docker clean

build:
	go build -trimpath -o bin/reveal-md .

test:
	go test ./...

race:
	go test ./... -race

lint:
	gofmt -l . internal
	go vet ./...

run:
	go run . demo

static:
	go run . demo --static _static

docker:
	docker build -t reveal-md-go .

clean:
	rm -rf bin _static coverage.out
