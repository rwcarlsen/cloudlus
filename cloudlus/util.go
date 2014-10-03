package cloudlus

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type Beat struct {
	Time     time.Time
	WorkerId WorkerId
	JobId    JobId
}

func NewBeat(w WorkerId, j JobId) Beat {
	return Beat{Time: time.Now(), WorkerId: w, JobId: j}
}

type WorkerId [16]byte

func (i WorkerId) MarshalJSON() ([]byte, error) {
	return []byte("\"" + hex.EncodeToString(i[:]) + "\""), nil
}

func (i *WorkerId) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), "\"")
	bs, err := hex.DecodeString(s)
	if err != nil {
		return err
	}
	if n := copy((*i)[:], bs); n < len(i) {
		return fmt.Errorf("JSON WorkerId has invalid length %v", n)
	}
	return nil
}

func (i WorkerId) String() string { return hex.EncodeToString(i[:]) }

type JobId [16]byte

func (i JobId) MarshalJSON() ([]byte, error) {
	return []byte("\"" + hex.EncodeToString(i[:]) + "\""), nil
}

func (i *JobId) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), "\"")
	bs, err := hex.DecodeString(s)
	if err != nil {
		return err
	}
	if n := copy((*i)[:], bs); n < len(i) {
		return fmt.Errorf("JSON JobId has invalid length %v", n)
	}
	return nil
}

func (i JobId) String() string { return hex.EncodeToString(i[:]) }
