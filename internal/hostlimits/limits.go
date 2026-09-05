package hostlimits

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const SchemaVersion = "quantum.runtime/host-limits/v1alpha1"

type Limits struct {
	SchemaVersion          string         `json:"schema_version"`
	CollectedAt            time.Time      `json:"collected_at"`
	EffectiveLogicalCPUs   int            `json:"effective_logical_cpus"`
	AffinityCPUList        string         `json:"affinity_cpu_list,omitempty"`
	AffinityLogicalCPUs    int            `json:"affinity_logical_cpus,omitempty"`
	CgroupCPUSet           string         `json:"cgroup_cpuset,omitempty"`
	CgroupCPUSetLogicalCPUs int           `json:"cgroup_cpuset_logical_cpus,omitempty"`
	CPUQuotaCores          float64        `json:"cpu_quota_cores,omitempty"`
	Virtualization         Virtualization `json:"virtualization"`
	Warnings               []string       `json:"warnings,omitempty"`
}

type Virtualization struct {
	Guest    bool   `json:"guest"`
	Kind     string `json:"kind"`
	Evidence string `json:"evidence,omitempty"`
}

func Discover() Limits {
	limits := Limits{
		SchemaVersion:        SchemaVersion,
		CollectedAt:          time.Now().UTC(),
		EffectiveLogicalCPUs: runtime.NumCPU(),
		Virtualization:       discoverVirtualization(),
	}

	if cpuList, err := processAffinityCPUList("/proc/self/status"); err == nil {
		limits.AffinityCPUList = cpuList
		if count, err := countCPUList(cpuList); err == nil {
			limits.AffinityLogicalCPUs = count
			limits.EffectiveLogicalCPUs = positiveMin(limits.EffectiveLogicalCPUs, count)
		} else {
			limits.Warnings = append(limits.Warnings, "affinity cpulist: "+err.Error())
		}
	} else {
		limits.Warnings = append(limits.Warnings, "process affinity: "+err.Error())
	}

	if cpuList, path, ok := discoverCgroupCPUSet(); ok {
		limits.CgroupCPUSet = cpuList
		if count, err := countCPUList(cpuList); err == nil {
			limits.CgroupCPUSetLogicalCPUs = count
			limits.EffectiveLogicalCPUs = positiveMin(limits.EffectiveLogicalCPUs, count)
		} else {
			limits.Warnings = append(limits.Warnings, path+": "+err.Error())
		}
	}

	if quota, ok, err := discoverCPUQuota(); err != nil {
		limits.Warnings = append(limits.Warnings, "cpu quota: "+err.Error())
	} else if ok {
		limits.CPUQuotaCores = quota
	}

	if limits.EffectiveLogicalCPUs < 1 {
		limits.EffectiveLogicalCPUs = runtime.NumCPU()
	}
	return limits
}

func (l Limits) Validate() error {
	if l.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unexpected host limits schema %q", l.SchemaVersion)
	}
	if l.EffectiveLogicalCPUs < 1 {
		return fmt.Errorf("effective logical CPU count must be positive")
	}
	if l.CPUQuotaCores < 0 {
		return fmt.Errorf("cpu quota must not be negative")
	}
	return nil
}

func processAffinityCPUList(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "Cpus_allowed_list:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "Cpus_allowed_list:"))
		if value == "" {
			return "", fmt.Errorf("Cpus_allowed_list is empty")
		}
		return value, nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("Cpus_allowed_list not found")
}

func discoverCgroupCPUSet() (string, string, bool) {
	paths := []string{
		"/sys/fs/cgroup/cpuset.cpus.effective",
		"/sys/fs/cgroup/cpuset.cpus",
		"/sys/fs/cgroup/cpuset/cpuset.cpus",
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		value := strings.TrimSpace(string(data))
		if value != "" {
			return value, path, true
		}
	}
	return "", "", false
}

