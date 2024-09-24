package gfapi

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"testing"
)

/* The testcases assume that it is being run on a peer in a gluster cluster,
 * and that the cluster has a volume named "test"
 */

var (
	vol    *Volume
	file   *File
	tmpDir = os.TempDir()
)

func TestInit(t *testing.T) {

	vol = new(Volume)

	if vol == nil {
		t.Fatalf("Failed to allocate variable")
	}

	err := vol.Init("test", "localhost")
	if err != nil {
		t.Fatalf("Failed to initialize volume. error: %v", err)
	}
}

func TestSetLogging(t *testing.T) {
	err := vol.SetLogging("./test.log", LogDebug)
	if err != nil {
		t.Fatalf("Unable to set Logging: error:  %v", err)
	}
}

func TestMount(t *testing.T) {
	err := vol.Mount()
	if err != nil {
		t.Fatalf("Failed to mount volume. error: %v", err)
	}
}

func TestMkdirAll(t *testing.T) {
	path := tmpDir + "/_TestMkdirAll_/dir/./dir2"
	err := vol.MkdirAll(path, 0777)
	if err != nil {
		t.Fatalf("MkdirAll %q: %s", path, err)
	}

	// Already exists, should succeed.
	err = vol.MkdirAll(path, 0777)
	if err != nil {
		t.Fatalf("MkdirAll %q (second time): %s", path, err)
	}

	// Make file.
	fpath := path + "/file"
	f, err := vol.Create(fpath)
	if err != nil {
		t.Fatalf("create %q: %s", fpath, err)
	}
	defer f.Close()

	// Can't make directory named after file.
	err = vol.MkdirAll(fpath, 0777)
	if err == nil {
		t.Fatalf("MkdirAll %q: no error", fpath)
	}
	perr, ok := err.(*os.PathError)
	if !ok {
		t.Fatalf("MkdirAll %q returned %T, not *PathError", fpath, err)
	}
	if filepath.Clean(perr.Path) != filepath.Clean(fpath) {
		t.Fatalf("MkdirAll %q returned wrong error path: %q not %q", fpath, filepath.Clean(perr.Path), filepath.Clean(fpath))
	}

	// Can't make subdirectory of file.
	ffpath := fpath + "/subdir"
	err = vol.MkdirAll(ffpath, 0777)
	if err == nil {
		t.Fatalf("MkdirAll %q: no error", ffpath)
	}

	perr, ok = err.(*os.PathError)
	if !ok {
		t.Fatalf("MkdirAll %q returned %T, not *PathError", ffpath, err)
	}
	if filepath.Clean(perr.Path) != filepath.Clean(fpath) {
		t.Fatalf("MkdirAll %q returned wrong error path: %q not %q", ffpath, filepath.Clean(perr.Path), filepath.Clean(fpath))
	}
}

func TestCreate(t *testing.T) {
	f, err := vol.Create(tmpDir + "/test")

	if err != nil {
		t.Fatalf("Failed to create file. Error = %v", err)
	}
	file = f
}

func TestClose1(t *testing.T) {
	err := file.Close()
	if err != nil {
		t.Errorf("Failed to close file. Error = %v", err)
	}
	file = nil
}

func TestOpen(t *testing.T) {
	d, err := vol.Open(tmpDir)
	defer d.Close()

	check(t, d.isDir == true, "Open %q: is not dir", tmpDir)

	f, err := vol.Open(tmpDir + "/test")
	check(t, f.isDir == false, "Open %q: is not file", tmpDir+"/test")

	if err != nil {
		t.Fatalf("Failed to open file. Error = %v", err)
	}
	file = f
}

func TestOpenFile(t *testing.T) {
	f, err := vol.OpenFile(tmpDir+"/test", os.O_RDONLY, 0600)

	if err != nil {
		t.Fatalf("Failed to open file. Error = %v", err)
	}
	file = f
}

func TestOpenDir(t *testing.T) {
	f, err := vol.Open(tmpDir)

	if err != nil {
		t.Fatalf("Failed to open dir. Error = %v", err)
	}
	check(t, f.isDir == true, "Open %q: is not dir", tmpDir)
	file = f
}

