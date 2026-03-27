// ArticDBM XDP IP Blocklist Filter
// High-performance kernel-level IP blocking for database proxy

#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/ipv6.h>
#include <linux/in.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

// Maximum number of blocked IPs (can be tuned based on memory constraints)
#define MAX_BLOCKED_IPS 65536
#define MAX_BLOCKED_CIDRS 4096

// Action codes
#define ACTION_PASS 0
#define ACTION_DROP 1
#define ACTION_RATE_LIMIT 2

// Structure for blocked IP entries
struct blocked_ip {
    __u32 ip_addr;      // IPv4 address in network byte order
    __u64 block_time;   // Timestamp when blocked
    __u32 reason_code;  // Reason for blocking (SQL injection, rate limit, etc.)
    __u32 flags;        // Additional flags
};

// Structure for CIDR blocks
struct blocked_cidr {
    __u32 network;      // Network address
    __u32 mask;         // Netmask
    __u64 block_time;   // Timestamp when blocked
    __u32 reason_code;  // Reason for blocking
    __u32 flags;        // Additional flags
};

// Statistics structure
struct ip_filter_stats {
    __u64 total_packets;
    __u64 blocked_packets;
    __u64 allowed_packets;
    __u64 ipv6_packets;
    __u64 non_ip_packets;
    __u64 sql_injection_blocks;
    __u64 rate_limit_blocks;
    __u64 manual_blocks;
};

// Hash map for blocked individual IPs
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_BLOCKED_IPS);
    __type(key, __u32);              // IPv4 address
    __type(value, struct blocked_ip); // Block info
    __uint(pinned, LIBBPF_PIN_BY_NAME);
} blocked_ips SEC(".maps");

// Array map for blocked CIDR ranges
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, MAX_BLOCKED_CIDRS);
    __type(key, __u32);                // Index
    __type(value, struct blocked_cidr); // CIDR info
    __uint(pinned, LIBBPF_PIN_BY_NAME);
} blocked_cidrs SEC(".maps");

// Statistics map
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct ip_filter_stats);
    __uint(pinned, LIBBPF_PIN_BY_NAME);
} stats_map SEC(".maps");

// Configuration map for runtime parameters
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 16);
    __type(key, __u32);
    __type(value, __u64);
    __uint(pinned, LIBBPF_PIN_BY_NAME);
} config_map SEC(".maps");

// Helper function to update statistics
static __always_inline void update_stats(__u32 stat_type) {
    __u32 key = 0;
    struct ip_filter_stats *stats = bpf_map_lookup_elem(&stats_map, &key);
    if (!stats) return;

    switch (stat_type) {
        case 0: // total packets
            __sync_fetch_and_add(&stats->total_packets, 1);
            break;
        case 1: // blocked packets
            __sync_fetch_and_add(&stats->blocked_packets, 1);
            break;
        case 2: // allowed packets
            __sync_fetch_and_add(&stats->allowed_packets, 1);
            break;
        case 3: // ipv6 packets
            __sync_fetch_and_add(&stats->ipv6_packets, 1);
            break;
        case 4: // non-ip packets
            __sync_fetch_and_add(&stats->non_ip_packets, 1);
            break;
        case 5: // sql injection blocks
            __sync_fetch_and_add(&stats->sql_injection_blocks, 1);
            break;
        case 6: // rate limit blocks
            __sync_fetch_and_add(&stats->rate_limit_blocks, 1);
            break;
        case 7: // manual blocks
            __sync_fetch_and_add(&stats->manual_blocks, 1);
            break;
    }
}

// Check if IP is in a blocked CIDR range
static __always_inline int check_cidr_block(__u32 ip_addr) {
    struct blocked_cidr *cidr;
    __u32 i;

    // Check first 64 CIDR entries (unrolled for performance)
    #pragma unroll
    for (i = 0; i < 64; i++) {
        cidr = bpf_map_lookup_elem(&blocked_cidrs, &i);
        if (!cidr || cidr->network == 0) continue;

        if ((ip_addr & cidr->mask) == cidr->network) {
            return 1; // Found matching CIDR block
        }
    }

    // Check remaining CIDR entries with bounded loop
    for (i = 64; i < MAX_BLOCKED_CIDRS && i < 1024; i++) {
        cidr = bpf_map_lookup_elem(&blocked_cidrs, &i);
        if (!cidr || cidr->network == 0) break;

        if ((ip_addr & cidr->mask) == cidr->network) {
            return 1; // Found matching CIDR block
        }
    }

    return 0; // No matching CIDR block found
}

