client:
	CGO_ENABLED=0 go build -o bin/mock_client ./mock/cmd/client.go

exporter:
	CGO_ENABLED=0 go build -o bin/mock_exporter ./mock/exporter/server.go

image:
	docker build -f ./mock/exporter/Dockerfile -t example/exporter:v1 .

updator:
	go build -o bin/updator ./updator/server/server.go

main:
	go build -o bin/main ./main.go

build:
	make client
	make exporter
	make image
	make updator
	make main
