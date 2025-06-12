package controllers

import (
	"fmt"
	"time"
	"xivi/backend/pkg/streaming"

	"github.com/gofiber/fiber/v2"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

// SystemStatus represents the system status response
type SystemStatus struct {
	Uptime      string `json:"uptime"`
	CPU         string `json:"cpu"`
	Memory      string `json:"memory"`
	Connections int    `json:"connections"`
	Timestamp   int64  `json:"timestamp"`
}

// GetSystemStatus
// @Description Get system status metrics including uptime, CPU, memory, and connections.
// @Summary get system status metrics
// @Tags System
// @Accept json
// @Produce json
// @Success 200 {object} SystemStatus
// @Router /system/status [get]
func GetSystemStatus(c *fiber.Ctx) error {
	// Get system uptime
	uptime, err := getSystemUptime()
	if err != nil {
		uptime = "Unknown"
	}

	// Get CPU usage
	cpuUsage, err := getCPUUsage()
	if err != nil {
		cpuUsage = "0%"
	}

	// Get memory usage
	memoryUsage, err := getMemoryUsage()
	if err != nil {
		memoryUsage = "0 MB / 0 MB (0%)"
	}

	// Get active stream connections
	connections := getActiveConnections()

	// Create system status response
	status := SystemStatus{
		Uptime:      uptime,
		CPU:         cpuUsage,
		Memory:      memoryUsage,
		Connections: connections,
		Timestamp:   time.Now().Unix(),
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":  false,
		"msg":    nil,
		"status": status,
	})
}

// getSystemUptime returns formatted system uptime
func getSystemUptime() (string, error) {
	hostInfo, err := host.Info()
	if err != nil {
		return "", err
	}

	uptime := time.Duration(hostInfo.Uptime) * time.Second
	days := int(uptime.Hours()) / 24
	hours := int(uptime.Hours()) % 24
	minutes := int(uptime.Minutes()) % 60

	return fmt.Sprintf("%d days, %d hours, %d minutes", days, hours, minutes), nil
}

// getCPUUsage returns formatted CPU usage percentage
func getCPUUsage() (string, error) {
	percentages, err := cpu.Percent(time.Second, false)
	if err != nil {
		return "", err
	}

	if len(percentages) > 0 {
		return fmt.Sprintf("%.1f%%", percentages[0]), nil
	}

	return "0%", nil
}

// getMemoryUsage returns formatted memory usage
func getMemoryUsage() (string, error) {
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		return "", err
	}

	usedMB := memInfo.Used / 1024 / 1024
	totalMB := memInfo.Total / 1024 / 1024
	usedPercent := memInfo.UsedPercent

	return fmt.Sprintf("%d MB / %d MB (%.1f%%)", usedMB, totalMB, usedPercent), nil
}

// getActiveConnections returns the total number of active stream connections
func getActiveConnections() int {
	// For now, return the number of active streams as a proxy for connections
	// This is a simplified approach since we can't access the private activeClients field
	return len(streaming.Streams)
}
