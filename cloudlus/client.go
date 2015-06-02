package cloudlus

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/rpc"
	"strings"
	"time"
)

type Client struct {
	client *rpc.Client
	err    error
	addr   string
}

func Dial(addr string) (*Client, error) {
	if !strings.Contains(addr, ":") {
		addr += ":80"
	}
	client, err := rpc.DialHTTP("tcp", addr)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(addr, "http://") {
		addr = "http://" + addr
	}
	return &Client{client: client, addr: addr}, nil
}

func (c *Client) Heartbeat(w WorkerId, j JobId, done chan struct{}) (kill chan bool) {
	kill = make(chan bool, 1)
	go func() {
		tick := time.NewTicker(beatInterval)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				var killval bool
				err := c.client.Call("RPC.Heartbeat", NewBeat(w, j), &killval)
				if err != nil {
					log.Print(err)
					return
				} else if killval {
					kill <- true
					return
				}
			case <-done:
				return
			}
		}
	}()
	return kill
}

func (c *Client) Retrieve(j JobId) (*Job, error) {
	var result *Job
	err := c.client.Call("RPC.Retrieve", j, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) PushOutfile(j JobId, r io.Reader) error {
	path := "/api/v1/job-outfiles/" + j.String()

	req, err := http.NewRequest("POST", c.addr+path, r)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func (c *Client) RetrieveOutfile(j JobId) (io.ReadCloser, error) {
	path := "/api/v1/job-outfiles/" + j.String()
	resp, err := http.Get(c.addr + path)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (c *Client) RetrieveOutfileData(j *Job, fname string) ([]byte, error) {
	path := "/api/v1/job-outfiles/" + j.Id.String()
	resp, err := http.Get(c.addr + path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	rc, err := j.GetOutfile(bytes.NewReader(data), len(data), fname)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	return ioutil.ReadAll(rc)
}

func (c *Client) Submit(j *Job) error {
	var unused int
	return c.client.Call("RPC.SubmitAsync", j, &unused)
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

// Start submits j and returns a channel where the completed job can be
// retrieved from.  If the the program doesn't block on the channel, there is
// no guarantee that the job will be submitted.  For asynchronous submission,
// use the Submit method.
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
