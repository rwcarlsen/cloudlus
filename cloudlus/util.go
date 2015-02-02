package cloudlus

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/petar/GoLLRB/llrb"
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
	db *leveldb.DB
	// Limit is the cumulative maximum number of bytes that all jobs in the
	// database can occupy without garbage collection (GC) purging jobs from
	// the database.
	Limit int64
	// PurgeAge is the minimum age at which completed (successful and failed) jobs
	// become elegible for removal from the database during GC.
	PurgeAge time.Duration
}

// NewDB returns a new database with a
func NewDB(path string, dblimit int) (*DB, error) {
	d := &DB{PurgeAge: 1 * time.Hour}
	d.Limit = int64(dblimit)

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

// GC runs garbage collection if the database is larger than the specified
// DB.Limit.  Jobs older than DB.PurgeAge are removed if they have been
// completed.  The number of removed jobs and the number of jobs still in the
// database is returned along with any error that occured.
func (d *DB) GC() (npurged, nremain int, err error) {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()

	var size int64
	now := time.Now()

	for it.Next() {
		j := &Job{}
		data := it.Value()
		err := json.Unmarshal(data, &j)
		if err != nil {
			return npurged, nremain, err
		}

		if size > d.Limit && j.Done() && now.Sub(j.Finished) > d.PurgeAge {
			d.db.Delete(it.Key(), nil)
			npurged++
		} else {
			size += int64(len(it.Value()))
			nremain++
		}
	}
	if err := it.Error(); err != nil {
		return npurged, nremain, err
	}

	return npurged, nremain, nil
}

// Size returns the cumulative size of all jobs in the database (uncompressed
// and in json form).
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

// Count returns the number of jobs in the database.
func (d *DB) Count() (int, error) {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()

	njobs := 0
	for it.Next() {
		njobs++
	}
	if err := it.Error(); err != nil {
		return 0, err
	}
	return njobs, nil
}

// DiskSize returns the approximate size of the database on disk.
func (d *DB) DiskSize() (int64, error) {
	sizes, err := d.db.SizeOf(nil)
	if err != nil {
		return 0, err
	}
	return int64(sizes.Sum()), nil
}

func (d *DB) Close() error { return d.db.Close() }

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

type item struct{ *Job }

func (j item) Less(than llrb.Item) bool {
	j2 := than.(item)
	return j.Finished.Before(j2.Finished)
}

// Recent returns up to n of the most recently completed jobs (including
// failed ones) completed within dur of now.
func (d *DB) Recent(n int, dur time.Duration) ([]*Job, error) {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()

	tree := llrb.New()
	now := time.Now()
	for it.Next() {
		j := &Job{}
		data := it.Value()
		err := json.Unmarshal(data, &j)
		if err != nil {
			return nil, err
		}

		if j.Done() && now.Sub(j.Finished) < dur {
			tree.InsertNoReplace(item{j})
			for tree.Len() > n {
				tree.DeleteMin()
			}
		}
	}
	if err := it.Error(); err != nil {
		return nil, err
	}

	jobs := []*Job{}
	for i := 0; i < tree.Len(); i++ {
		j := tree.DeleteMax().(item).Job
		jobs = append(jobs, j)
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
