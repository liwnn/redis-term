if [ "$1" ]; then
    go run ./cmd/redis-term/main.go -p $1;
else
    go run ./cmd/redis-term/main.go;
fi
