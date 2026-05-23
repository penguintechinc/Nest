// ArticDBM XDP Rate Limiter with Token Bucket Algorithm
// High-performance kernel-level rate limiting for database connections

#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/ipv6.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/in.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

// Rate limiting configuration
#define MAX_TRACKED_IPS 32768
#define DEFAULT_BUCKET_SIZE 100
#define DEFAULT_REFILL_RATE 10  // tokens per second
#define NSEC_PER_SEC 1000000000ULL

// Token bucket structure for per-IP rate limiting
struct token_bucket {
    __u64 tokens;           // Current tokens (in fixed point, multiply by 1000)
    __u64 last_refill;      // Last refill timestamp (nanoseconds)
    __u32 bucket_size;      // Maximum bucket size
    __u32 refill_rate;      // Tokens per second
    __u32 packets_allowed;  // Statistics: packets allowed
    __u32 packets_dropped;  // Statistics: packets dropped
};

// Rate limiter configuration structure
struct rate_limiter_config {
    __u32 enabled;              // Rate limiter enabled/disabled
    __u32 default_bucket_size;  // Default bucket size for new IPs
    __u32 default_refill_rate;  // Default refill rate (tokens/sec)
    __u32 burst_detection;      // Enable burst detection
    __u32 burst_threshold;      // Packets per burst window
    __u32 burst_window_ms;      // Burst detection window (milliseconds)
    __u32 emergency_mode;       // Emergency rate limiting mode
    __u32 whitelist_enabled;    // IP whitelist enabled
};

// Per-IP rate limiting state
struct {
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, MAX_TRACKED_IPS);
    __type(key, __u32);                    // IPv4 address
    __type(value, struct token_bucket);    // Token bucket state
    __uint(pinned, LIBBPF_PIN_BY_NAME);
} rate_limit_buckets SEC(".maps");

// Rate limiter configuration
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct rate_limiter_config);
    __uint(pinned, LIBBPF_PIN_BY_NAME);
} rate_limiter_config SEC(".maps");

// IP whitelist for rate limiting bypass
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 4096);
    __type(key, __u32);     // IPv4 address
    __type(value, __u32);   // flags/metadata
    __uint(pinned, LIBBPF_PIN_BY_NAME);
} rate_limit_whitelist SEC(".maps");

// Global rate limiting statistics
struct rate_limit_stats {
    __u64 total_requests;
    __u64 allowed_requests;
    __u64 rate_limited_requests;
    __u64 burst_detections;
    __u64 whitelist_bypasses;
    __u64 bucket_creations;
    __u64 bucket_refills;
    __u64 emergency_drops;
};

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct rate_limit_stats);
    __uint(pinned, LIBBPF_PIN_BY_NAME);
} rate_limit_stats_map SEC(".maps");

// Burst detection per IP
struct burst_tracker {
    __u64 window_start;     // Start of current window
    __u32 packet_count;     // Packets in current window
    __u32 burst_detected;   // Burst flag
};

struct {
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, MAX_TRACKED_IPS);
    __type(key, __u32);                    // IPv4 address
    __type(value, struct burst_tracker);   // Burst tracking state
    __uint(pinned, LIBBPF_PIN_BY_NAME);
} burst_trackers SEC(".maps");

// Helper function to update statistics
static __always_inline void update_rate_limit_stats(__u32 stat_type) {
    __u32 key = 0;
    struct rate_limit_stats *stats = bpf_map_lookup_elem(&rate_limit_stats_map, &key);
    if (!stats) return;

    switch (stat_type) {
        case 0: // total_requests
            __sync_fetch_and_add(&stats->total_requests, 1);
            break;
        case 1: // allowed_requests
            __sync_fetch_and_add(&stats->allowed_requests, 1);
            break;
        case 2: // rate_limited_requests
            __sync_fetch_and_add(&stats->rate_limited_requests, 1);
            break;
        case 3: // burst_detections
            __sync_fetch_and_add(&stats->burst_detections, 1);
            break;
        case 4: // whitelist_bypasses
            __sync_fetch_and_add(&stats->whitelist_bypasses, 1);
            break;
        case 5: // bucket_creations
            __sync_fetch_and_add(&stats->bucket_creations, 1);
            break;
        case 6: // bucket_refills
            __sync_fetch_and_add(&stats->bucket_refills, 1);
            break;
        case 7: // emergency_drops
            __sync_fetch_and_add(&stats->emergency_drops, 1);
            break;
    }
}

