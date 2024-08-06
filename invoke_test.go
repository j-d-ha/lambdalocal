package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/lambda/messages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockFunction struct {
	mock.Mock
}

func (fn *MockFunction) setValues() messages.InvokeResponse {
	args := fn.Called()

	return args.Get(0).(messages.InvokeResponse)
}

func (fn *MockFunction) Invoke(req *messages.InvokeRequest, response *messages.InvokeResponse) error {
	responseNew := fn.setValues()
	response.Error = responseNew.Error
	response.Payload = responseNew.Payload

	return nil
}

func mustStartRPCServer(ctx context.Context, service any, address string) {
	if err := rpc.Register(service); err != nil {
		panic(fmt.Sprintf("failed to register RPC service: %v", err))
	}

	listener, err := net.Listen("tcp", address)
	defer func() {
		if err := listener.Close(); err != nil {
			log.Printf("failed to close listener: %v", err)
		}
	}()
	if err != nil {
		panic(fmt.Sprintf("Failed to listen: %v", err))
	}

	fmt.Printf("Server is running on %s\n", address) //nolint:forbidigo

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				if ctx.Err() != nil {
					log.Println("ctx.Err(), Server closed")
					return
				}
				log.Println("Accept error:", err)
			}
			rpc.ServeConn(conn)
		}
	}()

	<-ctx.Done()
	fmt.Println("Server is shutting down")
}

func TestLambdaRPC_Invoke(t *testing.T) { //nolint:paralleltest

	// run test RPC client
	mockService := new(MockFunction)

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		fmt.Println("Cancelled")
	}()
	go mustStartRPCServer(ctx, mockService, "localhost:8000")

	tests := map[string]struct {
		mockReturn     func()
		inputAddress   string
		serviceMethod  string
		executionLimit time.Duration
		input          []byte
		expectedOutput messages.InvokeResponse
		expectError    bool
	}{
		"successful invocation": {
			mockReturn: func() {
				mockService.On("setValues").
					Return(messages.InvokeResponse{
						Payload: []byte("response"),
					}).
					Once()
			},
			inputAddress:   "localhost:8000",
			serviceMethod:  "MockFunction.Invoke",
			executionLimit: time.Second * 5,
			input:          []byte("test"),
			expectedOutput: messages.InvokeResponse{
				Payload: []byte("response"),
			},
			expectError: false,
		},
		"invalid inputAddress": {
			mockReturn:     func() {},
			inputAddress:   "localhost:3000",
			serviceMethod:  "MockFunction.Invoke",
			executionLimit: time.Second * 5,
			input:          []byte("test"),
			expectedOutput: messages.InvokeResponse{},
			expectError:    true,
		},
		"invalid service method": {
			mockReturn:     func() {},
			inputAddress:   "localhost:8000",
			serviceMethod:  "MockFunction.DoesNotExist",
			executionLimit: time.Second * 5,
			input:          []byte("test"),
			expectedOutput: messages.InvokeResponse{},
			expectError:    true,
		},
		"invalid service": {
			mockReturn:     func() {},
			inputAddress:   "localhost:8000",
			serviceMethod:  "DoesNotExist.methodName",
			executionLimit: time.Second * 5,
			input:          []byte("test"),
			expectedOutput: messages.InvokeResponse{},
			expectError:    true,
		},
	}
	for name, tc := range tests { //nolint:paralleltest
		t.Run(name, func(t *testing.T) {
			tc.mockReturn()

			lambdaRPC := NewLambdaRPC(
				tc.inputAddress,
				tc.executionLimit,
				WithServiceMethod(tc.serviceMethod),
			)

			output, err := lambdaRPC.Invoke(tc.input)

			assert.Equal(t, tc.expectedOutput, output)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
