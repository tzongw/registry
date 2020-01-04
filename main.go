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
	log.SetLevel(log.ErrorLevel)
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	client := redis.NewClient(&redis.Options{})
	s := NewService(client)
	s.Start(map[string]string{"aa": "bb", "cc": "dd"})
	test()
	select {}
}

func test() {
	c := NewClient("127.0.0.1:19090", nil)
	cli := tutorial.NewCalculatorClient(c)
	for i := 0; i < 10; i += 1 {
		go func() {
			for {
				handleClient(cli)
				time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
			}
		}()
	}
}

var defaultCtx = context.Background()

func handleClient(client *tutorial.CalculatorClient) (err error) {
	client.Ping(defaultCtx)
	fmt.Println("ping()")

	sum, _ := client.Add(defaultCtx, 1, 1)
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