// Refill token bucket based on elapsed time
static __always_inline void refill_bucket(struct token_bucket *bucket,
                                          __u64 current_time,
                                          struct rate_limiter_config *config) {
    if (bucket->last_refill == 0) {
        bucket->last_refill = current_time;
        return;
    }

    __u64 elapsed_ns = current_time - bucket->last_refill;
    __u64 tokens_to_add = (elapsed_ns * bucket->refill_rate) / NSEC_PER_SEC;

    if (tokens_to_add > 0) {
        bucket->tokens += tokens_to_add * 1000;  // Fixed point arithmetic

        // Cap at bucket size
        __u32 max_tokens = bucket->bucket_size * 1000;
        if (bucket->tokens > max_tokens) {
            bucket->tokens = max_tokens;
        }

        bucket->last_refill = current_time;
        update_rate_limit_stats(6); // bucket_refills
    }
}

// Check and update burst detection
static __always_inline int check_burst_detection(__u32 src_ip,
                                                __u64 current_time,
                                                struct rate_limiter_config *config) {
    if (!config->burst_detection) {
        return 0; // Burst detection disabled
    }

    struct burst_tracker *tracker = bpf_map_lookup_elem(&burst_trackers, &src_ip);
    if (!tracker) {
        // Create new burst tracker
        struct burst_tracker new_tracker = {
            .window_start = current_time,
            .packet_count = 1,
            .burst_detected = 0
        };
        bpf_map_update_elem(&burst_trackers, &src_ip, &new_tracker, BPF_ANY);
        return 0;
    }

    __u64 window_duration = (__u64)config->burst_window_ms * 1000000ULL; // Convert to ns

    // Check if we need to reset the window
    if (current_time - tracker->window_start > window_duration) {
        tracker->window_start = current_time;
        tracker->packet_count = 1;
        tracker->burst_detected = 0;
        return 0;
    }

    tracker->packet_count++;

    // Check for burst
    if (tracker->packet_count > config->burst_threshold) {
        tracker->burst_detected = 1;
        update_rate_limit_stats(3); // burst_detections
        return 1; // Burst detected
    }

    return 0;
}

// Main XDP rate limiting program
SEC("xdp")
int xdp_rate_limiter(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;

    // Get configuration
    __u32 config_key = 0;
    struct rate_limiter_config *config = bpf_map_lookup_elem(&rate_limiter_config, &config_key);
    if (!config || !config->enabled) {
        return XDP_PASS; // Rate limiting disabled
    }

    update_rate_limit_stats(0); // total_requests

    // Parse Ethernet header
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end) {
        return XDP_PASS;
    }

    // Handle IPv4 packets only for now
    if (eth->h_proto != bpf_htons(ETH_P_IP)) {
        return XDP_PASS;
    }

    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end) {
        return XDP_PASS;
    }

    __u32 src_ip = ip->saddr;

    // Check whitelist
    if (config->whitelist_enabled) {
        __u32 *whitelist_entry = bpf_map_lookup_elem(&rate_limit_whitelist, &src_ip);
        if (whitelist_entry) {
            update_rate_limit_stats(4); // whitelist_bypasses
            return XDP_PASS;
        }
    }

    __u64 current_time = bpf_ktime_get_ns();

    // Emergency mode - aggressive rate limiting
    if (config->emergency_mode) {
        // In emergency mode, drop every other packet
        if ((current_time / 1000) % 2 == 0) {
            update_rate_limit_stats(7); // emergency_drops
            return XDP_DROP;
        }
    }

    // Check for burst detection
    if (check_burst_detection(src_ip, current_time, config)) {
        update_rate_limit_stats(2); // rate_limited_requests
        return XDP_DROP;
    }

    // Get or create token bucket for this IP
    struct token_bucket *bucket = bpf_map_lookup_elem(&rate_limit_buckets, &src_ip);
    if (!bucket) {
        // Create new bucket
        struct token_bucket new_bucket = {
            .tokens = config->default_bucket_size * 1000, // Fixed point
            .last_refill = current_time,
            .bucket_size = config->default_bucket_size,
            .refill_rate = config->default_refill_rate,
            .packets_allowed = 0,
            .packets_dropped = 0
        };

        if (bpf_map_update_elem(&rate_limit_buckets, &src_ip, &new_bucket, BPF_ANY) != 0) {
            // Failed to create bucket, allow packet (fail-open)
            update_rate_limit_stats(1); // allowed_requests
            return XDP_PASS;
        }

        bucket = bpf_map_lookup_elem(&rate_limit_buckets, &src_ip);
        if (!bucket) {
            // Still can't get bucket, allow packet
            update_rate_limit_stats(1); // allowed_requests
            return XDP_PASS;
        }

        update_rate_limit_stats(5); // bucket_creations
    }

    // Refill the bucket
    refill_bucket(bucket, current_time, config);

    // Check if we have tokens
    if (bucket->tokens >= 1000) { // 1 token in fixed point
        bucket->tokens -= 1000;
        bucket->packets_allowed++;
        update_rate_limit_stats(1); // allowed_requests
        return XDP_PASS;
    } else {
        bucket->packets_dropped++;
        update_rate_limit_stats(2); // rate_limited_requests
        return XDP_DROP;
    }
}

