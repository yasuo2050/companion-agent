module companion-agent/services/mock-business-service

go 1.25.0

require (
	companion-agent/contracts/companion-business v0.0.0
	github.com/redis/go-redis/v9 v9.22.0
	go.mongodb.org/mongo-driver/v2 v2.8.0
	google.golang.org/grpc v1.75.0
	google.golang.org/protobuf v1.36.12
)

replace companion-agent/contracts/companion-business => ../../contracts/companion-business

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.2.0 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/crypto v0.51.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250825161204-c5933d9347a5 // indirect
)
