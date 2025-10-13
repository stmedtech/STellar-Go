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
	info.CPU = cpuStat[0].ModelName

	gpus, err := getGpus()
	if err != nil {
		return info, err
	}
	info.GPU = gpus

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
		return gpus, err
	}

	for _, card := range gpu.GraphicsCards {
		gpus = append(gpus, card.DeviceInfo.Product.Name)
	}

	return gpus, nil
}
