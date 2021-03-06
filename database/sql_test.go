package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/baetyl/baetyl-go/utils"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
)

type dummy struct {
	ID   uint64
	Data string
}

type dummyEncoder struct{}

func (e *dummyEncoder) Encode(v interface{}) []byte {
	d, _ := json.Marshal(v)
	return d
}

func (e *dummyEncoder) Decode(value []byte, others ...interface{}) interface{} {
	v := new(dummy)
	json.Unmarshal(value, v)
	if len(others) > 0 {
		v.ID = others[0].(uint64)
	}
	return v
}

func getFileSize(name string) (int64, error) {
	fi, err := os.Stat(name)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

func TestDatabaseDriveNotFound(t *testing.T) {
	db, err := New(Conf{Driver: "does not exist", Source: "t.db"}, nil)
	assert.EqualError(t, err, "database driver not found")
	assert.Nil(t, db)
}

func TestDatabaseDriveReopen(t *testing.T) {
	dir, err := ioutil.TempDir("", t.Name())
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	file := path.Join(dir, "t.db")
	db1, err := New(Conf{Driver: "sqlite3", Source: file}, &dummyEncoder{})
	assert.NoError(t, err)

	db2, err := New(Conf{Driver: "sqlite3", Source: file}, &dummyEncoder{})
	assert.NoError(t, err)

	value := &dummy{
		ID:   1,
		Data: "hi",
	}
	err = db1.Put([]interface{}{value})
	assert.NoError(t, err)
	err = db2.Put([]interface{}{value})
	assert.NoError(t, err)

	values, err := db1.Get(0, 10)
	assert.NoError(t, err)
	assert.Len(t, values, 2)

	db2.Close(false)
	assert.True(t, utils.FileExists(file))

	values, err = db1.Get(0, 10)
	assert.NoError(t, err)
	assert.Len(t, values, 2)

	db1.Close(true)
	assert.False(t, utils.FileExists(file))

	values, err = db1.Get(0, 10)
	assert.EqualError(t, err, "sql: database is closed")
	assert.Len(t, values, 0)
}

func TestDatabaseSQLite(t *testing.T) {
	dir, err := ioutil.TempDir("", t.Name())
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	db, err := New(Conf{Driver: "sqlite3", Source: path.Join(dir, "t.db")}, &dummyEncoder{})
	assert.NoError(t, err)
	assert.NotNil(t, db)
	defer db.Close(false)

	values, err := db.Get(0, 1)
	assert.NoError(t, err)
	assert.Len(t, values, 0)

	value := &dummy{
		ID:   111,
		Data: "hi",
	}
	err = db.Put([]interface{}{value})
	assert.NoError(t, err)

	values, err = db.Get(0, 10)
	assert.NoError(t, err)
	assert.Len(t, values, 1)
	assert.Equal(t, uint64(1), values[0].(*dummy).ID)
	assert.Equal(t, "hi", values[0].(*dummy).Data)

	err = db.Del([]uint64{0, 1})
	assert.NoError(t, err)
	values, err = db.Get(0, 10)
	assert.NoError(t, err)
	assert.Len(t, values, 0)

	err = db.Put([]interface{}{value, value, value, value, value})
	assert.NoError(t, err)

	values, err = db.Get(0, 10)
	assert.NoError(t, err)
	assert.Len(t, values, 5)
	assert.Equal(t, uint64(2), values[0].(*dummy).ID)
	assert.Equal(t, uint64(3), values[1].(*dummy).ID)
	assert.Equal(t, uint64(4), values[2].(*dummy).ID)
	assert.Equal(t, uint64(5), values[3].(*dummy).ID)
	assert.Equal(t, uint64(6), values[4].(*dummy).ID)

	values, err = db.Get(5, 10)
	assert.NoError(t, err)
	assert.Len(t, values, 2)
	assert.Equal(t, uint64(5), values[0].(*dummy).ID)
	assert.Equal(t, uint64(6), values[1].(*dummy).ID)

	err = db.Del([]uint64{5, 6})
	assert.NoError(t, err)
	values, err = db.Get(5, 10)
	assert.NoError(t, err)
	assert.Len(t, values, 0)
}

func TestDatabaseSQLiteNoEncoder(t *testing.T) {
	dir, err := ioutil.TempDir("", t.Name())
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	db, err := New(Conf{Driver: "sqlite3", Source: path.Join(dir, "t.db")}, nil)
	assert.NoError(t, err)
	assert.NotNil(t, db)
	defer db.Close(false)

	values, err := db.Get(0, 1)
	assert.NoError(t, err)
	assert.Len(t, values, 0)

	value := []byte("hi")
	err = db.Put([]interface{}{value})
	assert.NoError(t, err)

	values, err = db.Get(0, 10)
	assert.NoError(t, err)
	assert.Len(t, values, 1)
	assert.Equal(t, value, values[0])

	err = db.Del([]uint64{0, 1})
	assert.NoError(t, err)
	values, err = db.Get(0, 10)
	assert.NoError(t, err)
	assert.Len(t, values, 0)

	err = db.Put([]interface{}{value, value, value, value, value})
	assert.NoError(t, err)

	values, err = db.Get(0, 10)
	assert.NoError(t, err)
	assert.Len(t, values, 5)
	assert.Equal(t, value, values[0])

	values, err = db.Get(5, 10)
	assert.NoError(t, err)
	assert.Len(t, values, 2)
	assert.Equal(t, value, values[0])

	err = db.Del([]uint64{5, 6})
	assert.NoError(t, err)
	values, err = db.Get(5, 10)
	assert.NoError(t, err)
	assert.Len(t, values, 0)
}

func TestSQLiteDelExpiredData(t *testing.T) {
	dir, err := ioutil.TempDir("", t.Name())
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	file := path.Join(dir, "kv.db")
	db, err := New(Conf{Driver: "sqlite3", Source: file}, nil)
	assert.NoError(t, err)
	assert.NotNil(t, db)
	defer db.Close(false)

	var a []interface{}
	value := make([]byte, 1024)
	for i := 0; i < 100; i++ {
		a = append(a, value)
	}
	err = db.Put(a)
	assert.NoError(t, err)

	v, err := db.Get(0, 1000)
	assert.NoError(t, err)
	assert.Len(t, v, 100)

	s1, err := getFileSize(file)
	assert.NoError(t, err)

	ids := make([]uint64, 50)
	for i := 0; i < 50; i++ {
		ids = append(ids, uint64(i))
	}
	err = db.Del(ids)
	assert.NoError(t, err)

	s2, err := getFileSize(file)
	assert.NoError(t, err)
	assert.Equal(t, s1, s2)

	time.Sleep(100 * time.Millisecond)
	err = db.DelBefore(time.Now())

	s3, err := getFileSize(file)
	assert.NoError(t, err)
	assert.Less(t, s3, s1)

	v, err = db.Get(0, 1000)
	assert.NoError(t, err)
	assert.Len(t, v, 0)
}

func TestDatabaseSQLiteKV(t *testing.T) {
	dir, err := ioutil.TempDir("", t.Name())
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	db, err := New(Conf{Driver: "sqlite3", Source: path.Join(dir, "kv.db")}, &dummyEncoder{})
	assert.NoError(t, err)
	assert.NotNil(t, db)
	defer db.Close(false)

	k1 := "k1"
	k2 := "k2"
	v1 := &dummy{
		ID:   1,
		Data: "1",
	}
	v2 := &dummy{
		ID:   2,
		Data: "2",
	}

	// list empty db
	vs, err := db.ListKV()
	assert.NoError(t, err)
	assert.Empty(t, vs)

	// k1 does not exist
	v, err := db.GetKV(k1)
	assert.NoError(t, err)
	assert.Nil(t, v)

	// set k1
	err = db.SetKV(k1, v1)
	assert.NoError(t, err)

	// list db
	vs, err = db.ListKV()
	assert.NoError(t, err)
	assert.Len(t, vs, 1)
	assert.Equal(t, v1, vs[0])

	// k1 exists
	v, err = db.GetKV(k1)
	assert.NoError(t, err)
	assert.Equal(t, v1, v)

	// set k2
	err = db.SetKV(k2, v2)
	assert.NoError(t, err)

	// list db
	vs, err = db.ListKV()
	assert.NoError(t, err)
	assert.Len(t, vs, 2)

	// set k1 again
	err = db.SetKV(k1, v2)
	assert.NoError(t, err)

	// list db
	vs, err = db.ListKV()
	assert.NoError(t, err)
	assert.Len(t, vs, 2)

	// k1 exists
	v, err = db.GetKV(k1)
	assert.NoError(t, err)
	assert.Equal(t, v2, v)

	// delete k1
	err = db.DelKV(k1)
	assert.NoError(t, err)

	// k1 does not exist
	v, err = db.GetKV(k1)
	assert.NoError(t, err)
	assert.Nil(t, v)

	// delete k1 again
	err = db.DelKV(k1)
	assert.NoError(t, err)

	// delete k2
	err = db.DelKV(k2)
	assert.NoError(t, err)

	// list empty db
	vs, err = db.ListKV()
	assert.NoError(t, err)
	assert.Empty(t, vs)
}

func TestDatabaseSQLiteKVNoEncoder(t *testing.T) {
	dir, err := ioutil.TempDir("", t.Name())
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	db, err := New(Conf{Driver: "sqlite3", Source: path.Join(dir, "kv.db")}, nil)
	assert.NoError(t, err)
	assert.NotNil(t, db)
	defer db.Close(false)

	k1 := []byte("k1")
	k2 := []byte("k2")

	// list empty db
	vs, err := db.ListKV()
	assert.NoError(t, err)
	assert.Empty(t, vs)

	// k1 does not exist
	v, err := db.GetKV(k1)
	assert.NoError(t, err)
	assert.Nil(t, v)

	// set k1
	err = db.SetKV(k1, k1)
	assert.NoError(t, err)

	// list db
	vs, err = db.ListKV()
	assert.NoError(t, err)
	assert.Len(t, vs, 1)
	assert.Equal(t, k1, vs[0])

	// k1 exists
	v, err = db.GetKV(k1)
	assert.NoError(t, err)
	assert.Equal(t, k1, v)

	// set k2
	err = db.SetKV(k2, k2)
	assert.NoError(t, err)

	// list db
	vs, err = db.ListKV()
	assert.NoError(t, err)
	assert.Len(t, vs, 2)

	// set k1 again
	err = db.SetKV(k1, k2)
	assert.NoError(t, err)

	// list db
	vs, err = db.ListKV()
	assert.NoError(t, err)
	assert.Len(t, vs, 2)

	// k1 exists
	v, err = db.GetKV(k1)
	assert.NoError(t, err)
	assert.Equal(t, k2, v)

	// delete k1
	err = db.DelKV(k1)
	assert.NoError(t, err)

	// k1 does not exist
	v, err = db.GetKV(k1)
	assert.NoError(t, err)
	assert.Nil(t, v)

	// delete k1 again
	err = db.DelKV(k1)
	assert.NoError(t, err)

	// delete k2
	err = db.DelKV(k2)
	assert.NoError(t, err)

	// list empty db
	vs, err = db.ListKV()
	assert.NoError(t, err)
	assert.Empty(t, vs)
}

func BenchmarkDatabaseSQLite(b *testing.B) {
	dir, err := ioutil.TempDir("", "")
	assert.NoError(b, err)
	defer os.RemoveAll(dir)

	db, err := New(Conf{Driver: "sqlite3", Source: path.Join(dir, "t.db")}, &dummyEncoder{})
	assert.NoError(b, err)
	assert.NotNil(b, db)
	defer db.Close(false)

	values, err := db.Get(0, 1)
	assert.NoError(b, err)
	assert.Len(b, values, 0)

	value := &dummy{
		ID:   111,
		Data: "hi",
	}

	b.ResetTimer()
	b.Run("Put", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			db.Put([]interface{}{value})
		}
	})
	b.Run("Get", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			db.Get(1, 10)
		}
	})
	b.Run("Del", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			db.Del([]uint64{uint64(i)})
		}
	})
}

