package main

import (
	"context"
	"fmt"
	"github.com/apache/thrift/lib/go/thrift"
	log "github.com/sirupsen/logrus"
	"registry/common"
	"tutorial"
)

func test() {
	var e error
	e = tutorial.NewInvalidOperation()
	if err, ok := e.(error); ok {
		println("error", err)
	}
	if err, ok := e.(thrift.TException); ok {
		println("execpt", err)
	}
	if err, ok := e.(thrift.TApplicationException); ok {
		println("app", err)
	}
	return
}

func main() {
	log.SetLevel(log.InfoLevel)
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	s := "tutorial"
	common.registry.Start(map[string]string{s: "localhost:19090"})
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
