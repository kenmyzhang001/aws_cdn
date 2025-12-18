env GOOS=linux GOARCH=amd64 CGO_ENABLED=0   go build   -o aws_cdn  cmd/server/main.go 
sudo docker restart aws_cdn
