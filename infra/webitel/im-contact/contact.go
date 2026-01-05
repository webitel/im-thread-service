package imcontact

import (
	"log/slog"

	"github.com/webitel/im-thread-service/infra/webitel"
	"github.com/webitel/webitel-go-kit/infra/discovery"
	"google.golang.org/grpc"
)

const ServiceName string = "im-contact-service"

type Client struct {
	Logger *slog.Logger
	Conn *grpc.ClientConn
}

func New(logger *slog.Logger, discovery discovery.Discovery) (*Client, error) {
	conn, err := webitel.New(logger, discovery, ServiceName)
	if err != nil {
		return nil, err
	}

	var client = new(Client) 
	{
		client.Conn = conn
		client.Logger = logger
	}

	return client, nil
}

//TODO: add proper shutdown and health checks

func (c *Client) ContactsService() *ContactsClient {
	return newContactsClient(c)
}