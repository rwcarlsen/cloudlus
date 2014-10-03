package cloudlus

import "fmt"

type RPC struct {
	s *Server
}

func (r *RPC) Heartbeat(b Beat, unused *int) error {
	r.s.beat <- b
	return nil
}

// Submit j via rpc and block until complete returning the result job.
func (r *RPC) Submit(j *Job, result **Job) error {
	*result = r.s.Run(j)
	return nil
}

func (r *RPC) Retrieve(j JobId, result **Job) error {
	var err error
	*result, err = r.s.Get(j)
	if err != nil {
		return err
	}
	return nil
}

func (r *RPC) Fetch(wid WorkerId, j **Job) error {
	fmt.Printf("got work request from worker %v\n", wid)
	req := workRequest{wid, make(chan *Job)}
	r.s.fetchjobs <- req
	*j = <-req.Ch
	if *j == nil {
		return nojoberr
	}

	return nil
}

func (r *RPC) Push(j *Job, unused *int) error {
	fmt.Printf("received job %v back from worker %v\n", j.Id, j.WorkerId)
	r.s.pushjobs <- j
	return nil
}