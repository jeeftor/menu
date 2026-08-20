.PHONY: build run test clean show serve cf-deploy

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

cf-deploy:
	@set -a; [ -f ../homelab/caddy/.env ] && . ../homelab/caddy/.env; set +a; \
	export CLOUDFLARE_EMAIL="$$CF_EMAIL"; \
	export CLOUDFLARE_API_KEY="$$CF_GLOBAL_API_KEY"; \
	export CLOUDFLARE_ACCOUNT_ID="$$CF_ACCOUNT_ID"; \
	cd alexa-worker && npx -y wrangler@3 deploy