// Main XDP program for IP filtering
SEC("xdp")
int xdp_ip_filter(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;

    // Update total packet counter
    update_stats(0);

    // Parse Ethernet header
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end) {
        update_stats(4); // non-ip packets
        return XDP_PASS;
    }

    // Handle IPv4 packets
    if (eth->h_proto == bpf_htons(ETH_P_IP)) {
        struct iphdr *ip = (void *)(eth + 1);
        if ((void *)(ip + 1) > data_end) {
            update_stats(4); // non-ip packets
            return XDP_PASS;
        }

        __u32 src_ip = ip->saddr;

        // Check if source IP is individually blocked
        struct blocked_ip *blocked = bpf_map_lookup_elem(&blocked_ips, &src_ip);
        if (blocked) {
            // Update statistics based on reason
            if (blocked->reason_code == 1) {
                update_stats(5); // sql injection blocks
            } else if (blocked->reason_code == 2) {
                update_stats(6); // rate limit blocks
            } else {
                update_stats(7); // manual blocks
            }

            update_stats(1); // blocked packets
            return XDP_DROP;
        }

        // Check if source IP is in a blocked CIDR range
        if (check_cidr_block(src_ip)) {
            update_stats(1); // blocked packets
            update_stats(7); // manual blocks (assuming CIDR blocks are manual)
            return XDP_DROP;
        }

        // Check destination port for database services
        if (ip->protocol == IPPROTO_TCP) {
            struct tcphdr *tcp = (void *)ip + (ip->ihl * 4);
            if ((void *)(tcp + 1) > data_end) {
                update_stats(2); // allowed packets
                return XDP_PASS;
            }

            __u16 dport = bpf_ntohs(tcp->dest);

            // Common database ports: 3306 (MySQL), 5432 (PostgreSQL),
            // 1433 (MSSQL), 27017 (MongoDB), 6379 (Redis)
            if (dport == 3306 || dport == 5432 || dport == 1433 ||
                dport == 27017 || dport == 6379) {

                // This is database traffic, apply strict filtering
                // Additional checks could be added here for suspicious patterns

                update_stats(2); // allowed packets
                return XDP_PASS;
            }
        }

        update_stats(2); // allowed packets
        return XDP_PASS;
    }

    // Handle IPv6 packets (basic passthrough for now)
    if (eth->h_proto == bpf_htons(ETH_P_IPV6)) {
        update_stats(3); // ipv6 packets
        return XDP_PASS;
    }

    // Non-IP traffic
    update_stats(4); // non-ip packets
    return XDP_PASS;
}

// Secondary XDP program for advanced filtering (can be chained)
SEC("xdp")
int xdp_advanced_filter(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;

    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end) {
        return XDP_PASS;
    }

    if (eth->h_proto == bpf_htons(ETH_P_IP)) {
        struct iphdr *ip = (void *)(eth + 1);
        if ((void *)(ip + 1) > data_end) {
            return XDP_PASS;
        }

        // Advanced filtering logic can be added here
        // - Deep packet inspection for SQL injection patterns
        // - Protocol-specific filtering
        // - Connection tracking
        // - Behavioral analysis

        return XDP_PASS;
    }

    return XDP_PASS;
}

// XDP program for emergency DDoS mitigation
SEC("xdp")
int xdp_ddos_mitigation(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;

    // Get DDoS mitigation configuration
    __u32 config_key = 0; // emergency_mode
    __u64 *emergency_mode = bpf_map_lookup_elem(&config_map, &config_key);
    if (!emergency_mode || *emergency_mode == 0) {
        return XDP_PASS; // Not in emergency mode
    }

    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end) {
        return XDP_PASS;
    }

    if (eth->h_proto == bpf_htons(ETH_P_IP)) {
        struct iphdr *ip = (void *)(eth + 1);
        if ((void *)(ip + 1) > data_end) {
            return XDP_PASS;
        }

        // In emergency mode, implement aggressive filtering
        // - Only allow known good IPs
        // - Drop SYN flood attempts
        // - Rate limit aggressively

        // Simple rate limiting: drop every other packet in emergency mode
        __u64 current_time = bpf_ktime_get_ns();
        if ((current_time / 1000) % 2 == 0) {
            update_stats(1); // blocked packets
            return XDP_DROP;
        }

        return XDP_PASS;
    }

    return XDP_PASS;
}

char _license[] SEC("license") = "GPL";