package util

import (
	"github.com/jaypipes/ghw"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/mem"
)

type SystemInformation struct {
	Platform string
	CPU      string
	GPU      []string
	RAM      uint64
}

func GetSystemInformation() (SystemInformation, error) {
	info := SystemInformation{GPU: make([]string, 0)}

	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return info, err
	}
	info.RAM = vmStat.Total / 1024 / 1024

	cpuStat, err := cpu.Info()
	if err != nil {
		return info, err
	}
	if len(cpuStat) > 0 && cpuStat[0].ModelName != "" {
		info.CPU = cpuStat[0].ModelName
	}

	// Get GPUs - don't fail if GPU detection fails (e.g., in Docker)
	gpus, err := getGpus()
	if err == nil {
		info.GPU = gpus
	}
	// Continue even if GPU detection fails

	hostStat, err := host.Info()
	if err != nil {
		return info, err
	}
	info.Platform = hostStat.Platform

	return info, nil
}

// TODO fix docker GPU
func getGpus() ([]string, error) {
	gpus := make([]string, 0)

	gpu, err := ghw.GPU()
	if err != nil {
		// Return empty list instead of error to allow system info to be collected
		// even when GPU detection fails (e.g., in Docker containers)
		return gpus, nil
	}

	if gpu == nil {
		return gpus, nil
	}

	for _, card := range gpu.GraphicsCards {
		// Safely access nested fields with nil checks
		if card == nil {
			continue
		}
		if card.DeviceInfo == nil {
			continue
		}
		if card.DeviceInfo.Product == nil {
			continue
		}
		if card.DeviceInfo.Product.Name != "" {
			gpus = append(gpus, card.DeviceInfo.Product.Name)
		}
	}

	return gpus, nil
}
