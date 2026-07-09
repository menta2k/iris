package data

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
)

// pseudoFS are non-disk filesystem types excluded from mount enumeration.
var pseudoFS = map[string]bool{
	"proc": true, "sysfs": true, "devtmpfs": true, "devpts": true, "tmpfs": true,
	"cgroup": true, "cgroup2": true, "mqueue": true, "hugetlbfs": true, "debugfs": true,
	"tracefs": true, "securityfs": true, "pstore": true, "bpf": true, "configfs": true,
	"fusectl": true, "autofs": true, "binfmt_misc": true, "rpc_pipefs": true, "nsfs": true,
	"efivarfs": true, "ramfs": true, "fuse.gvfsd-fuse": true, "squashfs": true,
}

// HostSampler reads host CPU / memory / disk usage from /proc and statfs (Linux).
// CPU is measured as an in-call delta over a short window so each sample is an
// instantaneous reading with no cross-call state.
type HostSampler struct{}

// NewHostSampler constructs the sampler.
func NewHostSampler() *HostSampler { return &HostSampler{} }

// Sample reads current CPU, memory, and per-path disk usage. A failure reading
// one disk path is skipped (not fatal); CPU/memory read failures return an error.
func (h *HostSampler) Sample(ctx context.Context, diskPaths []string) (biz.SystemSnapshot, error) {
	cpu, err := sampleCPU(ctx)
	if err != nil {
		return biz.SystemSnapshot{}, fmt.Errorf("sample cpu: %w", err)
	}
	memUsed, memTotal, err := sampleMem()
	if err != nil {
		return biz.SystemSnapshot{}, fmt.Errorf("sample memory: %w", err)
	}
	memPct := 0.0
	if memTotal > 0 {
		memPct = float64(memUsed) / float64(memTotal) * 100
	}
	disks := make([]biz.DiskUsage, 0, len(diskPaths))
	for _, p := range diskPaths {
		du, ok := sampleDisk(p)
		if ok {
			disks = append(disks, du)
		}
	}
	return biz.SystemSnapshot{
		CollectedAt:   time.Now().UTC(),
		CPUPercent:    cpu,
		MemPercent:    memPct,
		MemUsedBytes:  memUsed,
		MemTotalBytes: memTotal,
		Disks:         disks,
		Available:     true,
	}, nil
}

// sampleCPU reads /proc/stat twice over a short window and returns the busy
// percentage across all cores.
func sampleCPU(ctx context.Context) (float64, error) {
	total1, idle1, err := readCPUTimes()
	if err != nil {
		return 0, err
	}
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-time.After(200 * time.Millisecond):
	}
	total2, idle2, err := readCPUTimes()
	if err != nil {
		return 0, err
	}
	dTotal := float64(total2 - total1)
	dIdle := float64(idle2 - idle1)
	if dTotal <= 0 {
		return 0, nil
	}
	pct := (dTotal - dIdle) / dTotal * 100
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return pct, nil
}

// readCPUTimes parses the aggregate "cpu" line of /proc/stat into total and idle
// jiffies. idle includes iowait.
func readCPUTimes() (total, idle uint64, err error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0, 0, fmt.Errorf("/proc/stat: empty")
	}
	fields := strings.Fields(sc.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, fmt.Errorf("/proc/stat: unexpected first line")
	}
	for i, v := range fields[1:] {
		n, e := strconv.ParseUint(v, 10, 64)
		if e != nil {
			continue
		}
		total += n
		// idle (index 3) + iowait (index 4) within fields[1:].
		if i == 3 || i == 4 {
			idle += n
		}
	}
	return total, idle, nil
}

// sampleMem reads MemTotal and MemAvailable (kB) from /proc/meminfo and returns
// used and total bytes.
func sampleMem() (used, total uint64, err error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	var memTotalKB, memAvailKB uint64
	var haveTotal, haveAvail bool
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			memTotalKB, haveTotal = parseMeminfoKB(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			memAvailKB, haveAvail = parseMeminfoKB(line)
		}
		if haveTotal && haveAvail {
			break
		}
	}
	if !haveTotal {
		return 0, 0, fmt.Errorf("/proc/meminfo: MemTotal missing")
	}
	total = memTotalKB * 1024
	availBytes := memAvailKB * 1024
	if availBytes > total {
		availBytes = total
	}
	return total - availBytes, total, nil
}

func parseMeminfoKB(line string) (uint64, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0, false
	}
	n, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// Mounts enumerates the host's real (non-pseudo) filesystems from /proc/mounts,
// statfs-ing each, so the operator can pick which disks to monitor. Bind mounts
// and duplicate mount points are collapsed; pseudo/virtual filesystems are
// skipped.
func (h *HostSampler) Mounts(_ context.Context) ([]biz.Mount, error) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil, fmt.Errorf("open /proc/mounts: %w", err)
	}
	defer f.Close()
	seen := map[string]bool{}
	var out []biz.Mount
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		device, mount, fstype := fields[0], unescapeMount(fields[1]), fields[2]
		if pseudoFS[fstype] || seen[mount] {
			continue
		}
		if underSystemDir(mount) {
			continue
		}
		du, ok := sampleDisk(mount)
		if !ok || du.TotalBytes == 0 {
			continue
		}
		seen[mount] = true
		out = append(out, biz.Mount{
			Path: mount, Device: device, FSType: fstype,
			UsedPercent: du.UsedPercent, UsedBytes: du.UsedBytes, TotalBytes: du.TotalBytes,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, sc.Err()
}

// underSystemDir skips mount points under kernel/virtual trees.
func underSystemDir(mount string) bool {
	for _, p := range []string{"/proc", "/sys", "/dev", "/run"} {
		if mount == p || strings.HasPrefix(mount, p+"/") {
			return true
		}
	}
	return false
}

// unescapeMount decodes the octal escapes /proc/mounts uses for special chars.
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(s)
}

// sampleDisk returns usage for a filesystem path via statfs. ok=false when the
// path can't be stat-ed (e.g. missing mount) so the caller can skip it.
func sampleDisk(path string) (biz.DiskUsage, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return biz.DiskUsage{}, false
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return biz.DiskUsage{}, false
	}
	bsize := uint64(st.Bsize)
	if st.Blocks == 0 || bsize == 0 {
		return biz.DiskUsage{}, false
	}
	usedBlocks := st.Blocks - st.Bfree
	return biz.DiskUsage{
		Path:        path,
		UsedPercent: float64(usedBlocks) / float64(st.Blocks) * 100,
		UsedBytes:   usedBlocks * bsize,
		TotalBytes:  st.Blocks * bsize,
	}, true
}