func TestClose2(t *testing.T) {
	err := file.Close()
	if err != nil {
		t.Errorf("Failed to close file. Error = %v", err)
	}
	file = nil
}

func TestUnlink(t *testing.T) {
	f, err := vol.Create(tmpDir + "/TestUnlink")
	if err != nil {
		t.Fatalf("Failed to create file. Error = %v", err)
	}
	f.Close()

	err = vol.Unlink(tmpDir + "/TestUnlink")
	if err != nil {
		t.Errorf("vol.Unlink failed . Error = %v", err)
	}
}

func TestRmdir(t *testing.T) {
	err := vol.Mkdir(tmpDir+"/TestRmdir", 0755)
	if err != nil {
		t.Fatalf("Failed to create file. Error = %v", err)
	}

	err = vol.Rmdir(tmpDir + "/TestRmdir")
	if err != nil {
		t.Errorf("vol.Rmdir failed . Error = %v", err)
	}
}

func TestRename(t *testing.T) {
	f, err := vol.Create(tmpDir + "/TestRename")
	if err != nil {
		t.Fatalf("Failed to create file. Error = %v", err)
	}
	f.Close()

	err = vol.Rename(tmpDir+"/TestRename", tmpDir+"/TestRenameNew")
	if err != nil {
		t.Errorf("vol.Rename failed . Error = %v", err)
	}
}

func TestFxattrs(t *testing.T) {

	f, err := vol.Create(tmpDir + "/testFxattrs")
	if err != nil {
		t.Fatalf("Failed to create file. Error = %v", err)
	}
	defer f.Close()

	err = f.Setxattr("user.glusterfs", []byte("Gluster is awesome!"), 0)
	if err != nil {
		t.Errorf("f.Setxattr() failed. Error = %v", err)
	}

	size, err := f.Getxattr("user.glusterfs", nil)
	if err != nil {
		t.Errorf("f.Getxattr() failed. Error = %v", err)
	}
	buf := make([]byte, size)
	size, err = f.Getxattr("user.glusterfs", buf)
	if err != nil {
		t.Errorf("f.Getxattr() failed. Error = %v", err)
	}

	if "Gluster is awesome!" != string(buf[:size]) {
		t.Errorf("xattrs do not match")
	}

	err = f.Removexattr("user.glusterfs")
	if err != nil {
		t.Errorf("f.Removexattr() failed. Error = %v", err)
	}
}

func TestXattrs(t *testing.T) {

	path := tmpDir + "/testXattrs"
	f, err := vol.Create(path)
	if err != nil {
		t.Fatalf("Failed to create file. Error = %v", err)
	}
	f.Close()

	err = vol.Setxattr(path, "user.glusterfs", []byte("Gluster is awesome!"), 0)
	if err != nil {
		t.Errorf("vol.Setxattr() failed. Error = %v", err)
	}

	size, err := vol.Getxattr(path, "user.glusterfs", nil)
	if err != nil {
		t.Errorf("vol.Getxattr() failed. Error = %v", err)
	}
	buf := make([]byte, size)
	size, err = vol.Getxattr(path, "user.glusterfs", buf)
	if err != nil {
		t.Errorf("vol.Getxattr() failed. Error = %v", err)
	}

	if "Gluster is awesome!" != string(buf[:size]) {
		t.Errorf("xattrs do not match")
	}

	err = vol.Removexattr(path, "user.glusterfs")
	if err != nil {
		t.Errorf("vol.Removexattr() failed. Error = %v", err)
	}
}

func TestStatvfs(t *testing.T) {
	if runtime.GOOS == "linux" {
		var vbuf Statvfs_t
		err := vol.Statvfs("/", &vbuf)
		if err != nil {
			t.Errorf("vol.Statvfs() failed. Error = %v", err)
		}

		if vbuf.Namemax != 255 {
			t.Errorf("vol.Statvfs() failed. Incorrect Namemax")
		}
	}
}

