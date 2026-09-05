package calibration

import (
	"context"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Runtime/internal/hostprofile"
)

const SchemaVersion = "quantum.runtime/host-calibration/v1alpha1"

type Sample struct {
	Workers               int    `json:"workers"`
	MemoryCopyBytesPerSec uint64 `json:"memory_copy_bytes_per_sec"`
	MemoryReadBytesPerSec uint64 `json:"memory_read_bytes_per_sec"`
}

type Result struct {
	SchemaVersion         string    `json:"schema_version"`
	MeasuredAt            time.Time `json:"measured_at"`
	DurationMillis        int64     `json:"duration_ms"`
	BestWorkers           int       `json:"best_workers"`
	MemoryCopyBytesPerSec uint64    `json:"memory_copy_bytes_per_sec"`
	MemoryReadBytesPerSec uint64    `json:"memory_read_bytes_per_sec"`
	MemoryBandwidthClass  string    `json:"memory_bandwidth_class"`
	Samples               []Sample  `json:"samples"`
	Checksum              uint64    `json:"checksum"`
}

func Run(ctx context.Context, host hostprofile.Profile, budget time.Duration) Result {
	if budget <= 0 {
		budget = 250 * time.Millisecond
	}
	if budget > 2*time.Second {
		budget = 2 * time.Second
	}
	physical := host.CPU.PhysicalCores
	if physical <= 0 {
		physical = runtime.NumCPU()
	}
	logical := host.CPU.LogicalCores
	if logical <= 0 {
		logical = runtime.NumCPU()
	}
	candidates := workerCandidates(physical, logical)
	perSample := budget / time.Duration(len(candidates))
	if perSample < 20*time.Millisecond {
		perSample = 20 * time.Millisecond
	}

	started := time.Now()
	result := Result{SchemaVersion: SchemaVersion, MeasuredAt: time.Now().UTC()}
	var bestScore uint64
	for _, workers := range candidates {
		copyRate := measureParallelCopy(ctx, workers, perSample/2)
		readRate, checksum := measureParallelRead(ctx, workers, perSample/2)
		result.Checksum += checksum
		result.Samples = append(result.Samples, Sample{
			Workers:               workers,
			MemoryCopyBytesPerSec: copyRate,
			MemoryReadBytesPerSec: readRate,
		})
		score := min(copyRate, readRate)
		if score > bestScore {
			bestScore = score
			result.BestWorkers = workers
			result.MemoryCopyBytesPerSec = copyRate
			result.MemoryReadBytesPerSec = readRate
		}
		if ctx.Err() != nil {
			break
		}
	}
	result.DurationMillis = time.Since(started).Milliseconds()
	result.MemoryBandwidthClass = classifyBandwidth(max(result.MemoryCopyBytesPerSec, result.MemoryReadBytesPerSec))
	return result
}

func workerCandidates(physical, logical int) []int {
	if physical < 1 {
		physical = 1
	}
	if logical < physical {
		logical = physical
	}
	set := map[int]struct{}{1: {}}
	for _, n := range []int{2, 4, 8, 16, 32, 64} {
		if n <= physical {
			set[n] = struct{}{}
		}
	}
	set[physical] = struct{}{}
	if logical > physical {
		set[logical] = struct{}{}
	}
	out := make([]int, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

func measureParallelCopy(ctx context.Context, workers int, budget time.Duration) uint64 {
	if budget <= 0 || workers < 1 {
		return 0
	}
	chunkSize := (8 << 20) / workers
	if chunkSize < 64<<10 {
		chunkSize = 64 << 10
	}

	sources := make([][]byte, workers)
	destinations := make([][]byte, workers)
	for worker := 0; worker < workers; worker++ {
		sources[worker] = make([]byte, chunkSize)
		destinations[worker] = make([]byte, chunkSize)
		seed := byte(worker)
		for i := range sources[worker] {
			sources[worker][i] = byte(i) ^ seed
		}
	}

	started := time.Now()
	deadline := started.Add(budget)
	counts := make(chan uint64, workers)
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(src, dst []byte) {
			defer wg.Done()
			var total uint64
			for time.Now().Before(deadline) {
				select {
				case <-ctx.Done():
					counts <- total
					return
				default:
				}
				copy(dst, src)
				total += uint64(len(src))
			}
			runtime.KeepAlive(dst)
			counts <- total
		}(sources[worker], destinations[worker])
	}
	wg.Wait()
	close(counts)
	var total uint64
	for n := range counts {
		total += n
	}
	return bytesPerSecond(total, time.Since(started))
}

func measureParallelRead(ctx context.Context, workers int, budget time.Duration) (uint64, uint64) {
	if budget <= 0 || workers < 1 {
		return 0, 0
	}
	chunkSize := (8 << 20) / workers
	if chunkSize < 64<<10 {
		chunkSize = 64 << 10
	}

	buffers := make([][]byte, workers)
	for worker := 0; worker < workers; worker++ {
		buffers[worker] = make([]byte, chunkSize)
		seed := byte(worker)
		for i := range buffers[worker] {
			buffers[worker][i] = byte(i*31+7) ^ seed
		}
	}

	started := time.Now()
	deadline := started.Add(budget)
	type measurement struct{ bytes, sum uint64 }
	counts := make(chan measurement, workers)
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(buf []byte) {
			defer wg.Done()
			var total, sum uint64
			for time.Now().Before(deadline) {
				select {
				case <-ctx.Done():
					counts <- measurement{bytes: total, sum: sum}
					return
				default:
				}
				for i := 0; i < len(buf); i += 64 {
					sum += uint64(buf[i])
				}
				total += uint64(len(buf))
			}
			runtime.KeepAlive(sum)
			counts <- measurement{bytes: total, sum: sum}
		}(buffers[worker])
	}
	wg.Wait()
	close(counts)
	var total, sum uint64
	for n := range counts {
		total += n.bytes
		sum += n.sum
	}
	return bytesPerSecond(total, time.Since(started)), sum
}

func bytesPerSecond(bytes uint64, elapsed time.Duration) uint64 {
	if elapsed <= 0 {
		return 0
	}
	return uint64(float64(bytes) / elapsed.Seconds())
}

func classifyBandwidth(bytesPerSecond uint64) string {
	const gib = uint64(1 << 30)
	switch {
	case bytesPerSecond >= 100*gib:
		return "very_high"
	case bytesPerSecond >= 50*gib:
		return "high"
	case bytesPerSecond >= 20*gib:
		return "medium"
	case bytesPerSecond > 0:
		return "low"
	default:
		return "unknown"
	}
}
