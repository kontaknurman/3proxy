package main

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Status is a snapshot of the VPS and the 3proxy process. Everything is read
// from /proc, so it is only filled in on Linux.
type Status struct {
	Hostname  string      `json:"hostname"`
	OS        string      `json:"os"`
	Kernel    string      `json:"kernel"`
	Uptime    float64     `json:"uptime"`
	Load      [3]float64  `json:"load"`
	Cores     int         `json:"cores"`
	CPU       float64     `json:"cpu"`
	MemTotal  uint64      `json:"mem_total"`
	MemUsed   uint64      `json:"mem_used"`
	SwapTotal uint64      `json:"swap_total"`
	SwapUsed  uint64      `json:"swap_used"`
	DiskTotal uint64      `json:"disk_total"`
	DiskUsed  uint64      `json:"disk_used"`
	NetRx     uint64      `json:"net_rx"`
	NetTx     uint64      `json:"net_tx"`
	NetRxRate float64     `json:"net_rx_rate"`
	NetTxRate float64     `json:"net_tx_rate"`
	Proxy     ProcessInfo `json:"proxy"`
	Conns     map[int]int `json:"conns"`
}

// ProcessInfo describes the running 3proxy. It uses a thread per
// connection, so the thread count follows the load.
type ProcessInfo struct {
	Running bool    `json:"running"`
	PID     int     `json:"pid"`
	Threads int     `json:"threads"`
	RSS     uint64  `json:"rss"`
	Uptime  float64 `json:"uptime"`
}

// sampler measures CPU usage and network throughput, which need two
// readings some time apart.
type sampler struct {
	mu        sync.Mutex
	cpu       float64
	rxRate    float64
	txRate    float64
	lastBusy  uint64
	lastTotal uint64
	lastRx    uint64
	lastTx    uint64
	lastTime  time.Time
}

func (s *sampler) run(interval time.Duration) {
	for {
		s.sample()
		time.Sleep(interval)
	}
}

func (s *sampler) sample() {
	busy, total := readCPU()
	rx, tx := readNet()
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.lastTime.IsZero() {
		if dt := total - s.lastTotal; total > s.lastTotal {
			s.cpu = float64(busy-s.lastBusy) / float64(dt) * 100
		}
		if secs := now.Sub(s.lastTime).Seconds(); secs > 0 && rx >= s.lastRx && tx >= s.lastTx {
			s.rxRate = float64(rx-s.lastRx) / secs
			s.txRate = float64(tx-s.lastTx) / secs
		}
	}
	s.lastBusy, s.lastTotal, s.lastRx, s.lastTx, s.lastTime = busy, total, rx, tx, now
}

func (s *sampler) status(ports []int) Status {
	st := Status{Cores: runtime.NumCPU()}
	st.Hostname, _ = os.Hostname()
	st.OS = readOSName()
	st.Kernel = readFirstLine("/proc/sys/kernel/osrelease")
	st.Uptime = readUptime()
	if f := strings.Fields(readFirstLine("/proc/loadavg")); len(f) >= 3 {
		for i := 0; i < 3; i++ {
			st.Load[i], _ = strconv.ParseFloat(f[i], 64)
		}
	}

	mem := readKeyValues("/proc/meminfo")
	st.MemTotal = mem["MemTotal"] * 1024
	if avail, ok := mem["MemAvailable"]; ok {
		st.MemUsed = st.MemTotal - avail*1024
	} else {
		st.MemUsed = st.MemTotal - (mem["MemFree"]+mem["Buffers"]+mem["Cached"])*1024
	}
	st.SwapTotal = mem["SwapTotal"] * 1024
	st.SwapUsed = st.SwapTotal - mem["SwapFree"]*1024

	var fs syscall.Statfs_t
	if syscall.Statfs("/", &fs) == nil {
		st.DiskTotal = fs.Blocks * uint64(fs.Bsize)
		st.DiskUsed = (fs.Blocks - fs.Bfree) * uint64(fs.Bsize)
	}

	st.NetRx, st.NetTx = readNet()
	s.mu.Lock()
	st.CPU, st.NetRxRate, st.NetTxRate = s.cpu, s.rxRate, s.txRate
	s.mu.Unlock()

	st.Proxy = findProcess("3proxy", st.Uptime)
	st.Conns = countConnections(ports)
	return st
}

