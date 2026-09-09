.PHONY: test build build-linux install clean

APP=hostpulse
CMD=./cmd/monitor

test:
	go test ./...

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(APP) $(CMD)

# Cross-compile from Windows/macOS for a Linux server
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(APP)-linux $(CMD)

install: build
	@test -f .env || (echo "missing .env — copy .env.example"; exit 1)
	@test -f config.yaml || (echo "missing config.yaml — copy config.example.yaml"; exit 1)
	sudo mkdir -p /opt/hostpulse /var/lib/hostpulse
	sudo cp $(APP) /opt/hostpulse/$(APP)
	sudo cp .env /opt/hostpulse/.env
	sudo cp config.yaml /opt/hostpulse/config.yaml
	sudo chmod 600 /opt/hostpulse/.env
	sudo cp deploy/hostpulse.service /etc/systemd/system/hostpulse.service
	sudo systemctl daemon-reload
	sudo systemctl enable --now hostpulse
	@echo "installed — edit /opt/hostpulse/config.yaml then: sudo systemctl restart hostpulse"

clean:
	rm -f $(APP) $(APP)-linux
