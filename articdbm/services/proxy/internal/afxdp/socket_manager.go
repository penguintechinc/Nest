package afxdp

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"golang.org/x/sys/unix"
	"go.uber.org/zap"

	"github.com/penguintechinc/articdbm/proxy/internal/numa"
)

const (
	FRAME_SIZE       = 2048
	NUM_FRAMES       = 4096
	FILL_QUEUE_SIZE  = NUM_FRAMES * 2
	COMP_QUEUE_SIZE  = NUM_FRAMES
	RX_BATCH_SIZE    = 64
	TX_BATCH_SIZE    = 64
	SOCKET_TIMEOUT   = 1000 // milliseconds
)

// AF_XDP socket modes
const (
	XDP_ZEROCOPY = 1 << 0  // Zero-copy mode (requires driver support)
	XDP_COPY     = 1 << 1  // Copy mode (fallback)
	XDP_USE_NEED_WAKEUP = 1 << 3  // Use need_wakeup flag optimization
)

// XDP socket address structure
type XDPSockAddr struct {
	Family    uint16
	Flags     uint16
	IfIndex   uint32
	QueueID   uint32
	SharedUMem uint32
}

// UMEM (User Memory) configuration
type UMEMConfig struct {
	FillSize      uint32
	CompSize      uint32
	FrameSize     uint32
	FrameHeadroom uint32
	Flags         uint32
}

// XDP ring structures (simplified representations)
type XDPDesc struct {
	Addr uint64
	Len  uint32
	Options uint32
}

type XDPRing struct {
	Producer uint32
	Consumer uint32
	Flags    uint32
	Pad      uint32
	// Ring follows...
}

// AF_XDP Socket wrapper
type AFXDPSocket struct {
	fd          int
	ifIndex     int
	queueID     int
	rxRing      *XDPRing
	txRing      *XDPRing
	fillRing    *XDPRing
	compRing    *XDPRing
	umemArea    []byte
	frameOffsets []uint64
	rxBatch     [RX_BATCH_SIZE]XDPDesc
	txBatch     [TX_BATCH_SIZE]XDPDesc
	stats       AFXDPStats
	logger      *zap.Logger
}

// AF_XDP Socket Manager
type SocketManager struct {
	sockets     []*AFXDPSocket
	interfaces  []string
	numQueues   int
	workers     []chan *Packet
	topology    *numa.TopologyInfo
	xdpProgram  *ebpf.Program
	xdpLink     link.Link
	logger      *zap.Logger
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	stats       AFXDPManagerStats
}

// Packet structure for AF_XDP processing
type Packet struct {
	Data      []byte
	Length    int
	Timestamp time.Time
	IfIndex   int
	QueueID   int
	RxDesc    XDPDesc
}

// Statistics structures
type AFXDPStats struct {
	RxPackets    uint64 `json:"rx_packets"`
	TxPackets    uint64 `json:"tx_packets"`
	RxBytes      uint64 `json:"rx_bytes"`
	TxBytes      uint64 `json:"tx_bytes"`
	RxDropped    uint64 `json:"rx_dropped"`
	TxErrors     uint64 `json:"tx_errors"`
	RxBatches    uint64 `json:"rx_batches"`
	TxBatches    uint64 `json:"tx_batches"`
	ZeroCopyRx   uint64 `json:"zero_copy_rx"`
	ZeroCopyTx   uint64 `json:"zero_copy_tx"`
}

type AFXDPManagerStats struct {
	TotalSockets   int                    `json:"total_sockets"`
	ActiveWorkers  int                    `json:"active_workers"`
	SocketStats    map[string]AFXDPStats  `json:"socket_stats"`
	PacketRate     uint64                 `json:"packet_rate"`
	ByteRate       uint64                 `json:"byte_rate"`
	LastUpdate     time.Time              `json:"last_update"`
}

// Packet handler function type
type PacketHandler func(ctx context.Context, packet *Packet) error

func NewSocketManager(interfaces []string, topology *numa.TopologyInfo, logger *zap.Logger) (*SocketManager, error) {
	ctx, cancel := context.WithCancel(context.Background())

	numQueues := len(interfaces)
	if numQueues == 0 {
		numQueues = 1
	}

	sm := &SocketManager{
		interfaces: interfaces,
		numQueues:  numQueues,
		workers:    make([]chan *Packet, numQueues),
		topology:   topology,
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
		stats: AFXDPManagerStats{
			SocketStats: make(map[string]AFXDPStats),
		},
	}

	if err := sm.initialize(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize AF_XDP socket manager: %w", err)
	}

	return sm, nil
}

