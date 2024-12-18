# lambdalocal Example: Invoke Event

## Walkthrough

For this example, we wil be using a basic Go Lambda that accepts a custom event and returns a custom
response. Do to the flexible nature of Lambda, these events could be swapped out for any
other without materially changing the demonstration.

Lets start by looking at the Handler that is in `main.go`.

```go
package main

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
```

In this example, we have our handler, called `Handler` which accepts `InputEvent` and returns
`OutputEvent`. We also have some basic print statements to show the flow of execution.

Next, lets look at our event in `event.json`:

```json
{
  "name": "Gopher"
}
```

The contents of this file match the structure of our `InputEvent` struct.

Now that we have looked at our sample code, lets start our lambda running locally:

```bash
export _LAMBDA_SERVER_PORT=8000 && go run main.go
```

This command sets an environment variable, `_LAMBDA_SERVER_PORT` to `8000` and then runs the
`main.go` file. This environment variable is used by the `lambda` package to indicate how the lambda
should be started. If the `LAMBDA_SERVER_PORT` environment variable is set, the lambda will be
started as an RPC server. Lambda local uses this to pass events to the running lambda.

With the lambda running, lets invoke our lambda though an event using `lambdalocal`.

> [!NOTE]
> If you have not already installed `lambdalocal`, run
`go install github.com/j-d-ha/lambdalocal@latest`

```bash
lambdalocal event --file event.json
```

This command will invoke the lambda by making an RPC call to it and passing the contents of the
`event.json` file as the event. Running the above command should result in the following output:

```
---------------------------------------------------------------------------
21:37:31.471 INF Starting local Lambda invocation with Event
21:37:31.473 INF Lambda returned JSON payload:
{
    "message": "Hello, Gopher!"
}
21:37:31.473 INF Lambda invocation complete, Exiting...
---------------------------------------------------------------------------
```
