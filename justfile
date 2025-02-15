set dotenv-load

_list:
	just -l

build:
	go build -o ./.bin/runner ./cmd/runner

run-configure: build compose-up
	op run -- ./.bin/runner configure \
		--name "test-`date +%Y%m%d-%H%M%S`" \
		--label "runner-persistent-test" \
		--organization "https://github.com/test-0194ec68"

compose-up:
	docker-compose up -d
