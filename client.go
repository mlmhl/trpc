package trpc

type callHandler func(clientName, serviceMethod string, args interface{}, reply interface{}) error

type closeHandler func(name string) error

func newClient(name string, callHandler callHandler, closeHandler closeHandler) *Client {
	return &Client{
		name:         name,
		callHandler:  callHandler,
		closeHandler: closeHandler,
	}
}

// Client is an alternative of native Client in net/rpc for test. You can simulate
// request/response lost, messages delay and network partition with this client.
// NOTE: There's no actual network connections established if you use this client.
type Client struct {
	// Unique name to identify this client.
	name string
	// Handler to process client request, usually provided by Network.
	callHandler callHandler
	// Handler to close this client, usually provided by Network.
	closeHandler closeHandler
}

func (c *Client) Call(serviceMethod string, args interface{}, reply interface{}) error {
	return c.callHandler(c.name, serviceMethod, args, reply)
}

func (c *Client) Close() error {
	return c.closeHandler(c.name)
}
