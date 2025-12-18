sudo docker run --log-opt max-size=10m --log-opt max-file=5 -d --name aws_cdn -p 8080:8080 --restart=always  -v /opt/www/aws_cdn:/opt/www/aws_cdn   aws_cdn:v1
