package common

import "flag"

var redisAddr = flag.String("redis", "localhost:6379", "redis address")
var appName = flag.String("appName", "app", "app name")
