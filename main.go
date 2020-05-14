package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/nilp0inter/MiSTer_WebMenu_DataBank/rdb"

	"github.com/thetannerryan/ring"
	bolt "go.etcd.io/bbolt"
)

var outputDB string = "databank.db"
var outputJsonDir string = "db"
var bloomFalsePositive float64 = 0.001
var bloomBucket = "BLOOM"
var md5Bucket = "MD5"

func main() {

	db, err := bolt.Open(outputDB, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	count := 0
	for _, entry := range os.Args[1:] {
		pcount, err := dumpRDB(db, entry)
		if err != nil {
			log.Fatal(err.Error())
		}
		count += pcount
	}

	crc_ring, err := ring.Init(count, bloomFalsePositive)
	if err != nil {
		log.Fatal(err)
	}

	size_ring, err := ring.Init(count, bloomFalsePositive)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Calculating bloom filters for %d elements...\n", count)
	for _, entry := range os.Args[1:] {
		err = loadRings(crc_ring, size_ring, entry)
		if err != nil {
			log.Fatal(err.Error())
		}
	}
	err = db.Update(func(tx *bolt.Tx) error {
		bloom, err := tx.CreateBucketIfNotExists([]byte(bloomBucket))

		crc_bloom, err := crc_ring.MarshalBinary()
		if err != nil {
			return err
		}
		err = bloom.Put([]byte("crc"), crc_bloom)
		if err != nil {
			return err
		}

		size_bloom, err := size_ring.MarshalBinary()
		if err != nil {
			return err
		}
		err = bloom.Put([]byte("size"), size_bloom)
		if err != nil {
			return err
		}

		fmt.Printf("CRC: %d | SIZE: %d\n", len(crc_bloom), len(size_bloom))
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}

func loadRings(crc_ring, size_ring *ring.Ring, filename string) error {
	bytes, _ := ioutil.ReadFile(filename)
	var games = rdb.Parse(bytes)
	buf_crc := make([]byte, 4)
	buf_size := make([]byte, 8)
	for _, g := range games {
		binary.LittleEndian.PutUint32(buf_crc[:], g.CRC32)
		crc_ring.Add(buf_crc)
		binary.LittleEndian.PutUint64(buf_size[:], g.Size)
		size_ring.Add(buf_size)
	}
	return nil
}

func fileNameWithoutExtension(fileName string) string {
	return strings.TrimSuffix(fileName, filepath.Ext(fileName))
}

func dumpRDB(db *bolt.DB, filename string) (int, error) {
	fmt.Println("Dumping", filename)
	bytes, _ := ioutil.ReadFile(filename)
	var games = rdb.Parse(bytes)
	system := fileNameWithoutExtension(filepath.Base(filename))

	i := 0
	err := db.Batch(func(tx *bolt.Tx) error {
		bmd5, err := tx.CreateBucketIfNotExists([]byte(md5Bucket))
		if err != nil {
			return err
		}
		for _, g := range games {
			g.System = system
			if g.MD5 == "" || g.Name == "" || g.Size == 0 {
				fmt.Fprintln(os.Stderr, "Missing mandatory field: ", g)
				continue
			}
			g.MD5 = strings.ToLower(g.MD5)
			pay, err := json.Marshal(g)
			if err != nil {
				return err
			}
			l1 := g.MD5[0:2]
			l2 := g.MD5[2:4]
			path := filepath.Join(outputJsonDir, l1, l2)
			os.MkdirAll(path, os.ModePerm)
			filename := filepath.Join(path, g.MD5[4:]+".json")
			ioutil.WriteFile(filename, pay, 0644)
			err = bmd5.Put([]byte(g.MD5), []byte(g.System+";"+g.Name))
			if err != nil {
				return err
			}
			i += 1
		}
		return nil
	})
	return i, err
}

