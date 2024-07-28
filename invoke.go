package lambdalocal

import (
	"fmt"
	"net/rpc"
	"time"

	"github.com/aws/aws-lambda-go/lambda/messages"
)

func invoke(addr string, data []byte, executionLimit time.Duration) (messages.InvokeResponse, error) {
	deadline := time.Now().Add(executionLimit)
	request := messages.InvokeRequest{
		Payload: data,
		Deadline: messages.InvokeRequest_Timestamp{
			Seconds: deadline.Unix(),
			Nanos:   int64(deadline.Nanosecond()),
		},
	}
	client, err := rpc.Dial("tcp", addr)
	if err != nil {
		return messages.InvokeResponse{}, fmt.Errorf("[in lambdalocal.invoke] rpcDial error: %w", err)
	}

	var response messages.InvokeResponse
	err = client.Call("Function.Invoke", request, &response)
	if err != nil {
		return messages.InvokeResponse{}, fmt.Errorf("[in lambdalocal.invoke] client.Call error: %w", err)
	}

	return response, nil
}