func discoverCPUQuota() (float64, bool, error) {
	data, err := os.ReadFile("/sys/fs/cgroup/cpu.max")
	if err == nil {
		fields := strings.Fields(string(data))
		if len(fields) != 2 {
			return 0, false, fmt.Errorf("invalid cpu.max")
		}
		if fields[0] == "max" {
			return 0, false, nil
		}
		quota, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			return 0, false, err
		}
		period, err := strconv.ParseFloat(fields[1], 64)
		if err != nil || period <= 0 {
			return 0, false, fmt.Errorf("invalid cpu.max period")
		}
		return quota / period, true, nil
	}

	quotaData, quotaErr := os.ReadFile("/sys/fs/cgroup/cpu/cpu.cfs_quota_us")
	periodData, periodErr := os.ReadFile("/sys/fs/cgroup/cpu/cpu.cfs_period_us")
	if quotaErr != nil || periodErr != nil {
		return 0, false, nil
	}
	quota, err := strconv.ParseFloat(strings.TrimSpace(string(quotaData)), 64)
	if err != nil {
		return 0, false, err
	}
	if quota < 0 {
		return 0, false, nil
	}
	period, err := strconv.ParseFloat(strings.TrimSpace(string(periodData)), 64)
	if err != nil || period <= 0 {
		return 0, false, fmt.Errorf("invalid cgroup v1 cpu period")
	}
	return quota / period, true, nil
}

func discoverVirtualization() Virtualization {
	type evidenceFile struct {
		path  string
		label string
	}
	files := []evidenceFile{
		{path: "/sys/class/dmi/id/product_name", label: "dmi product"},
		{path: "/sys/class/dmi/id/sys_vendor", label: "dmi vendor"},
		{path: "/sys/hypervisor/type", label: "hypervisor"},
	}
	for _, item := range files {
		data, err := os.ReadFile(item.path)
		if err != nil {
			continue
		}
		value := strings.ToLower(strings.TrimSpace(string(data)))
		if kind := virtualizationKind(value); kind != "" {
			return Virtualization{Guest: true, Kind: kind, Evidence: item.label + ": " + strings.TrimSpace(string(data))}
		}
	}

	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil && strings.Contains(strings.ToLower(string(data)), " hypervisor") {
		return Virtualization{Guest: true, Kind: "unknown-hypervisor", Evidence: "cpu hypervisor flag"}
	}
	return Virtualization{Guest: false, Kind: "bare-metal-or-undetected"}
}

func virtualizationKind(value string) string {
	switch {
	case strings.Contains(value, "kvm"), strings.Contains(value, "qemu"):
		return "kvm"
	case strings.Contains(value, "vmware"):
		return "vmware"
	case strings.Contains(value, "virtualbox"):
		return "virtualbox"
	case strings.Contains(value, "microsoft"), strings.Contains(value, "hyper-v"), strings.Contains(value, "virtual machine"):
		return "hyper-v"
	case strings.Contains(value, "xen"):
		return "xen"
	default:
		return ""
	}
}

func countCPUList(value string) (int, error) {
	set := map[int]struct{}{}
	for _, part := range strings.Split(strings.TrimSpace(value), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			start, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
			if err != nil {
				return 0, fmt.Errorf("invalid CPU range %q", part)
			}
			end, err := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err != nil || end < start {
				return 0, fmt.Errorf("invalid CPU range %q", part)
			}
			for cpu := start; cpu <= end; cpu++ {
				set[cpu] = struct{}{}
			}
			continue
		}
		cpu, err := strconv.Atoi(part)
		if err != nil || cpu < 0 {
			return 0, fmt.Errorf("invalid CPU %q", part)
		}
		set[cpu] = struct{}{}
	}
	if len(set) == 0 {
		return 0, fmt.Errorf("CPU list is empty")
	}
	return len(set), nil
}

func positiveMin(values ...int) int {
	filtered := make([]int, 0, len(values))
	for _, value := range values {
		if value > 0 {
			filtered = append(filtered, value)
		}
	}
	if len(filtered) == 0 {
		return 0
	}
	sort.Ints(filtered)
	return filtered[0]
}
