package badger

import (
	"crypto/rand"
	"fmt"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	badger "github.com/ipfs/go-ds-badger"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var (
	// this benchmark can consume up to 1Gb of disk space
	numObjectsVariants        = []int{100, 1000}
	updatesIterationsVariants = []int{1, 100}
	valSizeVariants           = []int{64, 1000, 10000}
)

func opts(lsmOnly bool) *badger.Options {
	opts := badger.DefaultOptions
	if !lsmOnly {
		opts.ValueThreshold = 32
	}

	opts.GcDiscardRatio = 0.2
	opts.CompactL0OnClose = true
	opts.ValueLogFileSize = 64 * 1024 * 1024

	// disable periodic GC for benchmark purposes, will do it manually
	opts.GcInterval = 0
	opts.GcSleep = 0
	return &opts
}

func DirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size, err
}
func benchmarkWithOpts(b *testing.B, opts *badger.Options, valueSize, numKeys, updateIterations int) {
	temp, err := ioutil.TempDir("/tmp", "badger*")
	require.NoError(b, err)

	var val = make([]byte, valueSize)
	ds, err := badger.NewDatastore(temp, opts)
	require.NoError(b, err)
	start := time.Now()
	b.Run("create records", func(b *testing.B) {
		for j := 0; j < updateIterations; j++ {
			for i := 0; i < numKeys; i++ {
				_, err = rand.Read(val)
				require.NoError(b, err)
				k := datastore.NewKey(fmt.Sprintf("a%d", i))
				err = ds.Put(k, val)
				require.NoError(b, err)
			}
		}
		b.ReportMetric(float64(0), "ns/op")
		b.ReportMetric(float64(updateIterations*numKeys), "records")
		b.ReportMetric(float64(time.Since(start).Nanoseconds())/float64(updateIterations*numKeys), "ns/record")
	})

	// gc all the files
	for {
		if err := ds.DB.RunValueLogGC(0.2); err != nil && err.Error() == "Value log GC attempt didn't result in any cleanup" {
			break
		}
	}

	b.Run("iterate key only", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			res, err := ds.Query(query.Query{
				Prefix:   "",
				Limit:    0,
				Offset:   0,
				KeysOnly: true,
			})
			require.NoError(b, err)
			for _ = range res.Next() {

			}
		}
	})

	b.Run("iterate", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			res, err := ds.Query(query.Query{
				Prefix:   "",
				Limit:    0,
				Offset:   0,
				KeysOnly: false,
			})
			require.NoError(b, err)
			for _ = range res.Next() {

			}
		}
	})

	err = ds.Close()
	require.NoError(b, err)
	printDbSize(b, temp, opts)
	b.Logf("one more gc cycle after instance reload...")
	ds, err = badger.NewDatastore(temp, opts)
	require.NoError(b, err)
	// one more GC cycle
	for {
		if err := ds.DB.RunValueLogGC(0.2); err != nil && err.Error() == "Value log GC attempt didn't result in any cleanup" {
			break
		}
	}
	err = ds.Close()
	require.NoError(b, err)
	printDbSize(b, temp, opts)

	err = os.RemoveAll(temp)
	require.NoError(b, err)
}

func printDbSize(b *testing.B, path string, opts *badger.Options) {
	ds, err := badger.NewDatastore(path, opts)
	require.NoError(b, err)
	lsmSize, valSize := ds.DB.Size()
	err = ds.Close()
	require.NoError(b, err)
	dirSize, err := DirSize(path)
	require.NoError(b, err)
	b.Logf("Dir size: %.2fMb. Db size: lsm %.2fMb, val %.2fMb", float64(dirSize)/1024/1024, float64(lsmSize)/1024/1024, float64(valSize)/1024/1024)
}

func Benchmark_Badger(b *testing.B) {
	for _, numObjects := range numObjectsVariants {
		for _, valSize := range valSizeVariants {
			for _, updatesIterations := range updatesIterationsVariants {
				b.Run(fmt.Sprintf("%dx%dB", numObjects, valSize), func(b *testing.B) {
					benchmarkWithOpts(b, opts(false), numObjects, valSize, updatesIterations)
				})
			}
		}
	}
}

func Benchmark_BadgerLSMOnly(b *testing.B) {
	for _, numObjects := range numObjectsVariants {
		for _, valSize := range valSizeVariants {
			for _, updatesIterations := range updatesIterationsVariants {
				b.Run(fmt.Sprintf("%dx%dB", numObjects, valSize), func(b *testing.B) {
					benchmarkWithOpts(b, opts(true), numObjects, valSize, updatesIterations)
				})
			}
		}
	}
}
