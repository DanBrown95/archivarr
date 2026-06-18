// Package drive handles physical-drive identity and presence:
// marker-file identity for destination drives, disk usage, mount discovery,
// and a background monitor that reconciles online state into the database.
//
// Archivarr targets Linux (Docker) only; DiskUsage uses syscall.Statfs.
package drive

import "syscall"

// Usage reports the capacity and free space of the filesystem holding a path.
type Usage struct {
	CapacityBytes uint64
	FreeBytes     uint64
}

// DiskUsage returns capacity/free for the filesystem containing path.
func DiskUsage(path string) (Usage, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return Usage{}, err
	}
	bsize := uint64(st.Bsize)
	return Usage{
		CapacityBytes: st.Blocks * bsize, // total
		FreeBytes:     st.Bavail * bsize, // available to unprivileged users
	}, nil
}