// XDP program for connection-based rate limiting (TCP SYN)
SEC("xdp")
int xdp_connection_rate_limiter(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;

    // Get configuration
    __u32 config_key = 0;
    struct rate_limiter_config *config = bpf_map_lookup_elem(&rate_limiter_config, &config_key);
    if (!config || !config->enabled) {
        return XDP_PASS;
    }

    // Parse Ethernet header
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end) {
        return XDP_PASS;
    }

    if (eth->h_proto != bpf_htons(ETH_P_IP)) {
        return XDP_PASS;
    }

    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end) {
        return XDP_PASS;
    }

    // Only process TCP packets
    if (ip->protocol != IPPROTO_TCP) {
        return XDP_PASS;
    }

    struct tcphdr *tcp = (void *)ip + (ip->ihl * 4);
    if ((void *)(tcp + 1) > data_end) {
        return XDP_PASS;
    }

    // Only rate limit SYN packets (new connections)
    if (!(tcp->syn && !tcp->ack)) {
        return XDP_PASS;
    }

    // Check destination port for database services
    __u16 dport = bpf_ntohs(tcp->dest);
    if (dport != 3306 && dport != 5432 && dport != 1433 &&
        dport != 27017 && dport != 6379) {
        return XDP_PASS; // Not a database port
    }

    // Apply more aggressive rate limiting for SYN packets
    __u32 src_ip = ip->saddr;
    __u64 current_time = bpf_ktime_get_ns();

    struct token_bucket *bucket = bpf_map_lookup_elem(&rate_limit_buckets, &src_ip);
    if (!bucket) {
        // Create bucket with smaller size for SYN limiting
        struct token_bucket new_bucket = {
            .tokens = 10 * 1000, // Smaller bucket for connections
            .last_refill = current_time,
            .bucket_size = 10,
            .refill_rate = 1, // 1 connection per second
            .packets_allowed = 0,
            .packets_dropped = 0
        };

        bpf_map_update_elem(&rate_limit_buckets, &src_ip, &new_bucket, BPF_ANY);
        return XDP_PASS; // Allow first connection attempt
    }

    // Refill the bucket
    refill_bucket(bucket, current_time, config);

    // Check for tokens (connections)
    if (bucket->tokens >= 1000) {
        bucket->tokens -= 1000;
        bucket->packets_allowed++;
        return XDP_PASS;
    } else {
        bucket->packets_dropped++;
        return XDP_DROP; // Rate limit SYN packet
    }
}

// XDP program for adaptive rate limiting based on backend load
SEC("xdp")
int xdp_adaptive_rate_limiter(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;

    // Get configuration
    __u32 config_key = 0;
    struct rate_limiter_config *config = bpf_map_lookup_elem(&rate_limiter_config, &config_key);
    if (!config || !config->enabled) {
        return XDP_PASS;
    }

    // Parse packet headers
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end) {
        return XDP_PASS;
    }

    if (eth->h_proto != bpf_htons(ETH_P_IP)) {
        return XDP_PASS;
    }

    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end) {
        return XDP_PASS;
    }

    __u32 src_ip = ip->saddr;
    __u64 current_time = bpf_ktime_get_ns();

    // Adaptive logic: adjust rate limits based on system load
    // This could be enhanced with backend health metrics

    struct token_bucket *bucket = bpf_map_lookup_elem(&rate_limit_buckets, &src_ip);
    if (!bucket) {
        // Create adaptive bucket
        __u32 adaptive_size = config->default_bucket_size;
        __u32 adaptive_rate = config->default_refill_rate;

        // Adjust based on emergency mode or system load
        if (config->emergency_mode) {
            adaptive_size /= 4;  // Reduce bucket size
            adaptive_rate /= 2;  // Reduce refill rate
        }

        struct token_bucket new_bucket = {
            .tokens = adaptive_size * 1000,
            .last_refill = current_time,
            .bucket_size = adaptive_size,
            .refill_rate = adaptive_rate,
            .packets_allowed = 0,
            .packets_dropped = 0
        };

        bpf_map_update_elem(&rate_limit_buckets, &src_ip, &new_bucket, BPF_ANY);
        return XDP_PASS;
    }

    // Dynamically adjust bucket parameters based on current conditions
    if (config->emergency_mode && bucket->bucket_size > 10) {
        bucket->bucket_size /= 2;
        bucket->refill_rate /= 2;
    }

    refill_bucket(bucket, current_time, config);

    if (bucket->tokens >= 1000) {
        bucket->tokens -= 1000;
        bucket->packets_allowed++;
        return XDP_PASS;
    } else {
        bucket->packets_dropped++;
        return XDP_DROP;
    }
}

char _license[] SEC("license") = "GPL";