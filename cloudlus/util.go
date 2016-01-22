package cloudlus

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type Beat struct {
	Time     time.Time
	WorkerId WorkerId
	JobId    JobId
	kill     chan bool
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

func DecodeJobId(s string) (JobId, error) {
	var id JobId
	buf, err := hex.DecodeString(s)
	if err != nil {
		return id, err
	}

	if n := copy(id[:], buf); n < len(id) {
		return JobId{}, fmt.Errorf("invalid JobId string length %v", n)
	}
	return id, nil
}

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
	d := &DB{PurgeAge: 30 * time.Minute}
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
// database is returned along with any error that occured.  sometimes, -1 may
// be returned for nremain - this means that the jobs count is unknown because
// GC didn't occur.
func (d *DB) GC() (npurged, nremain int, err error) {
	size, err := d.Size()
	if err != nil {
		return 0, -1, err
	} else if size < int64(d.Limit) {
		return 0, -1, nil
	}

	it := d.db.NewIterator(nil, nil)
	defer it.Release()

	now := time.Now()
	for it.Next() {
		if notjob(it.Key()) {
			// TODO: test that non-job key entries are properly skipped
			continue
		}

		j := &Job{}
		data := it.Value()
		err := json.Unmarshal(data, &j)
		if err != nil {
			return npurged, -1, err
		}

		if j.Done() && now.Sub(j.Finished) > d.PurgeAge {
			os.Remove(outfileName(j.Id))
			d.db.Delete(it.Key(), nil)
			d.db.Delete(finishKey(j), nil)
			d.db.Delete(currentKey(j), nil)
			npurged++
		} else {
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
		if notjob(it.Key()) {
			continue
		}
		njobs++
	}
	if err := it.Error(); err != nil {
		return 0, err
	}
	return njobs, nil
}

func (d *DB) Close() error { return d.db.Close() }

func notjob(key []byte) bool {
	pfx1 := []byte(finishPrefix)
	pfx2 := []byte(currPrefix)
	if bytes.Equal(key[:len(pfx1)], pfx1) {
		return true
	} else if bytes.Equal(key[:len(pfx2)], pfx2) {
		return true
	}
	return false
}

// Failed returns the all jobs from the database that failed.
func (d *DB) Failed() ([]*Job, error) {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()

	jobs := []*Job{}
	for it.Next() {
		// TODO: test that non-job key entries are properly skipped
		if notjob(it.Key()) {
			continue
		}

		j := &Job{}
		err := json.Unmarshal(it.Value(), &j)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	if err := it.Error(); err != nil {
		return nil, err
	}
	return jobs, nil
}

// Current returns the all jobs from the database that aren't completed - e.g.
// queued or running.
func (d *DB) Current() ([]*Job, error) {
	it := d.db.NewIterator(util.BytesPrefix([]byte(currPrefix)), nil)
	defer it.Release()

	ids := []JobId{}
	for it.Next() {
		var id JobId
		copy(id[:], it.Value())
		ids = append(ids, id)
	}
	if err := it.Error(); err != nil {
		return nil, err
	}

	jobs := make([]*Job, len(ids))
	for i, id := range ids {
		j, err := d.Get(id)
		if err != nil {
			return nil, err
		}
		jobs[i] = j
	}

	return jobs, nil
}

// Recent returns up to n of the most recently completed jobs (including
// failed ones).
func (d *DB) Recent(n int) ([]*Job, error) {
	it := d.db.NewIterator(util.BytesPrefix([]byte(finishPrefix)), nil)
	defer it.Release()

	// the last iterated over jobs are the most recent
	ids := []JobId{}
	for it.Next() {
		var id JobId
		copy(id[:], it.Value())
		ids = append(ids, id)
	}
	if err := it.Error(); err != nil {
		return nil, err
	}

	if len(ids) > n {
		ids = ids[len(ids)-n:]
	}

	jobs := make([]*Job, len(ids))
	for i, id := range ids {
		j, err := d.Get(id)
		if err != nil {
			return nil, err
		}
		jobs[i] = j
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

const finishPrefix = "finish-"
const currPrefix = "curr-"

func finishKey(j *Job) []byte {
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, uint64(j.Finished.Unix()))
	key := append([]byte(finishPrefix), data...)
	key = append(key, '-')
	return append(key, j.Id[:]...)
}

func currentKey(j *Job) []byte {
	return append([]byte(currPrefix), j.Id[:]...)
}

func (d *DB) Put(j *Job) error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}

	// current index
	if j.Done() {
		d.db.Delete(currentKey(j), nil)
	} else {
		err = d.db.Put(currentKey(j), j.Id[:], nil)
		if err != nil {
			return err
		}
	}

	// time finished index
	if j.Done() && j.Finished.Unix() >= 0 {
		// TODO: test that we don't add entries for unfinished jobs - they have a
		// negative unix time and mess up the iteration order.
		err = d.db.Put(finishKey(j), j.Id[:], nil)
		if err != nil {
			return err
		}
	}

	return d.db.Put(j.Id[:], data, nil)
}

func outfileName(id JobId) string {
	return fmt.Sprintf("%s-outdata.zip", id)
}
