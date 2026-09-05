package hostprofile

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const SchemaVersion = "quantum.runtime/host-profile/v1alpha1"

type Profile struct {
	SchemaVersion string               `json:"schema_version"`
	CollectedAt   time.Time            `json:"collected_at"`
	OS            string               `json:"os"`
	Arch          string               `json:"arch"`
	CPU           CPUProfile           `json:"cpu"`
	NUMA          NUMAProfile          `json:"numa"`
	Memory        MemoryProfile        `json:"memory"`
	Storage       []StorageProfile     `json:"storage"`
	Accelerators  []AcceleratorProfile `json:"accelerators"`
	Warnings      []string             `json:"warnings,omitempty"`
}

type CPUProfile struct {
	VendorID       string   `json:"vendor_id,omitempty"`
	ModelName      string   `json:"model_name,omitempty"`
	PhysicalCores  int      `json:"physical_cores"`
	LogicalCores   int      `json:"logical_cores"`
	ThreadsPerCore int      `json:"threads_per_core"`
	Features       []string `json:"features,omitempty"`
}

type NUMAProfile struct {
	Count int        `json:"count"`
	Nodes []NUMANode `json:"nodes,omitempty"`
}

type NUMANode struct {
	ID          int    `json:"id"`
	CPUList     string `json:"cpu_list,omitempty"`
	MemoryBytes uint64 `json:"memory_bytes,omitempty"`
	FreeBytes   uint64 `json:"free_bytes,omitempty"`
}

type MemoryProfile struct {
	TotalBytes        uint64 `json:"total_bytes"`
	AvailableBytes    uint64 `json:"available_bytes"`
	HugePageSizeBytes uint64 `json:"huge_page_size_bytes,omitempty"`
	HugePagesTotal    uint64 `json:"huge_pages_total,omitempty"`
	HugePagesFree     uint64 `json:"huge_pages_free,omitempty"`
}

type StorageProfile struct {
	Device     string `json:"device"`
	Kind       string `json:"kind"`
	SizeBytes  uint64 `json:"size_bytes,omitempty"`
	Rotational bool   `json:"rotational"`
	NUMANode   *int   `json:"numa_node,omitempty"`
}

type AcceleratorProfile struct {
	Kind      string `json:"kind"`
	Vendor    string `json:"vendor"`
	Device    string `json:"device"`
	VRAMBytes uint64 `json:"vram_bytes,omitempty"`
	NUMANode  *int   `json:"numa_node,omitempty"`
}

func Discover() Profile {
	p := Profile{
		SchemaVersion: SchemaVersion,
		CollectedAt:   time.Now().UTC(),
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		CPU: CPUProfile{
			LogicalCores: runtime.NumCPU(),
		},
	}
	if err := discoverCPU(&p); err != nil {
		p.Warnings = append(p.Warnings, "cpu: "+err.Error())
	}
	if err := discoverMemory(&p); err != nil {
		p.Warnings = append(p.Warnings, "memory: "+err.Error())
	}
	if err := discoverNUMA(&p); err != nil {
		p.Warnings = append(p.Warnings, "numa: "+err.Error())
	}
	if err := discoverStorage(&p); err != nil {
		p.Warnings = append(p.Warnings, "storage: "+err.Error())
	}
	if err := discoverAccelerators(&p); err != nil {
		p.Warnings = append(p.Warnings, "accelerators: "+err.Error())
	}
	if p.CPU.PhysicalCores <= 0 {
		p.CPU.PhysicalCores = p.CPU.LogicalCores
	}
	if p.CPU.PhysicalCores > 0 {
		p.CPU.ThreadsPerCore = max(1, p.CPU.LogicalCores/p.CPU.PhysicalCores)
	}
	sort.Strings(p.CPU.Features)
	return p
}

func discoverCPU(p *Profile) error {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return err
	}
	defer f.Close()

	type coreKey struct{ socket, core string }
	cores := map[coreKey]struct{}{}
	var physicalID, coreID string
	featureSet := map[string]struct{}{}
	flushCore := func() {
		if coreID != "" {
			cores[coreKey{socket: physicalID, core: coreID}] = struct{}{}
		}
		physicalID, coreID = "", ""
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			flushCore()
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "vendor_id", "CPU implementer":
			if p.CPU.VendorID == "" {
				p.CPU.VendorID = value
			}
		case "model name", "Processor", "Hardware":
			if p.CPU.ModelName == "" {
				p.CPU.ModelName = value
			}
		case "physical id":
			physicalID = value
		case "core id":
			coreID = value
		case "flags", "Features":
			for _, feature := range strings.Fields(value) {
				featureSet[strings.ToLower(feature)] = struct{}{}
			}
		}
	}
	flushCore()
	if err := scanner.Err(); err != nil {
		return err
	}
	if len(cores) > 0 {
		p.CPU.PhysicalCores = len(cores)
	}
	for feature := range featureSet {
		p.CPU.Features = append(p.CPU.Features, feature)
	}
	return nil
}

func discoverMemory(p *Profile) error {
	values, err := parseMemInfo("/proc/meminfo")
	if err != nil {
		return err
	}
	p.Memory.TotalBytes = values["MemTotal"] * 1024
	p.Memory.AvailableBytes = values["MemAvailable"] * 1024
	if p.Memory.AvailableBytes == 0 {
		p.Memory.AvailableBytes = (values["MemFree"] + values["Buffers"] + values["Cached"]) * 1024
	}
	p.Memory.HugePageSizeBytes = values["Hugepagesize"] * 1024
	p.Memory.HugePagesTotal = values["HugePages_Total"]
	p.Memory.HugePagesFree = values["HugePages_Free"]
	return nil
}

