.PHONY: test run dry-run vet lint clean

# Run all tests with verbose output
test:
	go test ./... -v -count=1

# Run the M3U filter (requires .env or exported vars)
run:
	go run ./cmd/iptv-filter/

# Dry-run (save locally, skip S3 upload)
dry-run:
	DRY_RUN=true go run ./cmd/iptv-filter/

# Run go vet
vet:
	go vet ./...

# Clean output directory and temporary files
clean:
	rm -rf output/
	rm -f playlist.m3u playlist-all.m3u
