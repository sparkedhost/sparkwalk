// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package walk_test

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	walk "github.com/sparkedhost/sparkwalk"
)

var LstatP = walk.LstatP

type Node struct {
	name    string
	entries []*Node // nil if the entry is a file
	mark    int
}

var tree = &Node{
	"testdata",
	[]*Node{
		{"a", nil, 0},
		{"b", []*Node{}, 0},
		{"c", nil, 0},
		{
			"d",
			[]*Node{
				{"x", nil, 0},
				{"y", []*Node{}, 0},
				{
					"z",
					[]*Node{
						{"u", nil, 0},
						{"v", nil, 0},
					},
					0,
				},
			},
			0,
		},
	},
	0,
}

func walkTree(n *Node, path string, f func(path string, n *Node)) {
	f(path, n)
	for _, e := range n.entries {
		walkTree(e, walk.Join(path, e.name), f)
	}
}

func makeTree(t *testing.T) {
	walkTree(tree, tree.name, func(path string, n *Node) {
		if n.entries == nil {
			fd, err := os.Create(path)
			if err != nil {
				t.Errorf("makeTree: %v", err)
				return
			}
			fd.Close()
		} else {
			if err := os.Mkdir(path, 0770); err != nil {
				t.Errorf("makeTree: %v", err)
			}
		}
	})
}

func markTree(n *Node) { walkTree(n, "", func(path string, n *Node) { n.mark++ }) }

func checkMarks(t *testing.T, report bool) {
	walkTree(tree, tree.name, func(path string, n *Node) {
		if n.mark != 1 && report {
			t.Errorf("node %s mark = %d; expected 1", path, n.mark)
		}
		n.mark = 0
	})
}

// Assumes that each node name is unique. Good enough for a test.
// If clear is true, any incoming error is cleared before return. The errors
// are always accumulated, though.
func mark(path string, info os.FileInfo, err error, errors *[]error, clear bool, mu *sync.Mutex) error {
	mu.Lock()
	defer mu.Unlock()

	if err != nil {
		*errors = append(*errors, err)
		if clear {
			return nil
		}
		return err
	}
	name := info.Name()
	walkTree(tree, tree.name, func(path string, n *Node) {
		if n.name == name {
			n.mark++
		}
	})
	return nil
}

func TestWalk(t *testing.T) {
	makeTree(t)
	errors := make([]error, 0, 10)
	clear := true
	var mu sync.Mutex
	markFn := func(path string, info os.FileInfo, err error) error {
		return mark(path, info, err, &errors, clear, &mu)
	}
	// Expect no errors.
	err := walk.Walk(tree.name, markFn)
	if err != nil {
		t.Fatalf("no error expected, found: %s", err)
	}
	if len(errors) != 0 {
		t.Fatalf("unexpected errors: %s", errors)
	}
	checkMarks(t, true)
	errors = errors[0:0]

	// Test permission errors.  Only possible if we're not root
	// and only on some file systems (AFS, FAT).  To avoid errors during
	// all.bash on those file systems, skip during go test -short.
	if os.Getuid() > 0 && !testing.Short() {
		// introduce 2 errors: chmod top-level directories to 0
		os.Chmod(walk.Join(tree.name, tree.entries[1].name), 0)
		os.Chmod(walk.Join(tree.name, tree.entries[3].name), 0)

		// 3) capture errors, expect two.
		// mark respective subtrees manually
		markTree(tree.entries[1])
		markTree(tree.entries[3])
		// correct double-marking of directory itself
		tree.entries[1].mark--
		tree.entries[3].mark--
		err := walk.Walk(tree.name, markFn)
		if err != nil {
			t.Fatalf("expected no error return from Walk, got %s", err)
		}
		if len(errors) != 2 {
			t.Errorf("expected 2 errors, got %d: %s", len(errors), errors)
		}
		// the inaccessible subtrees were marked manually
		checkMarks(t, true)
		errors = errors[0:0]

		// 4) capture errors, stop after first error.
		// mark respective subtrees manually
		markTree(tree.entries[1])
		markTree(tree.entries[3])
		// correct double-marking of directory itself
		tree.entries[1].mark--
		tree.entries[3].mark--
		clear = false // error will stop processing
		err = walk.Walk(tree.name, markFn)
		if err == nil {
			t.Fatalf("expected error return from Walk")
		}
		if len(errors) < 1 || len(errors) > 2 {
			t.Errorf("expected 1-2 errors, got %d: %s", len(errors), errors)
		}
		// the inaccessible subtrees were marked manually
		checkMarks(t, false)
		errors = errors[0:0]

		// restore permissions
		os.Chmod(walk.Join(tree.name, tree.entries[1].name), 0770)
		os.Chmod(walk.Join(tree.name, tree.entries[3].name), 0770)
	}

	// cleanup
	if err := os.RemoveAll(tree.name); err != nil {
		t.Errorf("removeTree: %v", err)
	}
}

func touch(t *testing.T, name string) {
	f, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func equalErrorMaps(got, want map[string]error) bool {
	if len(got) != len(want) {
		return false
	}

	for path, wantErr := range want {
		gotErr, ok := got[path]
		if !ok {
			return false
		}
		if wantErr == nil || gotErr == nil {
			if gotErr != wantErr {
				return false
			}
			continue
		}
		if !errors.Is(gotErr, wantErr) {
			return false
		}
	}

	return true
}

func goRoot(t *testing.T) string {
	t.Helper()

	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatalf("go env GOROOT: %v", err)
	}

	root := strings.TrimSpace(string(out))
	if root == "" {
		t.Fatal("go env GOROOT returned an empty path")
	}

	return root
}