func (sm *SocketManager) initialize() error {
	// Load XDP program for AF_XDP redirection
	if err := sm.loadXDPProgram(); err != nil {
		return fmt.Errorf("failed to load XDP program: %w", err)
	}

	// Create AF_XDP sockets for each interface/queue
	for i, iface := range sm.interfaces {
		socket, err := sm.createAFXDPSocket(iface, i)
		if err != nil {
			sm.logger.Warn("Failed to create AF_XDP socket",
				zap.String("interface", iface),
				zap.Int("queue", i),
				zap.Error(err))
			continue
		}

		sm.sockets = append(sm.sockets, socket)

		// Create worker channel for this socket
		sm.workers[i] = make(chan *Packet, 1024)

		// Start worker goroutine with NUMA affinity
		sm.wg.Add(1)
		go sm.socketWorker(socket, sm.workers[i])
	}

	sm.stats.TotalSockets = len(sm.sockets)
	sm.stats.ActiveWorkers = len(sm.workers)

	// Start statistics collector
	go sm.statsCollector()

	sm.logger.Info("AF_XDP socket manager initialized",
		zap.Int("sockets", len(sm.sockets)),
		zap.Int("workers", len(sm.workers)))

	return nil
}

func (sm *SocketManager) loadXDPProgram() error {
	// Load XDP program that redirects packets to AF_XDP sockets
	xdpProgram := `
		#include <linux/bpf.h>
		#include <bpf/bpf_helpers.h>

		struct {
			__uint(type, BPF_MAP_TYPE_XSKMAP);
			__uint(max_entries, 64);
			__uint(key_size, sizeof(int));
			__uint(value_size, sizeof(int));
		} xsks_map SEC(".maps");

		SEC("xdp_sock")
		int xdp_sock_prog(struct xdp_md *ctx) {
			int index = ctx->rx_queue_index;
			return bpf_redirect_map(&xsks_map, index, XDP_DROP);
		}
	`
	_ = xdpProgram

	// This would normally be compiled from C and loaded
	// For now, return success as a placeholder
	return nil
}

func (sm *SocketManager) createAFXDPSocket(iface string, queueID int) (*AFXDPSocket, error) {
	netIface, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, fmt.Errorf("interface %s not found: %w", iface, err)
	}

	// Create AF_XDP socket
	fd, err := unix.Socket(unix.AF_XDP, unix.SOCK_RAW, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to create AF_XDP socket: %w", err)
	}

	socket := &AFXDPSocket{
		fd:      fd,
		ifIndex: netIface.Index,
		queueID: queueID,
		logger:  sm.logger,
	}

	// Allocate UMEM (User Memory)
	if err := socket.setupUMEM(); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("failed to setup UMEM: %w", err)
	}

	// Setup RX and TX rings
	if err := socket.setupRings(); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("failed to setup rings: %w", err)
	}

	// Bind socket to interface and queue
	if err := socket.bind(); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("failed to bind socket: %w", err)
	}

	return socket, nil
}

func (s *AFXDPSocket) setupUMEM() error {
	// Allocate memory for UMEM
	umemSize := NUM_FRAMES * FRAME_SIZE
	umemArea := make([]byte, umemSize)

	// Align to page boundary
	pageSize := unix.Getpagesize()
	aligned := uintptr(unsafe.Pointer(&umemArea[0]))
	aligned = (aligned + uintptr(pageSize-1)) & ^uintptr(pageSize-1)
	s.umemArea = (*[1 << 30]byte)(unsafe.Pointer(aligned))[:umemSize:umemSize]

	// Calculate frame offsets
	s.frameOffsets = make([]uint64, NUM_FRAMES)
	for i := 0; i < NUM_FRAMES; i++ {
		s.frameOffsets[i] = uint64(i * FRAME_SIZE)
	}

	// Register UMEM with kernel
	umemConfig := UMEMConfig{
		FillSize:      FILL_QUEUE_SIZE,
		CompSize:      COMP_QUEUE_SIZE,
		FrameSize:     FRAME_SIZE,
		FrameHeadroom: 0,
		Flags:         0,
	}

	_, _, errno := syscall.Syscall6(
		syscall.SYS_SETSOCKOPT,
		uintptr(s.fd),
		unix.SOL_XDP,
		unix.XDP_UMEM_REG,
		uintptr(unsafe.Pointer(&s.umemArea[0])),
		uintptr(len(s.umemArea)),
		uintptr(unsafe.Pointer(&umemConfig)),
	)

	if errno != 0 {
		return fmt.Errorf("failed to register UMEM: %v", errno)
	}

	return nil
}