func parseMemInfo(path string) (map[string]uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	values := map[string]uint64{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		key, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		value, err := strconv.ParseUint(fields[0], 10, 64)
		if err == nil {
			values[key] = value
		}
	}
	return values, scanner.Err()
}

func discoverNUMA(p *Profile) error {
	paths, err := filepath.Glob("/sys/devices/system/node/node[0-9]*")
	if err != nil {
		return err
	}
	for _, path := range paths {
		base := filepath.Base(path)
		id, err := strconv.Atoi(strings.TrimPrefix(base, "node"))
		if err != nil {
			continue
		}
		node := NUMANode{ID: id}
		if data, err := os.ReadFile(filepath.Join(path, "cpulist")); err == nil {
			node.CPUList = strings.TrimSpace(string(data))
		}
		if values, err := parseNodeMemInfo(filepath.Join(path, "meminfo")); err == nil {
			node.MemoryBytes = values["MemTotal"] * 1024
			node.FreeBytes = values["MemFree"] * 1024
		}
		p.NUMA.Nodes = append(p.NUMA.Nodes, node)
	}
	sort.Slice(p.NUMA.Nodes, func(i, j int) bool { return p.NUMA.Nodes[i].ID < p.NUMA.Nodes[j].ID })
	p.NUMA.Count = len(p.NUMA.Nodes)
	return nil
}

func parseNodeMemInfo(path string) (map[string]uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	values := map[string]uint64{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		key := strings.TrimSuffix(fields[2], ":")
		value, err := strconv.ParseUint(fields[3], 10, 64)
		if err == nil {
			values[key] = value
		}
	}
	return values, scanner.Err()
}

func discoverStorage(p *Profile) error {
	devices, err := filepath.Glob("/sys/block/*")
	if err != nil {
		return err
	}
	for _, path := range devices {
		name := filepath.Base(path)
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "fd") {
			continue
		}
		storage := StorageProfile{Device: name, Kind: "block"}
		if strings.HasPrefix(name, "nvme") {
			storage.Kind = "nvme"
		}
		if data, err := os.ReadFile(filepath.Join(path, "size")); err == nil {
			if sectors, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); err == nil {
				storage.SizeBytes = sectors * 512
			}
		}
		if data, err := os.ReadFile(filepath.Join(path, "queue", "rotational")); err == nil {
			storage.Rotational = strings.TrimSpace(string(data)) == "1"
		}
		if data, err := os.ReadFile(filepath.Join(path, "device", "numa_node")); err == nil {
			if value, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && value >= 0 {
				storage.NUMANode = &value
			}
		}
		p.Storage = append(p.Storage, storage)
	}
	sort.Slice(p.Storage, func(i, j int) bool { return p.Storage[i].Device < p.Storage[j].Device })
	return nil
}

func discoverAccelerators(p *Profile) error {
	cards, err := filepath.Glob("/sys/class/drm/card[0-9]*")
	if err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, card := range cards {
		devicePath := filepath.Join(card, "device")
		resolved, err := filepath.EvalSymlinks(devicePath)
		if err != nil {
			continue
		}
		if _, ok := seen[resolved]; ok {
			continue
		}
		seen[resolved] = struct{}{}
		vendorBytes, err := os.ReadFile(filepath.Join(devicePath, "vendor"))
		if err != nil {
			continue
		}
		vendorID := strings.ToLower(strings.TrimSpace(string(vendorBytes)))
		vendor, kind := acceleratorVendor(vendorID)
		if vendor == "" {
			continue
		}
		acc := AcceleratorProfile{Kind: kind, Vendor: vendor, Device: filepath.Base(card)}
		if data, err := os.ReadFile(filepath.Join(devicePath, "mem_info_vram_total")); err == nil {
			if value, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); err == nil {
				acc.VRAMBytes = value
			}
		}
		if data, err := os.ReadFile(filepath.Join(devicePath, "numa_node")); err == nil {
			if value, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && value >= 0 {
				acc.NUMANode = &value
			}
		}
		p.Accelerators = append(p.Accelerators, acc)
	}
	return nil
}

func acceleratorVendor(id string) (string, string) {
	switch id {
	case "0x1002":
		return "AMD", "gpu"
	case "0x10de":
		return "NVIDIA", "gpu"
	case "0x8086":
		return "Intel", "gpu"
	default:
		return "", ""
	}
}

func (p Profile) Validate() error {
	if p.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unexpected host profile schema %q", p.SchemaVersion)
	}
	if p.CPU.LogicalCores <= 0 {
		return errors.New("logical CPU count must be positive")
	}
	if p.CPU.PhysicalCores <= 0 || p.CPU.PhysicalCores > p.CPU.LogicalCores {
		return errors.New("physical CPU count is invalid")
	}
	if p.Memory.TotalBytes > 0 && p.Memory.AvailableBytes > p.Memory.TotalBytes {
		return errors.New("available memory exceeds total memory")
	}
	return nil
}

func HasFeature(p Profile, feature string) bool {
	feature = strings.ToLower(strings.TrimSpace(feature))
	i := sort.SearchStrings(p.CPU.Features, feature)
	return i < len(p.CPU.Features) && p.CPU.Features[i] == feature
}