func readFirstLine(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(data), "\n")
	return strings.TrimSpace(line)
}

// readOSName names the distribution. In a container the host's os-release
// is expected at /host/etc/os-release, its own describes the image.
func readOSName() string {
	for _, path := range []string{"/host/etc/os-release", "/etc/os-release"} {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if v, ok := strings.CutPrefix(sc.Text(), "PRETTY_NAME="); ok {
				return strings.Trim(v, `"'`)
			}
		}
	}
	return runtime.GOOS
}

func readUptime() float64 {
	f := strings.Fields(readFirstLine("/proc/uptime"))
	if len(f) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(f[0], 64)
	return v
}

// readKeyValues reads "Key: number ..." lines, as in /proc/meminfo and
// /proc/<pid>/status.
func readKeyValues(path string) map[string]uint64 {
	m := map[string]uint64{}
	f, err := os.Open(path)
	if err != nil {
		return m
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		if fields := strings.Fields(v); len(fields) > 0 {
			m[k], _ = strconv.ParseUint(fields[0], 10, 64)
		}
	}
	return m
}

// readCPU returns the busy and total jiffies of all CPUs together.
func readCPU() (busy, total uint64) {
	f := strings.Fields(readFirstLine("/proc/stat"))
	if len(f) < 5 || f[0] != "cpu" {
		return 0, 0
	}
	for i, v := range f[1:] {
		n, _ := strconv.ParseUint(v, 10, 64)
		// guest and guest_nice are already counted in user and nice.
		if i >= 8 {
			break
		}
		total += n
		// idle and iowait
		if i != 3 && i != 4 {
			busy += n
		}
	}
	return busy, total
}

// readNet returns the bytes received and sent on all interfaces but the
// loopback.
func readNet() (rx, tx uint64) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok || strings.TrimSpace(name) == "lo" {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 9 {
			continue
		}
		r, _ := strconv.ParseUint(fields[0], 10, 64)
		t, _ := strconv.ParseUint(fields[8], 10, 64)
		rx += r
		tx += t
	}
	return rx, tx
}

// findProcess looks for a process by its command name. sysUptime is the
// system uptime the process start time is subtracted from.
func findProcess(name string, sysUptime float64) ProcessInfo {
	dirs, _ := filepath.Glob("/proc/[0-9]*")
	for _, dir := range dirs {
		if readFirstLine(dir+"/comm") != name {
			continue
		}
		pid, _ := strconv.Atoi(filepath.Base(dir))
		info := ProcessInfo{Running: true, PID: pid}
		status := readKeyValues(dir + "/status")
		info.Threads = int(status["Threads"])
		info.RSS = status["VmRSS"] * 1024
		// The fields after the command name, which is in parentheses and
		// may contain spaces; starttime is the 22nd field of the line.
		stat := readFirstLine(dir + "/stat")
		if i := strings.LastIndexByte(stat, ')'); i >= 0 {
			if f := strings.Fields(stat[i+1:]); len(f) > 19 {
				// Clock ticks since boot; USER_HZ is 100 on Linux.
				start, _ := strconv.ParseFloat(f[19], 64)
				info.Uptime = sysUptime - start/100
			}
		}
		return info
	}
	return ProcessInfo{}
}

// countConnections counts the established TCP connections to each of the
// given local ports.
func countConnections(ports []int) map[int]int {
	counts := make(map[int]int, len(ports))
	want := make(map[int]bool, len(ports))
	for _, p := range ports {
		counts[p] = 0
		want[p] = true
	}
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Scan() // header
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			// 01 is TCP_ESTABLISHED
			if len(fields) < 4 || fields[3] != "01" {
				continue
			}
			_, hexPort, ok := strings.Cut(fields[1], ":")
			if !ok {
				continue
			}
			port, err := strconv.ParseUint(hexPort, 16, 16)
			if err == nil && want[int(port)] {
				counts[int(port)]++
			}
		}
		f.Close()
	}
	return counts
}
