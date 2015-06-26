package post

import (
	"database/sql"
	"io"
	"os"
	"testing"
)

func TestProcess(t *testing.T) {
	dbname := "/home/r/eg23.sqlite"
	err := copydb(dbname, "tmp.sqlite")
	if err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite3", dbname)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = Process(db)
	if err != nil {
		t.Fatal(err)
	}
}

func copydb(src, dst string) error {
	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer d.Close()

	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer s.Close()

	_, err = io.Copy(d, s)
	return err
}
