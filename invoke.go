package main

import (
	"fmt"
	"net/rpc"
	"time"

	"github.com/aws/aws-lambda-go/lambda/messages"
	"github.com/google/uuid"
)

type Option func(*LambdaRPCClient)

type LambdaRPCClient struct {
	// address is the address of the locally running lambda.
	address string
	// executionLimit is the maximum allowed duration of the lambda request
	executionLimit time.Duration
	// serviceMethod is the name of the RPC method that is called
	serviceMethod string
}

// WithServiceMethod sets the service method for the RPC call.
func WithServiceMethod(serviceMethod string) Option {
	return func(lambda *LambdaRPCClient) {
		lambda.serviceMethod = serviceMethod
	}
}

// NewLambdaLambdaRPCClient is a constructor for LambdaRPCClient struct.
func NewLambdaLambdaRPCClient(address string, executionLimit time.Duration, options ...Option) LambdaRPCClient {
	lambdaRPC := LambdaRPCClient{
		address:        address,
		executionLimit: executionLimit,
		serviceMethod:  "Function.Invoke",
	}

	for _, option := range options {
		option(&lambdaRPC)
	}

	return lambdaRPC
}

// Invoke sends an RPC request to invoke a lambda function with the given payload data.
func (l LambdaRPCClient) Invoke(data []byte) (messages.InvokeResponse, error) {
	deadline := time.Now().Add(l.executionLimit)
	request := messages.InvokeRequest{
		Payload:   data,
		RequestId: uuid.New().String(),
		Deadline: messages.InvokeRequest_Timestamp{
			Seconds: deadline.Unix(),
			Nanos:   int64(deadline.Nanosecond()),
		},
	}

	client, err := rpc.Dial("tcp", l.address)
	if err != nil {
		return messages.InvokeResponse{}, fmt.Errorf(
			"[in lambdalocal.invoke] rpcDial error, address '%s': %w",
			l.address,
			err,
		)
	}

	defer func() {
		_ = client.Close()
	}()

	var response messages.InvokeResponse

	err = client.Call(l.serviceMethod, request, &response)
	if err != nil {
		return messages.InvokeResponse{}, fmt.Errorf(
			"[in lambdalocal.invoke] client.Call error: %w",
			err,
		)
	}

	return response, nil
}
