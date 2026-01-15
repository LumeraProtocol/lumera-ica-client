APP ?= lumera-ica-client
BIN_DIR ?= build

.PHONY: build
build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(APP) .
