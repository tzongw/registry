package main

import (
	"context"
	"fmt"
	"github.com/go-redis/redis"
	log "github.com/sirupsen/logrus"
	"math/rand"
	"time"
	"tutorial"
)

func main() {
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	client := redis.NewClient(&redis.Options{})
	r := NewRegistry(client)
	s := "tutorial"
	r.Start(map[string]string{s: "localhost:19090"})
	c := NewServiceClient(r, s, nil)
	cli := tutorial.NewCalculatorClient(c)
	for i := 0; i < 10; i += 1 {
		go func() {
			for {
				handleClient(cli)
				time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
			}
		}()
	}
	select {}
}

var defaultCtx = context.Background()

func handleClient(client *tutorial.CalculatorClient) (err error) {
	client.Ping(defaultCtx)
	fmt.Println("ping()")

	sum, err := client.Add(defaultCtx, 1, 1)
	if err != nil {
		fmt.Println("add error:", err)
		return
	}
	fmt.Print("1+1=", sum, "\n")

	work := tutorial.NewWork()
	work.Op = tutorial.Operation_DIVIDE
	work.Num1 = 1
	work.Num2 = 0
	quotient, err := client.Calculate(defaultCtx, 1, work)
	if err != nil {
		switch v := err.(type) {
		case *tutorial.InvalidOperation:
			fmt.Println("Invalid operation:", v)
		default:
			fmt.Println("Error during operation:", err)
		}
		return err
	} else {
		fmt.Println("Whoa we can divide by 0 with new value:", quotient)
	}

	work.Op = tutorial.Operation_SUBTRACT
	work.Num1 = 15
	work.Num2 = 10
	diff, err := client.Calculate(defaultCtx, 1, work)
	if err != nil {
		switch v := err.(type) {
		case *tutorial.InvalidOperation:
			fmt.Println("Invalid operation:", v)
		default:
			fmt.Println("Error during operation:", err)
		}
		return err
	} else {
		fmt.Print("15-10=", diff, "\n")
	}

	log, err := client.GetStruct(defaultCtx, 1)
	if err != nil {
		fmt.Println("Unable to get struct:", err)
		return err
	} else {
		fmt.Println("Check log:", log.Value)
	}
	return err
}