func (s *AFXDPSocket) setupRings() error {
	// Setup RX ring
	rxRingSize := unsafe.Sizeof(XDPRing{}) + uintptr(NUM_FRAMES*int(unsafe.Sizeof(XDPDesc{})))
	rxRingMem, err := unix.Mmap(s.fd, unix.XDP_PGOFF_RX_RING,
		int(rxRingSize), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("failed to mmap RX ring: %w", err)
	}
	s.rxRing = (*XDPRing)(unsafe.Pointer(&rxRingMem[0]))

	// Setup TX ring
	txRingSize := unsafe.Sizeof(XDPRing{}) + uintptr(NUM_FRAMES*int(unsafe.Sizeof(XDPDesc{})))
	txRingMem, err := unix.Mmap(s.fd, unix.XDP_PGOFF_TX_RING,
		int(txRingSize), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("failed to mmap TX ring: %w", err)
	}
	s.txRing = (*XDPRing)(unsafe.Pointer(&txRingMem[0]))

	// Setup FILL ring
	fillRingSize := unsafe.Sizeof(XDPRing{}) + uintptr(FILL_QUEUE_SIZE*8)
	fillRingMem, err := unix.Mmap(s.fd, unix.XDP_UMEM_PGOFF_FILL_RING,
		int(fillRingSize), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("failed to mmap FILL ring: %w", err)
	}
	s.fillRing = (*XDPRing)(unsafe.Pointer(&fillRingMem[0]))

	// Setup COMPLETION ring
	compRingSize := unsafe.Sizeof(XDPRing{}) + uintptr(COMP_QUEUE_SIZE*8)
	compRingMem, err := unix.Mmap(s.fd, unix.XDP_UMEM_PGOFF_COMPLETION_RING,
		int(compRingSize), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("failed to mmap COMPLETION ring: %w", err)
	}
	s.compRing = (*XDPRing)(unsafe.Pointer(&compRingMem[0]))

	return nil
}

func (s *AFXDPSocket) bind() error {
	addr := XDPSockAddr{
		Family:  unix.AF_XDP,
		Flags:   XDP_USE_NEED_WAKEUP,
		IfIndex: uint32(s.ifIndex),
		QueueID: uint32(s.queueID),
	}

	_, _, errno := syscall.Syscall(
		syscall.SYS_BIND,
		uintptr(s.fd),
		uintptr(unsafe.Pointer(&addr)),
		unsafe.Sizeof(addr),
	)

	if errno != 0 {
		return fmt.Errorf("failed to bind AF_XDP socket: %v", errno)
	}

	return nil
}

func (sm *SocketManager) socketWorker(socket *AFXDPSocket, packetChan chan *Packet) {
	defer sm.wg.Done()

	// Set CPU affinity for NUMA optimization
	if sm.topology != nil {
		if cpuID, err := sm.topology.GetOptimalCPUForNIC(""); err == nil {
			if err := sm.topology.SetCPUAffinity(cpuID); err != nil {
				sm.logger.Warn("Failed to set CPU affinity", zap.Error(err))
			}
		}
	}

	// Pin goroutine to OS thread for performance
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	sm.logger.Info("AF_XDP worker started",
		zap.Int("queue_id", socket.queueID),
		zap.Int("if_index", socket.ifIndex))

	for {
		select {
		case <-sm.ctx.Done():
			return
		default:
			// Process RX packets
			if err := socket.processRX(packetChan); err != nil {
				sm.logger.Error("Error processing RX packets", zap.Error(err))
			}

			// Process TX completion
			if err := socket.processTXCompletion(); err != nil {
				sm.logger.Error("Error processing TX completion", zap.Error(err))
			}
		}
	}
}

func (s *AFXDPSocket) processRX(packetChan chan *Packet) error {
	// Check if we need wakeup
	if s.rxRing.Flags&unix.XDP_RING_NEED_WAKEUP != 0 {
		// Perform syscall to wake up kernel
		unix.Syscall(unix.SYS_RECVMSG, uintptr(s.fd), 0, unix.MSG_DONTWAIT)
	}

	producer := atomic.LoadUint32(&s.rxRing.Producer)
	consumer := atomic.LoadUint32(&s.rxRing.Consumer)

	available := producer - consumer
	if available == 0 {
		return nil // No packets available
	}

	batchSize := available
	if batchSize > RX_BATCH_SIZE {
		batchSize = RX_BATCH_SIZE
	}

	// Process batch of packets
	for i := uint32(0); i < batchSize; i++ {
		idx := (consumer + i) & (NUM_FRAMES - 1)
		desc := s.getRXDesc(idx)

		// Create packet from descriptor
		packet := &Packet{
			Data:      s.umemArea[desc.Addr:desc.Addr+uint64(desc.Len)],
			Length:    int(desc.Len),
			Timestamp: time.Now(),
			IfIndex:   s.ifIndex,
			QueueID:   s.queueID,
			RxDesc:    desc,
		}

		// Send to packet channel (non-blocking)
		select {
		case packetChan <- packet:
			atomic.AddUint64(&s.stats.RxPackets, 1)
			atomic.AddUint64(&s.stats.RxBytes, uint64(desc.Len))
		default:
			atomic.AddUint64(&s.stats.RxDropped, 1)
		}
	}

	// Update consumer pointer
	atomic.StoreUint32(&s.rxRing.Consumer, consumer+batchSize)
	atomic.AddUint64(&s.stats.RxBatches, 1)

	return nil
}

