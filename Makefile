.PHONY: run

run:
	source .env && go run main.go --oneshot --verbose