func TestReaddirnames(t *testing.T) {
	tmpReadDir, clean := setupReaddir(t)
	defer clean()
	d, err := vol.OpenDir(tmpReadDir)
	check(t, err == nil, "Open %q: %s", tmpReadDir, err)

	var names []string
	names, err = d.Readdirnames(0)
	check(t, err == nil, "Readdirnames %q: %s", tmpReadDir, err)

	err = d.Close()
	check(t, err == nil, "Close %q: %s", tmpReadDir, err)

	check(t, len(names) == 4,
		"incorrect number of files %v != %v", len(names), 4)

	expected := []string{
		".",
		"..",
		"dir",
		"file",
	}

	sort.Strings(names)
	check(t, reflect.DeepEqual(names, expected),
		"file names doesn't match %v != %v", names, expected)

	// test readdirnames with limit

	d, err = vol.OpenDir(tmpReadDir)
	check(t, err == nil, "Open %q: %s", tmpReadDir, err)

	var all []string

	names, err = d.Readdirnames(2)
	check(t, err == nil, "Readdirnames %q: %s", tmpReadDir, err)
	check(t, len(names) == 2, "should only read 2 files")
	all = append(all, names...)

	names, err = d.Readdirnames(2)
	check(t, err == nil, "Readdirnames %q: %s", tmpReadDir, err)
	check(t, len(names) == 2, "should only read 2 files")
	all = append(all, names...)

	names, err = d.Readdirnames(2)
	check(t, err == nil, "Readdirnames %q: %s", tmpReadDir, err)
	check(t, len(names) == 0, "should not read more files %+v", names)

	err = d.Close()
	check(t, err == nil, "Close %q: %s", tmpReadDir, err)

	check(t, len(all) == 4,
		"incorrect number of files %v != %v", len(all), 4)

	sort.Strings(all)
	check(t, reflect.DeepEqual(all, expected),
		"file names doesn't match %v != %v", all, expected)
}

//cursor

func TestReaddir(t *testing.T) {
	tmpReadDir, clean := setupReaddir(t)
	defer clean()
	vol.SetLogging("testreaddirlog", LogDebug)

	d, err := vol.OpenDir(tmpReadDir)
	check(t, err == nil, "Open %q: %s", tmpReadDir, err)

	info, err := d.Readdir(0)
	check(t, err == nil, "Readdir %q: %s", tmpReadDir, err)

	err = d.Close()
	check(t, err == nil, "Close %q: %s", tmpReadDir, err)

	check(t, len(info) == 4,
		"incorrect number of files %v != %v", len(info), 4)

	files := map[string]os.FileInfo{
		"dir":  nil,
		"file": nil,
	}

	for _, d := range info {
		if d == nil {
			continue
		}
		if _, ok := files[d.Name()]; ok {
			files[d.Name()] = d
		}
	}

	check(t, files["file"] != nil, "no info for file")
	check(t, files["file"].IsDir() == false, "file should not be a dir")
	check(t, files["file"].Size() == int64(len(data)),
		"incorrect file size %v != %v", files["file"].Size(), len(data))

	check(t, files["dir"] != nil, "no info for dir")
	check(t, files["dir"].IsDir() == true, "dir should be a directory")
	check(t, files["dir"].Mode()&os.ModePerm == dirPerm,
		"incorrect dir mode %#o != %#o", files["dir"].Mode(), dirPerm)

	// test readdir with limit

	d, err = vol.OpenDir(tmpReadDir)
	check(t, err == nil, "Open %q: %s", tmpReadDir, err)

	info, err = d.Readdir(2)
	check(t, err == nil, "Readdir %q: %s", tmpReadDir, err)
	check(t, len(info) == 2, "should only read 2 files")

	info, err = d.Readdir(2)
	check(t, err == nil, "Readdir %q: %s", tmpReadDir, err)
	check(t, len(info) == 2, "should only read 2 files")

	info, err = d.Readdir(2)
	check(t, err == nil, "Readdir %q: %s", tmpReadDir, err)
	check(t, len(info) == 0, "should not read more files")

	err = d.Close()
	check(t, err == nil, "Close %q: %s", tmpReadDir, err)
}

