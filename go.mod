module registry

go 1.13

replace shared v0.0.0 => /Users/tangzongwei/Documents/thrift/tutorial/gen-go/shared

replace tutorial v0.0.0 => /Users/tangzongwei/Documents/thrift/tutorial/gen-go/tutorial

require (
	github.com/apache/thrift v0.13.0
	github.com/go-redis/redis v6.15.6+incompatible
	github.com/google/uuid v1.1.1
	github.com/gorilla/websocket v1.4.1
	github.com/micro/go-micro v1.18.0
	github.com/sirupsen/logrus v1.4.2
	shared v0.0.0 // indirect
	tutorial v0.0.0
)
