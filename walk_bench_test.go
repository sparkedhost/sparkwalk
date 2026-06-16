package walk_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	walk "github.com/sparkedhost/sparkwalk"
)

const benchDirEnv = "SPARKWALK_BENCH_DIR"

var benchResultSink benchWalkResult

type benchWalkResult struct {
	bytes int64
	files int64
	dirs  int64
}

type atomicBenchWalkResult struct {
	bytes atomic.Int64
	files atomic.Int64
	dirs  atomic.Int64
}

func (r *atomicBenchWalkResult) add(info os.FileInfo) {
	if info.IsDir() {
		r.dirs.Add(1)
		return
	}
	r.files.Add(1)
	r.bytes.Add(info.Size())
}

func (r *atomicBenchWalkResult) result() benchWalkResult {
	return benchWalkResult{
		bytes: r.bytes.Load(),
		files: r.files.Load(),
		dirs:  r.dirs.Load(),
	}
}

func BenchmarkWalkDirSize(b *testing.B) {
	root := benchmarkRootDir(b)

	expected, err := walkDirSizeStdlib(root)
	if err != nil {
		b.Fatalf("filepath.WalkDir setup failed: %v", err)
	}
	b.Logf("benchmark root=%s files=%d dirs=%d bytes=%d", root, expected.files, expected.dirs, expected.bytes)

	benchmarks := []struct {
		name string
		fn   func(string) (benchWalkResult, error)
	}{
		{name: "sparkwalk", fn: walkDirSizeSparkWalk},
		{name: "filepath-walkdir", fn: walkDirSizeStdlib},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			var got benchWalkResult
			for i := 0; i < b.N; i++ {
				got, err = bm.fn(root)
				if err != nil {
					b.Fatalf("%s failed: %v", bm.name, err)
				}
			}

			b.StopTimer()
			if got != expected {
				b.Fatalf("%s mismatch: got %+v want %+v", bm.name, got, expected)
			}
			benchResultSink = got
			b.SetBytes(got.bytes)
		})
	}
}

func benchmarkRootDir(b *testing.B) string {
	b.Helper()

	root := os.Getenv(benchDirEnv)
	if root == "" {
		b.Skipf("%s not set", benchDirEnv)
	}

	info, err := os.Stat(root)
	if err != nil {
		b.Fatalf("stat %s: %v", root, err)
	}
	if !info.IsDir() {
		b.Fatalf("%s=%q is not directory", benchDirEnv, root)
	}

	return root
}

func walkDirSizeSparkWalk(root string) (benchWalkResult, error) {
	var result atomicBenchWalkResult

	err := walk.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		result.add(info)
		return nil
	})

	return result.result(), err
}

func walkDirSizeStdlib(root string) (benchWalkResult, error) {
	var result atomicBenchWalkResult

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		result.add(info)
		return nil
	})

	return result.result(), err
}
