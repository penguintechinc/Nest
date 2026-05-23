package numa

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

type TopologyInfo struct {
	NumaNodes     []NumaNode
	CPUCores      []CPUCore
	TotalMemory   uint64
	TotalCores    int
	NICInterfaces []NICInfo
	Nodes         map[int]*Node
	logger        *zap.Logger
}

// Node represents a NUMA node with runtime metrics for monitoring
type Node struct {
	ID          int
	MemoryUsed  uint64
	MemoryFree  uint64
	CPUUsage    map[int]float64
	NICs        []NICInfo
}

type NumaNode struct {
	ID            int
	CPUs          []int
	Memory        uint64
	Distance      []int
	LocalMemory   uint64
	AvailMemory   uint64
}

type CPUCore struct {
	ID         int
	NumaNode   int
	Siblings   []int
	Online     bool
	Frequency  int
}

type NICInfo struct {
	Name       string
	NumaNode   int
	Queues     int
	Driver     string
	PCIAddr    string
}

func NewTopologyInfo(logger *zap.Logger) (*TopologyInfo, error) {
	topo := &TopologyInfo{
		logger: logger,
	}

	if err := topo.discoverTopology(); err != nil {
		return nil, fmt.Errorf("failed to discover NUMA topology: %w", err)
	}

	return topo, nil
}

func (t *TopologyInfo) discoverTopology() error {
	if err := t.discoverNumaNodes(); err != nil {
		return fmt.Errorf("failed to discover NUMA nodes: %w", err)
	}

	if err := t.discoverCPUs(); err != nil {
		return fmt.Errorf("failed to discover CPUs: %w", err)
	}

	if err := t.discoverNICs(); err != nil {
		return fmt.Errorf("failed to discover NICs: %w", err)
	}

	t.logger.Info("NUMA topology discovered",
		zap.Int("numa_nodes", len(t.NumaNodes)),
		zap.Int("cpu_cores", len(t.CPUCores)),
		zap.Int("nics", len(t.NICInterfaces)))

	return nil
}

func (t *TopologyInfo) discoverNumaNodes() error {
	nodesPath := "/sys/devices/system/node"

	entries, err := os.ReadDir(nodesPath)
	if err != nil {
		// Single NUMA node system
		t.NumaNodes = []NumaNode{{
			ID:   0,
			CPUs: make([]int, runtime.NumCPU()),
		}}
		for i := 0; i < runtime.NumCPU(); i++ {
			t.NumaNodes[0].CPUs[i] = i
		}
		return nil
	}

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "node") {
			continue
		}

		nodeIDStr := strings.TrimPrefix(entry.Name(), "node")
		nodeID, err := strconv.Atoi(nodeIDStr)
		if err != nil {
			continue
		}

		node := NumaNode{ID: nodeID}

		if err := t.discoverNodeCPUs(&node); err != nil {
			t.logger.Warn("Failed to discover CPUs for NUMA node",
				zap.Int("node", nodeID), zap.Error(err))
		}

		if err := t.discoverNodeMemory(&node); err != nil {
			t.logger.Warn("Failed to discover memory for NUMA node",
				zap.Int("node", nodeID), zap.Error(err))
		}

		t.NumaNodes = append(t.NumaNodes, node)
	}

	return nil
}

func (t *TopologyInfo) discoverNodeCPUs(node *NumaNode) error {
	cpuListPath := fmt.Sprintf("/sys/devices/system/node/node%d/cpulist", node.ID)

	content, err := os.ReadFile(cpuListPath)
	if err != nil {
		return err
	}

	cpuList := strings.TrimSpace(string(content))
	cpus, err := parseCPUList(cpuList)
	if err != nil {
		return err
	}

	node.CPUs = cpus
	return nil
}

func (t *TopologyInfo) discoverNodeMemory(node *NumaNode) error {
	meminfoPath := fmt.Sprintf("/sys/devices/system/node/node%d/meminfo", node.ID)

	file, err := os.Open(meminfoPath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "MemTotal:") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				memKB, err := strconv.ParseUint(parts[3], 10, 64)
				if err == nil {
					node.Memory = memKB * 1024
				}
			}
		} else if strings.Contains(line, "MemFree:") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				memKB, err := strconv.ParseUint(parts[3], 10, 64)
				if err == nil {
					node.AvailMemory = memKB * 1024
				}
			}
		}
	}

	return scanner.Err()
}

func (t *TopologyInfo) discoverCPUs() error {
	for nodeIdx := range t.NumaNodes {
		node := &t.NumaNodes[nodeIdx]

		for _, cpuID := range node.CPUs {
			core := CPUCore{
				ID:       cpuID,
				NumaNode: node.ID,
				Online:   true,
			}

			if err := t.discoverCPUInfo(&core); err != nil {
				t.logger.Warn("Failed to discover CPU info",
					zap.Int("cpu", cpuID), zap.Error(err))
			}

			t.CPUCores = append(t.CPUCores, core)
		}
	}

	t.TotalCores = len(t.CPUCores)
	return nil
}

