package cloudlus

import (
	"net/rpc"
)

type Client struct {
	client *rpc.Client
}

func Dial(addr string) (*Client, error) {
	client, err := rpc.DialHTTP("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &Client{client: client}, nil
}

func (c *Client) Submit(j *Job) (*Job, error) {
	result := NewJob()
	err := c.client.Call("RPC.Submit", j, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) Fetch(w *Worker) (*Job, error) {
	panic("not implemented")
}

func (c *Client) Push(w *Worker, j *Job) error {
	panic("not implemented")
}
