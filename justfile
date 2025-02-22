set dotenv-load

_list:
	just -l

build:
	go build -o ./.bin/runner ./cmd/runner

run-configure: build
	op run -- ./.bin/runner configure \
		--name "test-`date +%Y%m%d-%H%M%S`" \
		--label "runner-persistent-test" \
		--organization "https://github.com/test-0194ec68"

run-run: build
	op run -- ./.bin/runner run

compose-up:
	docker-compose up -d
