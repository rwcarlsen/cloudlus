package cloudlus

import "net/rpc"

type Client struct {
	client *rpc.Client
	err    error
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

func (c *Client) Retrieve(j JobId) (*Job, error) {
	var result *Job
	err := c.client.Call("RPC.Retrieve", j, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) Run(j *Job) (*Job, error) {
	ch := c.Start(j, nil)
	result := <-ch
	if err := c.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) Err() error { return c.err }

func (c *Client) Start(j *Job, ch chan *Job) chan *Job {
	if ch == nil {
		ch = make(chan *Job, 1)
	}

	go func() {
		result := &Job{}
		c.err = c.client.Call("RPC.Submit", j, &result)
		if c.err != nil {
			ch <- nil
		} else {
			ch <- result
		}
	}()
	return ch
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

func (c *Client) Close() error { return c.client.Close() }