func (s *AFXDPSocket) processTXCompletion() error {
	producer := atomic.LoadUint32(&s.compRing.Producer)
	consumer := atomic.LoadUint32(&s.compRing.Consumer)

	completed := producer - consumer
	if completed == 0 {
		return nil // No completions
	}

	// Process completed TX frames
	for i := uint32(0); i < completed; i++ {
		// Return frame to available pool
		atomic.AddUint64(&s.stats.TxPackets, 1)
	}

	atomic.StoreUint32(&s.compRing.Consumer, consumer+completed)
	return nil
}

func (s *AFXDPSocket) getRXDesc(idx uint32) XDPDesc {
	// Calculate descriptor address in RX ring
	ringStart := uintptr(unsafe.Pointer(s.rxRing)) + unsafe.Sizeof(XDPRing{})
	descAddr := ringStart + uintptr(idx)*unsafe.Sizeof(XDPDesc{})
	return *(*XDPDesc)(unsafe.Pointer(descAddr))
}

func (sm *SocketManager) StartPacketProcessing(handler PacketHandler) error {
	for i, worker := range sm.workers {
		sm.wg.Add(1)
		go sm.packetProcessor(worker, handler, i)
	}

	sm.logger.Info("Packet processing started",
		zap.Int("processors", len(sm.workers)))

	return nil
}

func (sm *SocketManager) packetProcessor(packetChan chan *Packet, handler PacketHandler, workerID int) {
	defer sm.wg.Done()

	sm.logger.Info("Packet processor started", zap.Int("worker_id", workerID))

	for {
		select {
		case <-sm.ctx.Done():
			return
		case packet := <-packetChan:
			if err := handler(sm.ctx, packet); err != nil {
				sm.logger.Error("Error processing packet",
					zap.Int("worker_id", workerID),
					zap.Error(err))
			}
		}
	}
}

func (sm *SocketManager) statsCollector() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var lastRxPackets uint64

	for {
		select {
		case <-sm.ctx.Done():
			return
		case <-ticker.C:
			totalRx, totalTx := uint64(0), uint64(0)

			for i, socket := range sm.sockets {
				stats := AFXDPStats{
					RxPackets:    atomic.LoadUint64(&socket.stats.RxPackets),
					TxPackets:    atomic.LoadUint64(&socket.stats.TxPackets),
					RxBytes:      atomic.LoadUint64(&socket.stats.RxBytes),
					TxBytes:      atomic.LoadUint64(&socket.stats.TxBytes),
					RxDropped:    atomic.LoadUint64(&socket.stats.RxDropped),
					TxErrors:     atomic.LoadUint64(&socket.stats.TxErrors),
					RxBatches:    atomic.LoadUint64(&socket.stats.RxBatches),
					TxBatches:    atomic.LoadUint64(&socket.stats.TxBatches),
					ZeroCopyRx:   atomic.LoadUint64(&socket.stats.ZeroCopyRx),
					ZeroCopyTx:   atomic.LoadUint64(&socket.stats.ZeroCopyTx),
				}

				sm.stats.SocketStats[fmt.Sprintf("socket_%d", i)] = stats
				totalRx += stats.RxPackets
				totalTx += stats.TxPackets
			}

			// Calculate packet rates
			sm.stats.PacketRate = totalRx - lastRxPackets
			lastRxPackets = totalRx

			sm.stats.LastUpdate = time.Now()
		}
	}
}

func (sm *SocketManager) GetStats() AFXDPManagerStats {
	return sm.stats
}

func (sm *SocketManager) Close() error {
	sm.cancel()

	// Close all sockets
	for _, socket := range sm.sockets {
		if err := unix.Close(socket.fd); err != nil {
			sm.logger.Warn("Failed to close AF_XDP socket", zap.Error(err))
		}
	}

	// Close XDP link
	if sm.xdpLink != nil {
		sm.xdpLink.Close()
	}

	// Wait for all workers to finish
	sm.wg.Wait()

	sm.logger.Info("AF_XDP socket manager stopped")
	return nil
}