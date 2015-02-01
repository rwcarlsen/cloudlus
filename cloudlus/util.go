package cloudlus

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
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

type DB struct {
	db       *leveldb.DB
	Log      *log.Logger
	limit    int64 // max number of bytes for the leveldb db
	purgeAge time.Duration
}

func NewDB(path string, dblimit int) (*DB, error) {
	d := &DB{purgeAge: 1 * time.Hour}
	d.limit = int64(dblimit)

	var err error
	var db *leveldb.DB
	if path == "" { // use in-memory db
		db, err = leveldb.Open(storage.NewMemStorage(), nil)
		d.db = db
	} else {
		db, err = leveldb.OpenFile(path, nil)
		if err != nil {
			return nil, err
		}
		d.db = db
	}
	return d, nil
}

func (d *DB) CollectGarbage() error {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()

	count := 0
	purgecount := 0
	var size int64
	now := time.Now()

	for it.Next() {
		j := &Job{}
		data := it.Value()
		err := json.Unmarshal(data, &j)
		if err != nil {
			return err
		}

		if size > d.limit && j.Done() && now.Sub(j.Finished) > d.purgeAge {
			d.db.Delete(it.Key(), nil)
			purgecount++
		} else {
			size += int64(len(it.Value()))
			count++
		}
	}
	if err := it.Error(); err != nil {
		return err
	}

	d.Log.Printf("[INFO] purged %v old jobs from the disk db", purgecount)
	d.Log.Printf("[INFO] disk db holds %v jobs", count)
	return nil
}

func (d *DB) Size() (int64, error) {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()

	var size int64
	for it.Next() {
		size += int64(len(it.Value()))
	}
	if err := it.Error(); err != nil {
		return 0, err
	}
	return size, nil
}

func (d *DB) DiskSize() (int64, error) {
	sizes, err := d.db.SizeOf(nil)
	if err != nil {
		return 0, err
	}
	return int64(sizes.Sum()), nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

// Current returns the all jobs from the database that aren't completed - e.g.
// queued or running.
func (d *DB) Current() ([]*Job, error) {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()

	queue := []*Job{}
	for it.Next() {
		j := &Job{}
		data := it.Value()
		err := json.Unmarshal(data, &j)
		if err != nil {
			return nil, err
		}

		if !j.Done() {
			queue = append(queue, j)
		}
	}
	if err := it.Error(); err != nil {
		return nil, err
	}
	return queue, nil
}

// Recent returns all completed jobs (including failed ones).
// completed within dur of now.
func (d *DB) Recent(dur time.Duration) ([]*Job, error) {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()

	jobs := []*Job{}
	now := time.Now()
	for it.Next() {
		j := &Job{}
		data := it.Value()
		err := json.Unmarshal(data, &j)
		if err != nil {
			return nil, err
		}

		if j.Done() && now.Sub(j.Finished) < dur {
			jobs = append(jobs, j)
		}
	}
	if err := it.Error(); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (d *DB) Get(id JobId) (*Job, error) {
	data, err := d.db.Get(id[:], nil)
	if err != nil {
		return nil, err
	}
	j := &Job{}
	err = json.Unmarshal(data, &j)
	if err != nil {
		return nil, err
	}
	return j, nil
}

func (d *DB) Put(j *Job) error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return d.db.Put(j.Id[:], data, nil)
}
