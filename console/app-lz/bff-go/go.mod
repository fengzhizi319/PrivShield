module github.com/fengzhizi319/PrivShield/console/app-lz/bff-go

go 1.25.0

require (
	github.com/fengzhizi319/PrivShield/pkg v0.0.0
	github.com/gin-gonic/gin v1.12.0
	google.golang.org/grpc v1.82.1
	google.golang.org/protobuf v1.36.11
)

replace github.com/fengzhizi319/PrivShield/pkg => ../../../pkg
