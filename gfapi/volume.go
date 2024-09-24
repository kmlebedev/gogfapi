package gfapi

// This file includes operations that operate on a gluster volume
// for more information please 'api/src/glfs.h' in the glusterfs source.

//go:generate sh -c "go tool cgo -godefs types_unix.go | gofmt > ztypes_${GOOS}_${GOARCH}.go"
// TODO: Need to run `go generate` on different platforms to generate relevant ztypes file for each
// - *BSD
// - Mac OS X

// #cgo pkg-config: glusterfs-api
// #include "glusterfs/api/glfs.h"
// #include <stdlib.h>
// #include <time.h>
// #include <sys/stat.h>
import "C"

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Volume is the gluster filesystem object, which represents the virtual filesystem.
type Volume struct {
	fs *C.glfs_t
}

// Init creates a new glfs object "Volume". Volname is the name of the Gluster Volume
// and also the "volfile-id". Hosts accepts one or more hostname(s) and/or IP(s)
// of volname's constitute volfile servers (management server/glusterd).
//
// Limitations:
// * Assumes tcp transport and glusterd is listening on 24007
func (v *Volume) Init(volname string, hosts ...string) error {
	cvolname := C.CString(volname)
	defer C.free(unsafe.Pointer(cvolname))

	v.fs = C.glfs_new(cvolname)
	if v.fs == nil {
		return fmt.Errorf("error creating mount object")
	}

	for i, host := range hosts {
		ctrans := C.CString("tcp")
		if strings.HasSuffix(host, ".socket") {
			ctrans = C.CString("unix")
		}
		chost := C.CString(host)
		defer C.free(unsafe.Pointer(ctrans))
		defer C.free(unsafe.Pointer(chost))
		// NOTE: This API is special, multiple calls to this function with different
		// volfile servers, port or transport-type would create a list of volfile
		// servers which would be polled during `volfile_fetch_attempts()`
		ret, err := C.glfs_set_volfile_server(v.fs, ctrans, chost, 24007)
		if int(ret) < 0 {
			return fmt.Errorf("error adding host %d of %d %q as a volserver: %s", i, len(hosts), host, err)
		}
	}

	return nil
}

// InitWithVolfile initializes the Volume using the given volfile.
// This must be done before calling Mount.
//
// volfile is the path to the locally available volfile
//
// Return value is 0 for success and non 0 for failure
func (v *Volume) InitWithVolfile(volname, volfile string) int {
	cvolname := C.CString(volname)
	cvolfile := C.CString(volfile)
	defer C.free(unsafe.Pointer(cvolname))
	defer C.free(unsafe.Pointer(cvolfile))

	v.fs = C.glfs_new(cvolname)

	ret := C.glfs_set_volfile(v.fs, cvolfile)

	return int(ret)
}

// Mount establishes a 'virtual mount.' Mount must be called after Init and
// before storage operations. Steps taken:
//
//   - Spawn a poll-loop thread.
//   - Establish connection to management daemon (volfile server) and receive volume specification (volfile).
//   - Construct translator graph and initialize graph.
//   - Wait for initialization (connecting to all bricks) to complete.
//
// Source: glfs.h
func (v *Volume) Mount() error {

	ret, err := C.glfs_init(v.fs)
	if int(ret) < 0 {
		return fmt.Errorf("mount failed: %s", err)
	}

	return nil
}

// LogLevel is the logging level to be used to logging
type LogLevel int

// LogNone .. LogTrace are LogLevel types which correspond to the equivalent gluster log levels
const (
	LogNone LogLevel = iota
	LogEmerg
	LogAlert
	LogCritical
	LogError
	LogWarning
	LogNotice
	LogInfo
	LogDebug
	LogTrace
)

// SetLogging sets the gfapi log file path and LogLevel. The Volume must be
// initialized before calling. An empty string "" is passed as 'name'
// sets the default log directory (/var/log/glusterfs).
func (v *Volume) SetLogging(name string, logLevel LogLevel) error {

	if name == "" {
		ret, err := C.glfs_set_logging(v.fs, nil, C.int(logLevel))
		if int(ret) < 0 {
			return err
		}
		return nil
	}

	if _, err := os.Stat(path.Dir(name)); err != nil {
		return err
	}

	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	ret, err := C.glfs_set_logging(v.fs, cname, C.int(logLevel))
	if int(ret) < 0 {
		return err
	}

	return nil
}

// Unmount ends the virtual mount. Return
// >0: filled N bytes of buffer
// 0: no volfile available
// <0: volfile length exceeds @len by N bytes (@buf unchanged)
func (v *Volume) Unmount() error {
	if v.fs == nil {
		return nil
	}
	ret, err := C.glfs_fini(v.fs)
	if err != nil {
		return fmt.Errorf("failure to unmount volume: %v", err)
	}
	if int(ret) < 0 {
		return fmt.Errorf("failure to unmount volume: volfile length exceeds")
	}
	return nil
}

