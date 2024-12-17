package main

import (
	"fmt"

	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	fmt.Println("Starting main()")

	lambda.Start(Handler)
}

type InputEvent struct {
	Name string `json:"name"`
}

type OutputEvent struct {
	Message string `json:"message"`
}

func Handler(event InputEvent) (OutputEvent, error) {
	fmt.Printf("Handler received event: %#v\n", event)

	if event.Name == "" {
		return OutputEvent{Message: "Hello, World!"}, nil
	}

	return OutputEvent{Message: fmt.Sprintf("Hello, %s!", event.Name)}, nil
}