func TestWalkFileError(t *testing.T) {
	td, err := os.MkdirTemp("", "walktest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(td)

	touch(t, walk.Join(td, "foo"))
	touch(t, walk.Join(td, "bar"))
	dir := walk.Join(td, "dir")
	if err := os.MkdirAll(walk.Join(td, "dir"), 0755); err != nil {
		t.Fatal(err)
	}
	touch(t, walk.Join(dir, "baz"))
	touch(t, walk.Join(dir, "stat-error"))
	defer func() {
		*walk.LstatP = os.Lstat
	}()
	statErr := errors.New("some stat error")
	*walk.LstatP = func(path string) (os.FileInfo, error) {
		if strings.HasSuffix(path, "stat-error") {
			return nil, statErr
		}
		return os.Lstat(path)
	}
	got := map[string]error{}
	var mu sync.Mutex
	err = walk.Walk(td, func(path string, fi os.FileInfo, err error) error {
		rel, _ := walk.Rel(td, path)
		mu.Lock()
		got[walk.ToSlash(rel)] = err
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Errorf("Walk error: %v", err)
	}
	want := map[string]error{
		".":              nil,
		"foo":            nil,
		"bar":            nil,
		"dir":            nil,
		"dir/baz":        nil,
		"dir/stat-error": statErr,
	}
	if !equalErrorMaps(got, want) {
		t.Errorf("Walked %#v; want %#v", got, want)
	}
}

func TestBug3486(t *testing.T) { // http://code.google.com/p/go/issues/detail?id=3486
	root, err := walk.EvalSymlinks(goRoot(t) + "/test")
	if err != nil {
		t.Fatal(err)
	}
	bugs := walk.Join(root, "bugs")
	ken := walk.Join(root, "ken")
	_, bugsErr := os.Stat(bugs)
	haveBugs := bugsErr == nil
	_, kenErr := os.Stat(ken)
	haveKen := kenErr == nil
	var seenBugs atomic.Bool
	var seenKen atomic.Bool
	err = walk.Walk(root, func(pth string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		switch pth {
		case bugs:
			seenBugs.Store(true)
			return walk.ErrSkipDir
		case ken:
			seenKen.Store(true)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if haveBugs && !seenBugs.Load() {
		t.Fatalf("%q not seen", bugs)
	}
	if haveKen && !seenKen.Load() {
		t.Fatalf("%q not seen", ken)
	}
}

func TestSkipDirOnFile(t *testing.T) {
	td, err := os.MkdirTemp("", "walktest_skipdir_file")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(td)

	if err := os.MkdirAll(walk.Join(td, "dir1"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(walk.Join(td, "dir2"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(walk.Join(td, "dir1", "file1"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(walk.Join(td, "dir1", "file2"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(walk.Join(td, "dir2", "file3"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	var seenDir2 atomic.Bool
	err = walk.Walk(td, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Name() == "dir2" {
			seenDir2.Store(true)
		}
		if info.Name() == "file1" || info.Name() == "file2" {
			return walk.ErrSkipDir
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Walk returned unexpected error: %v", err)
	}
	if !seenDir2.Load() {
		t.Errorf("Walk did not visit dir2, so ErrSkipDir on a file incorrectly aborted the global walk!")
	}
}

func TestSkipDirOnStatError(t *testing.T) {
	td, err := os.MkdirTemp("", "walktest_skipdir_stat")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(td)

	if err := os.MkdirAll(walk.Join(td, "dir1"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(walk.Join(td, "dir1", "file-stat-error"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(walk.Join(td, "dir1", "file-ok"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	defer func() {
		*walk.LstatP = os.Lstat
	}()
	*walk.LstatP = func(path string) (os.FileInfo, error) {
		if strings.HasSuffix(path, "file-stat-error") {
			return nil, errors.New("simulated stat error")
		}
		return os.Lstat(path)
	}

	var seenFileOk atomic.Bool
	err = walk.Walk(td, func(path string, info os.FileInfo, err error) error {
		if err != nil { // simulated stat error
			return walk.ErrSkipDir
		}
		if info.Name() == "file-ok" {
			seenFileOk.Store(true)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Walk returned unexpected error: %v", err)
	}
	if !seenFileOk.Load() {
		t.Errorf("file-ok was not visited: ErrSkipDir on a stat error should not skip remaining siblings")
	}
}

func TestSkipDirOnRoot(t *testing.T) {
	td, err := os.MkdirTemp("", "walktest_skipdir_root")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(td)

	if err := os.WriteFile(walk.Join(td, "root_file"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(walk.Join(td, "root_dir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(walk.Join(td, "root_dir", "child"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	// ErrSkipDir on a root file (lstat succeeds): Walk should return nil.
	err = walk.Walk(walk.Join(td, "root_file"), func(path string, info os.FileInfo, err error) error {
		return walk.ErrSkipDir
	})
	if err != nil {
		t.Errorf("root file + ErrSkipDir: got %v, want nil", err)
	}

	// ErrSkipDir on a root file (lstat fails): Walk should return nil.
	err = walk.Walk(walk.Join(td, "missing_file"), func(path string, info os.FileInfo, err error) error {
		return walk.ErrSkipDir
	})
	if err != nil {
		t.Errorf("missing root file + ErrSkipDir: got %v, want nil", err)
	}

	// ErrSkipDir on a root directory: Walk should return nil and not descend.
	var childSeen atomic.Bool
	err = walk.Walk(walk.Join(td, "root_dir"), func(path string, info os.FileInfo, err error) error {
		if info != nil && info.Name() == "child" {
			childSeen.Store(true)
		}
		if info != nil && info.IsDir() {
			return walk.ErrSkipDir
		}
		return nil
	})
	if err != nil {
		t.Errorf("root dir + ErrSkipDir: got %v, want nil", err)
	}
	if childSeen.Load() {
		t.Errorf("child of skipped root dir was visited")
	}
}