func TestDatabaseSQLiteData(t *testing.T) {
	t.Skip("only for dev test")

	db, err := New(Conf{Driver: "sqlite3", Source: "queue4.db"}, &dummyEncoder{})
	assert.NoError(t, err)
	assert.NotNil(t, db)
	defer db.Close(false)

	value := &dummy{
		ID:   1,
		Data: "hi",
	}
	for i := 0; i < 10000; i++ {
		db.Put([]interface{}{value})
	}
}

func TestSQLite3MultiDBsVSMultiTables(t *testing.T) {
	t.Skip("only for dev test")

	dir, err := ioutil.TempDir("", t.Name())
	assert.NoError(t, err)
	defer os.RemoveAll(dir)
	t.Log(dir)

	createTable := `CREATE TABLE IF NOT EXISTS t%d (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		value TEXT,
		ts TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`
	insertValue := "insert into t%d(value) values (?)"

	count := 10
	var dbs []*sql.DB
	tables, err := sql.Open("sqlite3", path.Join(dir, "t.db"))
	assert.NoError(t, err)
	for index := 0; index < count; index++ {
		_, err = tables.Exec(fmt.Sprintf(createTable, index))
		assert.NoError(t, err)

		db, err := sql.Open("sqlite3", path.Join(dir, fmt.Sprintf("%d.db", index)))
		assert.NoError(t, err)
		_, err = db.Exec(fmt.Sprintf(createTable, 0))
		assert.NoError(t, err)
		dbs = append(dbs, db)
	}

	messages := 1000
	var wg1 sync.WaitGroup
	start := time.Now()
	wg1.Add(count)
	for index := 0; index < count; index++ {
		go func(wg *sync.WaitGroup, db *sql.DB) {
			defer wg.Done()
			for i := 0; i < messages; i++ {
				db.Exec(fmt.Sprintf(insertValue, 0), "aaaaaaaaaa")
			}
		}(&wg1, dbs[index])
	}
	wg1.Wait()
	t.Log("dbs", time.Since(start))

	var wg2 sync.WaitGroup
	start = time.Now()
	wg2.Add(count)
	for index := 0; index < count; index++ {
		go func(wg *sync.WaitGroup, db *sql.DB, t int) {
			defer wg.Done()
			for i := 0; i < messages; i++ {
				db.Exec(fmt.Sprintf(insertValue, t), "aaaaaaaaaa")
			}
		}(&wg2, tables, index)
	}
	wg2.Wait()
	t.Log("tables", time.Since(start))
}
