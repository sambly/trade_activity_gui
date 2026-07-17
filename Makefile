.PHONY: install-wails build dev

# Фиксируем версию Wails CLI, соответствующую go.mod
WAILS_VERSION := v2.12.0

install-wails:
	go install github.com/wailsapp/wails/v2/cmd/wails@$(WAILS_VERSION)

build: install-wails
	wails build

dev: install-wails
	wails dev