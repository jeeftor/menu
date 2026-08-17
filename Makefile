.PHONY: build run test clean show serve

BINARY := bin/menu

build:
	go build -o $(BINARY) .

run: serve

serve:
	go run main.go serve

show:
	go run main.go show

test:
	gotestsum ./...

clean:
	rm -rf bin/ .cache/

lint:
	go vet ./...
