package cloudlus

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/rwcarlsen/lru"
	"github.com/syndtr/goleveldb/leveldb"
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
	cache     *lru.Cache
	dblimiter *lru.Cache
	db        *leveldb.DB
}

func NewDB(path string, cachelimit, dblimit int) (*DB, error) {
	d := &DB{}

	d.cache = lru.New(int64(cachelimit))
	d.cache.OnMiss(d.cacheMiss)

	d.dblimiter = lru.New(int64(dblimit))
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, err
	}
	d.db = db

	// populate dblimiter lru cache with currently existing jobs from the disk
	// leveldb
	it := db.NewIterator(nil, nil)
	defer it.Release()
	count := 0
	for it.Next() {
		count++
		j := &Job{}
		data := it.Value()
		err := json.Unmarshal(data, &j)
		if err != nil {
			return nil, err
		}
		d.dblimiter.Set(j.Id.String(), &jobProxy{j.Id, int64(len(data)), d})
	}
	if err := it.Error(); err != nil {
		return nil, err
	}
	fmt.Printf("loaded %v jobs from disk database\n", count)
	fmt.Printf("disk job store is %v%% full\n", float64(d.dblimiter.Size())/float64(dblimit))

	return d, nil
}

func (d *DB) Get(id JobId) (*Job, error) {
	v, err := d.cache.Get(id.String())
	if err != nil {
		return nil, err
	}
	return v.(*Job), nil
}

func (d *DB) Put(j *Job) error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}

	err = d.db.Put(j.Id[:], data, nil)
	if err != nil {
		return err
	}

	d.dblimiter.Set(j.Id.String(), &jobProxy{j.Id, int64(len(data)), d})

	d.cache.Set(j.Id.String(), j)
	return nil
}

func (d *DB) Items() ([]lru.Cacheable, error) { return d.cache.Items() }

func (d *DB) cacheMiss(idstr string) (lru.Cacheable, error) {
	id, err := hex.DecodeString(idstr)
	if err != nil {
		return nil, err
	}

	data, err := d.db.Get(id, nil)
	j := &Job{}
	err = json.Unmarshal(data, &j)
	if err != nil {
		return nil, err
	}
	return j, nil
}

type jobProxy struct {
	jid  JobId
	size int64
	d    *DB
}

func (jp *jobProxy) Size() int64 { return jp.size }

func (jp *jobProxy) OnPurge(why lru.PurgeReason) {
	if why != lru.CACHEFULL {
		return
	}

	err := jp.d.db.Delete(jp.jid[:], nil)
	if err != nil {
		log.Print(err)
	}
}