func (t *TopologyInfo) discoverCPUInfo(core *CPUCore) error {
	onlinePath := fmt.Sprintf("/sys/devices/system/cpu/cpu%d/online", core.ID)
	if content, err := os.ReadFile(onlinePath); err == nil {
		online := strings.TrimSpace(string(content)) == "1"
		core.Online = online
	}

	topologyPath := fmt.Sprintf("/sys/devices/system/cpu/cpu%d/topology/thread_siblings_list", core.ID)
	if content, err := os.ReadFile(topologyPath); err == nil {
		siblingsList := strings.TrimSpace(string(content))
		if siblings, err := parseCPUList(siblingsList); err == nil {
			core.Siblings = siblings
		}
	}

	return nil
}

func (t *TopologyInfo) discoverNICs() error {
	netPath := "/sys/class/net"

	entries, err := os.ReadDir(netPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		nicName := entry.Name()
		if nicName == "lo" {
			continue
		}

		nic := NICInfo{Name: nicName}

		if err := t.discoverNICInfo(&nic); err != nil {
			t.logger.Warn("Failed to discover NIC info",
				zap.String("nic", nicName), zap.Error(err))
			continue
		}

		t.NICInterfaces = append(t.NICInterfaces, nic)
	}

	return nil
}

func (t *TopologyInfo) discoverNICInfo(nic *NICInfo) error {
	devicePath := fmt.Sprintf("/sys/class/net/%s/device", nic.Name)

	realPath, err := filepath.EvalSymlinks(devicePath)
	if err != nil {
		return err
	}

	numaNodePath := filepath.Join(realPath, "numa_node")
	if content, err := os.ReadFile(numaNodePath); err == nil {
		if nodeID, err := strconv.Atoi(strings.TrimSpace(string(content))); err == nil && nodeID >= 0 {
			nic.NumaNode = nodeID
		}
	}

	queuesPath := fmt.Sprintf("/sys/class/net/%s/queues", nic.Name)
	if entries, err := os.ReadDir(queuesPath); err == nil {
		rxQueues := 0
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "rx-") {
				rxQueues++
			}
		}
		nic.Queues = rxQueues
	}

	return nil
}

func parseCPUList(cpuList string) ([]int, error) {
	var cpus []int

	parts := strings.Split(cpuList, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				continue
			}

			start, err1 := strconv.Atoi(rangeParts[0])
			end, err2 := strconv.Atoi(rangeParts[1])
			if err1 != nil || err2 != nil {
				continue
			}

			for i := start; i <= end; i++ {
				cpus = append(cpus, i)
			}
		} else {
			cpu, err := strconv.Atoi(part)
			if err != nil {
				continue
			}
			cpus = append(cpus, cpu)
		}
	}

	return cpus, nil
}

func (t *TopologyInfo) GetOptimalCPUForNIC(nicName string) (int, error) {
	for _, nic := range t.NICInterfaces {
		if nic.Name == nicName {
			if nic.NumaNode < len(t.NumaNodes) && len(t.NumaNodes[nic.NumaNode].CPUs) > 0 {
				return t.NumaNodes[nic.NumaNode].CPUs[0], nil
			}
		}
	}

	if len(t.CPUCores) > 0 {
		return t.CPUCores[0].ID, nil
	}

	return -1, fmt.Errorf("no available CPU found")
}

func (t *TopologyInfo) SetCPUAffinity(cpuID int) error {
	var cpuSet unix.CPUSet
	cpuSet.Zero()
	cpuSet.Set(cpuID)

	return unix.SchedSetaffinity(0, &cpuSet)
}

func (t *TopologyInfo) GetNumaNodeForCPU(cpuID int) int {
	for _, core := range t.CPUCores {
		if core.ID == cpuID {
			return core.NumaNode
		}
	}
	return 0
}

func (t *TopologyInfo) AllocateMemoryOnNode(size uintptr, nodeID int) ([]byte, error) {
	data := make([]byte, size)

	if len(t.NumaNodes) <= 1 {
		return data, nil
	}

	addr := uintptr(unsafe.Pointer(&data[0]))

	const MPOL_BIND = 2
	const MPOL_MF_MOVE = 1 << 1

	nodemask := uint64(1 << uint(nodeID))

	_, _, errno := syscall.Syscall6(
		syscall.SYS_MBIND,
		addr,
		size,
		MPOL_BIND,
		uintptr(unsafe.Pointer(&nodemask)),
		64,
		MPOL_MF_MOVE,
	)

	if errno != 0 {
		return data, fmt.Errorf("mbind failed: %v", errno)
	}

	return data, nil
}