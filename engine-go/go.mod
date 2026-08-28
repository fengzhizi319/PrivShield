module github.com/fengzhizi319/PrivShield/engine-go

go 1.25

require (
	github.com/gin-gonic/gin v1.10.0
	github.com/fengzhizi319/PrivShield/privacy-go-sdk v0.0.0
)

replace github.com/fengzhizi319/PrivShield/privacy-go-sdk => ../privacy-go-sdk
