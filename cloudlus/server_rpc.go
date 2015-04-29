package cloudlus

import "time"

type RPC struct {
	s *Server
}

func (r *RPC) Heartbeat(b Beat, kill *bool) error {
	b.Time = time.Now()
	b.kill = make(chan bool)
	r.s.beat <- b
	*kill = <-b.kill
	return nil
}

// Submit j via rpc and block until complete returning the result job.
func (r *RPC) Submit(j *Job, result **Job) error {
	*result = r.s.Run(j)
	return nil
}

// Submit j via rpc asynchronously.
func (r *RPC) SubmitAsync(j *Job, unused *int) error {
	r.s.Start(j, nil)
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
	req := workRequest{wid, make(chan *Job)}
	r.s.fetchjobs <- req
	*j = <-req.Ch
	if *j == nil {
		return nojoberr
	}

	return nil
}

func (r *RPC) Push(j *Job, unused *int) error {
	r.s.pushjobs <- j
	return nil
}
