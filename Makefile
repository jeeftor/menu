.PHONY: build run test clean show serve lambda-deploy

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

lambda-deploy:
	cd alexa-lambda && zip -r /tmp/menu-alexa-lambda.zip index.mjs package.json
	aws lambda update-function-code \
		--function-name menu-alexa \
		--zip-file fileb:///tmp/menu-alexa-lambda.zip \
		--region us-east-1
	aws lambda wait function-updated \
		--function-name menu-alexa \
		--region us-east-1
	@echo "Lambda deployed: arn:aws:lambda:us-east-1:260672429786:function:menu-alexa"