// Chmod changes the mode of the named file to given mode
//
// Returns an error on failure
func (v *Volume) Chmod(name string, mode os.FileMode) error {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	_, err := C.glfs_chmod(v.fs, cname, C.mode_t(posixMode(mode)))

	return err
}

// Chmod changes the uid, gid of the named file
//
// Returns an error on failure
func (v *Volume) Chown(name string, uid, gid int) error {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	_, err := C.glfs_chown(v.fs, cname, C.uid_t(uid), C.gid_t(gid))

	return err
}

// Chmod changes the mtime of the named file
//
// Returns an error on failure
func (v *Volume) Chtimes(name string, mtime time.Time) error {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	var amtime [2]C.struct_timespec
	amtime[1] = C.struct_timespec{tv_sec: C.long(mtime.Unix()), tv_nsec: C.long(mtime.Nanosecond())}
	_, err := C.glfs_utimens(v.fs, cname, &amtime[0])

	return err
}

// Create creates a file with given name on the the Volume v.
// The Volume must be mounted before calling Create.
// Create is similar to os.Create in its functioning.
//
// name is the name of the file to be create.
//
// Returns a File object on success and a os.PathError on failure.
func (v *Volume) Create(name string) (*File, error) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	cfd, err := C.glfs_creat(v.fs, cname, C.int(os.O_RDWR|os.O_CREATE|os.O_TRUNC), 0666)

	if cfd == nil {
		return nil, &os.PathError{"create", name, err}
	}

	return NewFile(name, &Glfs{cfd}, false), nil
}

// Unlink attempts to unlink a file a path and returns a non-nil error on failure.
func (v *Volume) Unlink(path string) error {

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	ret, err := C.glfs_unlink(v.fs, cpath)
	if int(ret) < 0 {
		return &os.PathError{"unlink", path, err}
	}
	return nil
}

// Lstat returns an os.FileInfo object describing the named file. It doesn't follow the link if the file is a symlink.
//
// Returns an error on failure
func (v *Volume) Lstat(name string) (os.FileInfo, error) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	var stat syscall.Stat_t
	ret, err := C.glfs_lstat(v.fs, cname, (*C.struct_stat)(unsafe.Pointer(&stat)))
	if int(ret) < 0 {
		return nil, err
	}
	return fileInfoFromStat(&stat, name), nil
}

// Mkdir creates a new directory with given name and permission bits
//
// Returns an error on failure
func (v *Volume) Mkdir(name string, perm os.FileMode) error {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	ret, err := C.glfs_mkdir(v.fs, cname, C.mode_t(posixMode(perm)))

	if ret != 0 {
		return &os.PathError{"mkdir", name, err}
	}
	return nil
}

// Removes an existing directory
//
// Returns error on failure
func (v *Volume) Rmdir(path string) error {

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	ret, err := C.glfs_rmdir(v.fs, cpath)

	if ret != 0 {
		return &os.PathError{"rmdir", path, err}
	}
	return nil
}

// MkdirAll creates a directory named path, along with any necessary parents,
// and returns nil, or else returns an error.
// The permission bits perm are used for all directories that MkdirAll creates.
// If path is already a directory, MkdirAll does nothing and returns nil.
func (v *Volume) MkdirAll(path string, perm os.FileMode) error {
	// Fast path: if we can tell whether path is a directory or file, stop with success or error.
	dir, err := v.Stat(path)
	if err == nil {
		if dir.IsDir() {
			return nil
		}
		return &os.PathError{"mkdir", path, syscall.ENOTDIR}
	}

	// Slow path: make sure parent exists and then call Mkdir for path.
	i := len(path)
	for i > 0 && os.IsPathSeparator(path[i-1]) { // Skip trailing path separator.
		i--
	}

	j := i
	for j > 0 && !os.IsPathSeparator(path[j-1]) { // Scan backward over element.
		j--
	}

	if j > 1 {
		// Create parent
		err = v.MkdirAll(path[0:j-1], perm)
		if err != nil {
			return err
		}
	}

	// Parent now exists; invoke Mkdir and use its result.
	err = v.Mkdir(path, perm)
	if err != nil {
		// Handle arguments like "foo/." by
		// double-checking that directory doesn't exist.
		dir, err1 := v.Lstat(path)
		if err1 == nil && dir.IsDir() {
			return nil
		}
		return err
	}

	return nil
}

// RemoveAll removes path and any children it con

// Open opens the named file on the the Volume v.
// The Volume must be mounted before calling Open.
// Open is similar to os.Open in its functioning.
//
// name is the name of the file to be open.
//
// Returns a File object on success and a os.PathError on failure.
func (v *Volume) Open(name string) (*File, error) {
	var isDir bool

	if stat, err := v.Stat(name); err != nil {
		return nil, &os.PathError{"open", name, err}
	} else {
		isDir = stat.IsDir()
	}
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	var cfd *C.glfs_fd_t
	var err error
	if isDir {
		cfd, err = C.glfs_opendir(v.fs, cname)
	} else {
		cfd, err = C.glfs_open(v.fs, cname, C.int(os.O_RDONLY))
	}
	if err != nil {
		return nil, &os.PathError{"open", name, fmt.Errorf("glfs_open: %v", err)}
	}
	if cfd == nil {
		return nil, &os.PathError{"open", name, err}
	}

	return NewFile(name, &Glfs{cfd}, isDir), nil
}

