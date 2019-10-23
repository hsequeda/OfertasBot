package main

import (
	"encoding/json"
	"fmt"
	"github.com/boltdb/bolt"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/snapcore/snapd/osutil"
	"time"
)

var db *bolt.DB

const (
	dbpah    = "data.db"
	dbbucket = "records"
)

var (
	errBucketEmpty = errors.New("Error: bucket are empty.")
	errValEmpty    = errors.New("Error: value are empty.")
)

func init() {
	if !osutil.FileExists(dbpah) {
		if err := startUp(false); err != nil {
			logrus.Fatal(err)
		}
		defer func() {
			if err := db.Close(); err != nil {
				logrus.Fatalf("Error close db: %s", err)
			}
		}()
	}
}

func startUp(readOnly bool) error {
	var err error
	db, err = bolt.Open(dbpah,
		0666,
		&bolt.Options{
			Timeout:  1 * time.Second,
			ReadOnly: readOnly,
		})
	if err != nil {
		return fmt.Errorf("Error opening the db: %s", err)
	}
	return nil
}

func dbWrite(key, val []byte) error {
	if err := startUp(false); err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			logrus.Fatalf("Error close db: %s", err)
		}
	}()

	return db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(dbbucket))
		if err != nil {
			return err
		}
		if err = b.Put(key, val); err != nil {
			return err
		}
		return nil
	})
}

func dbRead(key string) ([]byte, error) {
	err := startUp(true)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := db.Close(); err != nil {
			logrus.Fatalf("Error close db: %s", err)
		}
	}()

	var val []byte
	if err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(dbbucket))
		if b == nil {
			return errBucketEmpty
		}
		result := b.Get([]byte(key))
		val = make([]byte, len(result))
		copy(val, result)
		if val == nil {
			return errValEmpty
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return val, nil
}

func dbDelete(key string) error {
	err := startUp(false)
	if err != nil {
		return err
	}

	defer func() {
		if err := db.Close(); err != nil {
			logrus.Fatalf("Error close db: %s", err)
		}
	}()

	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(dbbucket))
		if b == nil {
			return errBucketEmpty
		}
		return b.Delete([]byte(key))
	})
}

func Insert(id, value string) error {
	list, err := Get(id)
	if err != nil && err != errBucketEmpty && err != errValEmpty {
		return err
	}

	list = append(list, value)
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}

	if err = dbWrite([]byte(id), b); err != nil {
		return err
	}
	return nil
}

func Get(id string) ([]string, error) {
	bb, err := dbRead(id)
	var result []string

	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(bb, &result); err != nil {
		return nil, err
	}
	return result, nil
}