func TestReaddirR(t *testing.T) {
	tmpReadDir, clean := setupReaddir(t)
	defer clean()
	vol.SetLogging("testreaddirlog", LogDebug)

	d, err := vol.OpenDir(tmpReadDir)
	check(t, err == nil, "Open %q: %s", tmpReadDir, err)

	info, err := d.ReaddirR(0)
	check(t, err == nil, "Readdir %q: %s", tmpReadDir, err)

	err = d.Close()
	check(t, err == nil, "Close %q: %s", tmpReadDir, err)

	check(t, len(info) == 4,
		"incorrect number of files %v != %v", len(info), 4)

	files := map[string]os.FileInfo{
		"dir":  nil,
		"file": nil,
	}

	for _, d := range info {
		if d == nil {
			continue
		}
		if _, ok := files[d.Name()]; ok {
			files[d.Name()] = d
		}
	}

	check(t, files["file"] != nil, "no info for file")
	check(t, files["file"].IsDir() == false, "file should not be a dir")
	check(t, files["file"].Size() == int64(len(data)),
		"incorrect file size %v != %v", files["file"].Size(), len(data))

	check(t, files["dir"] != nil, "no info for dir")
	check(t, files["dir"].IsDir() == true, "dir should be a directory")
	check(t, files["dir"].Mode()&os.ModePerm == dirPerm,
		"incorrect dir mode %#o != %#o", files["dir"].Mode(), dirPerm)

	// test readdir with limit

	d, err = vol.OpenDir(tmpReadDir)
	check(t, err == nil, "Open %q: %s", tmpReadDir, err)

	info, err = d.ReaddirR(2)
	check(t, err == nil, "Readdir %q: %s", tmpReadDir, err)
	check(t, len(info) == 2, "should only read 2 files")

	info, err = d.ReaddirR(2)
	check(t, err == nil, "Readdir %q: %s", tmpReadDir, err)
	check(t, len(info) == 2, "should only read 2 files")

	info, err = d.ReaddirR(2)
	check(t, err == nil, "Readdir %q: %s", tmpReadDir, err)
	check(t, len(info) == 0, "should not read more files")

	err = d.Close()
	check(t, err == nil, "Close %q: %s", tmpReadDir, err)
}

func TestUnmount(t *testing.T) {
	if err := vol.Unmount(); err != nil {
		t.Logf("Failed to unmount volume. Ret = %v", err)
	}
}

var (
	dirPerm = os.FileMode(0700)
	data    = []byte("data")
)

func setupReaddir(t *testing.T) (string, func()) {
	readdir := tmpDir + "/test-gluster-readdir"
	err := vol.MkdirAll(readdir, 0777)
	check(t, err == nil, "MkdirAll %q: %s", readdir, err)

	dir := filepath.Join(readdir, "dir")
	file := filepath.Join(readdir, "file")

	err = vol.MkdirAll(dir, dirPerm)
	check(t, err == nil, "MkdirAll %q: %s", dir, err)

	f, err := vol.Create(file)
	check(t, err == nil, "Create %q: %s", file, err)

	n, err := f.Write(data)
	check(t, err == nil, "Write %q: %s", file, err)
	check(t, n == len(data), "write length incorrect, %v != %v", n, len(data))

	_, err = f.Readdir(0)
	check(t, err != nil, "Readdir should fail with files, %v", file)

	err = f.Close()
	check(t, err == nil, "Close %q: %s", file, err)

	f, err = vol.Open(dir)
	check(t, err == nil, "OpenFile %s: %v", file, err)

	_, err = f.Readdirnames(0)
	check(t, err == nil, "Readdirnames fail with files, %v: %v", file, err)
	defer f.Close()

	return readdir, func() {
		vol.Unlink(file)
		vol.Unlink(dir)
		vol.Unlink(readdir)
	}
}

func check(t *testing.T, c bool, message string, args ...interface{}) {
	t.Helper()

	if !c {
		t.Fatalf(message, args...)
	}
}