// OpenFile opens the named file on the the Volume v.
// The Volume must be mounted before calling OpenFile.
// OpenFile is similar to os.OpenFile in its functioning.
//
// name is the name of the file to be open.
// flags is the access mode of the file.
// perm is the permissions for the opened file.
//
// Returns a File object on success and a os.PathError on failure.
//
// BUG : perm is not used for opening the file.
// NOTE: It is better to use Open, Create etc. instead of using OpenFile directly
func (v *Volume) OpenFile(name string, flags int, perm os.FileMode) (*File, error) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	var cfd *C.glfs_fd_t
	var err error
	if (os.O_CREATE & flags) == os.O_CREATE {
		cfd, err = C.glfs_creat(v.fs, cname, C.int(flags), C.mode_t(posixMode(perm)))
	} else {
		cfd, err = C.glfs_open(v.fs, cname, C.int(flags))
	}

	if cfd == nil {
		return nil, &os.PathError{"open", name, err}
	}

	return NewFile(name, &Glfs{cfd}, false), nil
}

func (v *Volume) OpenDir(name string) (*File, error) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	cfd, err := C.glfs_opendir(v.fs, cname)
	if cfd == nil {
		return nil, &os.PathError{"open", name, err}
	}

	return NewFile(name, &Glfs{cfd}, true), nil
}

// Stat returns an os.FileInfo object describing the named file
//
// Returns an error on failure
func (v *Volume) Stat(name string) (os.FileInfo, error) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	var stat syscall.Stat_t
	ret, err := C.glfs_stat(v.fs, cname, (*C.struct_stat)(unsafe.Pointer(&stat)))
	if int(ret) < 0 {
		return nil, &os.PathError{"stat", name, err}
	}
	return fileInfoFromStat(&stat, name), nil
}

// Truncate changes the size of the named file
//
// # Returns an error on failure
//
// TODO: gfapi currently (20131120) has not implement glfs_truncate.
//
//	Once it has been implemented, renable the commented out code
//	or write own function to implement the functionality of glfs_truncate
func (v *Volume) Truncate(name string, size int64) error {
	// cname := C.CString(name)
	// defer C.free(unsafe.Pointer(cname))

	// _, err := C.glfs_truncate(v.fs, cname, C.off_t(size))

	// return err
	return errors.New("Truncate not implemented")
}

// Rename a file or directory
//
// Returns error on failure
func (v *Volume) Rename(oldpath string, newpath string) error {

	coldpath := C.CString(oldpath)
	defer C.free(unsafe.Pointer(coldpath))

	cnewpath := C.CString(newpath)
	defer C.free(unsafe.Pointer(cnewpath))

	ret, err := C.glfs_rename(v.fs, coldpath, cnewpath)
	if int(ret) < 0 {
		return err
	}
	return nil
}

// Get value of the extended attribute 'attr' and place it in 'dest'
//
// Returns number of bytes placed in 'dest' and error if any
func (v *Volume) Getxattr(path string, attr string, dest []byte) (int64, error) {
	var ret C.ssize_t
	var err error

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	cattr := C.CString(attr)
	defer C.free(unsafe.Pointer(cattr))

	if len(dest) <= 0 {
		ret, err = C.glfs_getxattr(v.fs, cpath, cattr, nil, 0)
	} else {
		ret, err = C.glfs_getxattr(v.fs, cpath, cattr,
			unsafe.Pointer(&dest[0]), C.size_t(len(dest)))
	}

	if ret >= 0 {
		return int64(ret), nil
	} else {
		return int64(ret), err
	}
}

// Set extended attribute with key 'attr' and value 'data'
//
// Returns error on failure
func (v *Volume) Setxattr(path string, attr string, data []byte, flags int) error {

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	cattr := C.CString(attr)
	defer C.free(unsafe.Pointer(cattr))

	ret, err := C.glfs_setxattr(v.fs, cpath, cattr,
		unsafe.Pointer(&data[0]), C.size_t(len(data)),
		C.int(flags))

	if ret == 0 {
		err = nil
	}
	return err
}

// Remove extended attribute named 'attr'
//
// Returns error on failure
func (v *Volume) Removexattr(path string, attr string) error {

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	cattr := C.CString(attr)
	defer C.free(unsafe.Pointer(cattr))

	ret, err := C.glfs_removexattr(v.fs, cpath, cattr)

	if ret == 0 {
		err = nil
	}
	return err
}

// Get filesystem statistics
//
// Returns an error on failure
func (v *Volume) Statvfs(path string, buf *Statvfs_t) error {

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	ret, err := C.glfs_statvfs(v.fs, cpath,
		(*C.struct_statvfs)(unsafe.Pointer(buf)))

	if ret == 0 {
		err = nil
	}
	return err
}
