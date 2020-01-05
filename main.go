package main

import (
	log "github.com/sirupsen/logrus"
	"registry/shared"
)

func main() {
	log.SetLevel(log.InfoLevel)
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	shared.Registry.Start(nil)
	select {}
}
