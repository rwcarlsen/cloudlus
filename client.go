package cloudlus

import "net/rpc"

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

func (c *Client) Heartbeat(w WorkerId, j JobId) error {
	var unused int
	return c.client.Call("RPC.Heartbeat", NewBeat(w, j), &unused)
}

func (c *Client) Submit(j *Job) (*Job, error) {
	result := &Job{}
	err := c.client.Call("RPC.Submit", j, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) Fetch(w *Worker) (*Job, error) {
	j := &Job{}
	err := c.client.Call("RPC.Fetch", w.Id, &j)
	if err != nil {
		return nil, err
	}
	return j, nil
}

func (c *Client) Push(w *Worker, j *Job) error {
	var unused int
	return c.client.Call("RPC.Push", j, &unused)
}
