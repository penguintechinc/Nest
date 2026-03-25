// ArticDBM XDP Traffic Splitter for Blue/Green Deployments
// High-performance kernel-level traffic distribution and health-aware routing

#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/ipv6.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/in.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

// Traffic splitting configuration
#define MAX_ENVIRONMENTS 16
#define MAX_BACKENDS_PER_ENV 32
#define MAX_HEALTH_CHECKS 256

// Environment types
#define ENV_TYPE_BLUE     1
#define ENV_TYPE_GREEN    2
#define ENV_TYPE_CANARY   3
#define ENV_TYPE_STAGING  4

// Health states
#define HEALTH_UNKNOWN    0
#define HEALTH_HEALTHY    1
#define HEALTH_DEGRADED   2
#define HEALTH_UNHEALTHY  3

// Traffic splitting strategies
#define STRATEGY_PERCENTAGE    1
#define STRATEGY_USER_BASED    2
#define STRATEGY_CANARY        3
#define STRATEGY_AB_TEST       4
#define STRATEGY_GEOLOCATION   5

// Backend environment configuration
struct environment_config {
    __u32 env_id;
    __u32 env_type;                 // Blue, Green, Canary, etc.
    __u32 weight_percentage;        // 0-100, percentage of traffic
    __u32 strategy;                 // Traffic splitting strategy
    __u32 enabled;                  // Environment enabled/disabled
    __u32 health_check_enabled;     // Health checking enabled
    __u32 failover_enabled;         // Automatic failover enabled
    __u32 sticky_sessions;          // Session affinity enabled
    __u32 backend_count;            // Number of backends in this environment
    __u64 last_health_check;        // Last health check timestamp
    __u64 created_at;               // Environment creation time
    __u32 flags;                    // Additional configuration flags
};

// Individual backend configuration
struct backend_config {
    __u32 backend_id;
    __u32 env_id;                   // Which environment this belongs to
    __u32 ip_addr;                  // Backend IP address
    __u16 port;                     // Backend port
    __u16 weight;                   // Load balancing weight
    __u32 health_state;             // Current health state
    __u64 last_health_check;        // Last health check timestamp
    __u64 response_time_avg;        // Average response time (microseconds)
    __u32 connection_count;         // Active connection count
    __u32 error_count;              // Recent error count
    __u32 success_count;            // Recent success count
    __u32 numa_node;                // NUMA node affinity
    __u32 flags;                    // Backend-specific flags
};

// Health check configuration
struct health_check_config {
    __u32 check_id;
    __u32 backend_id;
    __u32 check_type;               // TCP, HTTP, custom
    __u32 interval_ms;              // Check interval in milliseconds
    __u32 timeout_ms;               // Check timeout
    __u32 failure_threshold;        // Failures before marking unhealthy
    __u32 success_threshold;        // Successes before marking healthy
    __u32 current_failures;         // Current consecutive failures
    __u32 current_successes;        // Current consecutive successes
    __u64 last_check_time;          // Last check timestamp
    __u32 enabled;                  // Health check enabled
};

// Session affinity tracking
struct session_info {
    __u32 client_ip;
    __u16 client_port;
    __u32 backend_id;
    __u64 created_at;
    __u64 last_access;
    __u32 request_count;
    __u32 flags;
};

// Traffic splitting statistics
struct traffic_stats {
    __u64 total_requests;
    __u64 blue_requests;
    __u64 green_requests;
    __u64 canary_requests;
    __u64 failed_requests;
    __u64 health_check_failures;
    __u64 automatic_failovers;
    __u64 session_hits;
    __u64 session_misses;
    __u32 active_sessions;
};

// Environment configuration map
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, MAX_ENVIRONMENTS);
    __type(key, __u32);                         // Environment ID
    __type(value, struct environment_config);   // Environment config
    __uint(pinned, LIBBPF_PIN_BY_NAME);
} environment_configs SEC(".maps");

// Backend configuration map
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_ENVIRONMENTS * MAX_BACKENDS_PER_ENV);
    __type(key, __u32);                         // Backend ID
    __type(value, struct backend_config);       // Backend config
    __uint(pinned, LIBBPF_PIN_BY_NAME);
} backend_configs SEC(".maps");

// Health check configuration
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_HEALTH_CHECKS);
    __type(key, __u32);                         // Health check ID
    __type(value, struct health_check_config);  // Health check config
    __uint(pinned, LIBBPF_PIN_BY_NAME);
} health_check_configs SEC(".maps");

// Session affinity tracking
struct {
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, 65536);
    __type(key, __u64);                         // Session key (IP + Port hash)
    __type(value, struct session_info);         // Session information
    __uint(pinned, LIBBPF_PIN_BY_NAME);
} session_map SEC(".maps");

// Traffic splitting statistics
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct traffic_stats);
    __uint(pinned, LIBBPF_PIN_BY_NAME);
} traffic_stats_map SEC(".maps");

// Current deployment state
struct deployment_state {
    __u32 active_strategy;          // Current traffic splitting strategy
    __u32 primary_env_id;           // Primary environment (blue/green)
    __u32 secondary_env_id;         // Secondary environment
    __u32 canary_env_id;            // Canary environment (if active)
    __u32 traffic_split_ratio;      // Primary:Secondary ratio (0-100)
    __u32 canary_ratio;             // Canary traffic ratio (0-100)
    __u32 failover_mode;            // Automatic failover enabled
    __u32 emergency_mode;           // Emergency failover mode
    __u64 deployment_start_time;    // When current deployment started
    __u64 last_update_time;         // Last configuration update
    __u32 flags;                    // Deployment flags
};

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct deployment_state);
    __uint(pinned, LIBBPF_PIN_BY_NAME);
} deployment_state_map SEC(".maps");

// Helper function to update statistics
static __always_inline void update_traffic_stats(__u32 stat_type) {
    __u32 key = 0;
    struct traffic_stats *stats = bpf_map_lookup_elem(&traffic_stats_map, &key);
    if (!stats) return;

    switch (stat_type) {
        case 0: // total_requests
            __sync_fetch_and_add(&stats->total_requests, 1);
            break;
        case 1: // blue_requests
            __sync_fetch_and_add(&stats->blue_requests, 1);
            break;
        case 2: // green_requests
            __sync_fetch_and_add(&stats->green_requests, 1);
            break;
        case 3: // canary_requests
            __sync_fetch_and_add(&stats->canary_requests, 1);
            break;
        case 4: // failed_requests
            __sync_fetch_and_add(&stats->failed_requests, 1);
            break;
        case 5: // health_check_failures
            __sync_fetch_and_add(&stats->health_check_failures, 1);
            break;
        case 6: // automatic_failovers
            __sync_fetch_and_add(&stats->automatic_failovers, 1);
            break;
        case 7: // session_hits
            __sync_fetch_and_add(&stats->session_hits, 1);
            break;
        case 8: // session_misses
            __sync_fetch_and_add(&stats->session_misses, 1);
            break;
    }
}

// Generate session key from client IP and port
static __always_inline __u64 generate_session_key(__u32 client_ip, __u16 client_port) {
    return ((__u64)client_ip << 16) | client_port;
}

// Simple hash function for consistent backend selection
static __always_inline __u32 hash_consistent(__u32 input, __u32 seed) {
    __u32 hash = seed;
    hash ^= input;
    hash *= 0x85ebca6b;
    hash ^= hash >> 13;
    hash *= 0xc2b2ae35;
    hash ^= hash >> 16;
    return hash;
}

// Check if backend is healthy
static __always_inline int is_backend_healthy(__u32 backend_id) {
    struct backend_config *backend = bpf_map_lookup_elem(&backend_configs, &backend_id);
    if (!backend) return 0;

    return backend->health_state == HEALTH_HEALTHY;
}

// Select backend based on environment and strategy
static __always_inline __u32 select_backend_from_environment(__u32 env_id, __u32 client_ip, __u16 client_port) {
    struct environment_config *env = bpf_map_lookup_elem(&environment_configs, &env_id);
    if (!env || !env->enabled) return 0;

    // For simple load balancing, hash client info to select backend
    __u32 hash = hash_consistent(client_ip, client_port);
    __u32 backend_index = hash % env->backend_count;

    // Find the backend_index-th backend in this environment
    // This is simplified - real implementation would iterate through backends
    __u32 backend_id = env_id * MAX_BACKENDS_PER_ENV + backend_index;

    if (is_backend_healthy(backend_id)) {
        return backend_id;
    }

    // If primary backend is unhealthy, try others
    for (__u32 i = 0; i < env->backend_count && i < 8; i++) {
        __u32 alt_backend_id = env_id * MAX_BACKENDS_PER_ENV + i;
        if (is_backend_healthy(alt_backend_id)) {
            return alt_backend_id;
        }
    }

    return 0; // No healthy backends found
}

// Implement percentage-based traffic splitting
static __always_inline __u32 select_environment_percentage(struct deployment_state *state, __u32 client_ip) {
    __u32 hash = hash_consistent(client_ip, state->deployment_start_time);
    __u32 percentage = hash % 100;

    // Check canary first if active
    if (state->canary_env_id > 0 && percentage < state->canary_ratio) {
        return state->canary_env_id;
    }

    // Primary vs Secondary split
    if (percentage < state->traffic_split_ratio) {
        return state->primary_env_id;
    } else {
        return state->secondary_env_id;
    }
}

// Implement user-based traffic splitting (A/B testing)
static __always_inline __u32 select_environment_user_based(struct deployment_state *state, __u32 client_ip) {
    // Use IP as user identifier for consistent routing
    __u32 user_hash = hash_consistent(client_ip, 0x12345678);

    // Split users deterministically between environments
    if (user_hash % 2 == 0) {
        return state->primary_env_id;
    } else {
        return state->secondary_env_id;
    }
}

// Handle session affinity
static __always_inline __u32 check_session_affinity(__u32 client_ip, __u16 client_port, __u64 current_time) {
    __u64 session_key = generate_session_key(client_ip, client_port);
    struct session_info *session = bpf_map_lookup_elem(&session_map, &session_key);

    if (session) {
        // Update last access time
        session->last_access = current_time;
        __sync_fetch_and_add(&session->request_count, 1);
        update_traffic_stats(7); // session_hits
        return session->backend_id;
    }

    update_traffic_stats(8); // session_misses
    return 0; // No session found
}

// Create new session affinity
static __always_inline void create_session_affinity(__u32 client_ip, __u16 client_port, __u32 backend_id, __u64 current_time) {
    __u64 session_key = generate_session_key(client_ip, client_port);

    struct session_info new_session = {
        .client_ip = client_ip,
        .client_port = client_port,
        .backend_id = backend_id,
        .created_at = current_time,
        .last_access = current_time,
        .request_count = 1,
        .flags = 0,
    };

    bpf_map_update_elem(&session_map, &session_key, &new_session, BPF_ANY);
}

// Main XDP traffic splitter program
SEC("xdp")
int xdp_traffic_splitter(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;

    update_traffic_stats(0); // total_requests

    // Parse packet headers
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end) {
        return XDP_PASS;
    }

    if (eth->h_proto != bpf_htons(ETH_P_IP)) {
        return XDP_PASS; // Only handle IPv4 for now
    }

    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end) {
        return XDP_PASS;
    }

    __u32 client_ip = ip->saddr;
    __u16 client_port = 0;
    __u16 dest_port = 0;

    // Extract port information for TCP/UDP
    if (ip->protocol == IPPROTO_TCP) {
        struct tcphdr *tcp = (void *)ip + (ip->ihl * 4);
        if ((void *)(tcp + 1) > data_end) {
            return XDP_PASS;
        }
        client_port = bpf_ntohs(tcp->source);
        dest_port = bpf_ntohs(tcp->dest);
    } else if (ip->protocol == IPPROTO_UDP) {
        struct udphdr *udp = (void *)ip + (ip->ihl * 4);
        if ((void *)(udp + 1) > data_end) {
            return XDP_PASS;
        }
        client_port = bpf_ntohs(udp->source);
        dest_port = bpf_ntohs(udp->dest);
    }

    // Only process database traffic (check destination ports)
    if (dest_port != 3306 && dest_port != 5432 && dest_port != 1433 &&
        dest_port != 27017 && dest_port != 6379) {
        return XDP_PASS; // Not database traffic
    }

    // Get current deployment state
    __u32 state_key = 0;
    struct deployment_state *state = bpf_map_lookup_elem(&deployment_state_map, &state_key);
    if (!state || state->primary_env_id == 0) {
        return XDP_PASS; // No deployment configuration
    }

    __u64 current_time = bpf_ktime_get_ns();
    __u32 selected_backend_id = 0;

    // Check session affinity first if enabled
    struct environment_config *primary_env = bpf_map_lookup_elem(&environment_configs, &state->primary_env_id);
    if (primary_env && primary_env->sticky_sessions) {
        selected_backend_id = check_session_affinity(client_ip, client_port, current_time);
        if (selected_backend_id > 0 && is_backend_healthy(selected_backend_id)) {
            return XDP_PASS; // Use existing session
        }
    }

    // Select environment based on strategy
    __u32 selected_env_id = 0;
    switch (state->active_strategy) {
        case STRATEGY_PERCENTAGE:
            selected_env_id = select_environment_percentage(state, client_ip);
            break;
        case STRATEGY_USER_BASED:
        case STRATEGY_AB_TEST:
            selected_env_id = select_environment_user_based(state, client_ip);
            break;
        case STRATEGY_CANARY:
            // Canary strategy: small percentage to canary, rest to primary
            if (state->canary_env_id > 0) {
                __u32 hash = hash_consistent(client_ip, current_time);
                if ((hash % 100) < state->canary_ratio) {
                    selected_env_id = state->canary_env_id;
                    update_traffic_stats(3); // canary_requests
                } else {
                    selected_env_id = state->primary_env_id;
                    update_traffic_stats(1); // blue_requests (assuming primary is blue)
                }
            } else {
                selected_env_id = state->primary_env_id;
            }
            break;
        default:
            selected_env_id = state->primary_env_id;
            break;
    }

    // Select backend from chosen environment
    selected_backend_id = select_backend_from_environment(selected_env_id, client_ip, client_port);

    if (selected_backend_id == 0) {
        // No healthy backend found, try failover
        if (state->failover_mode && selected_env_id == state->primary_env_id) {
            selected_backend_id = select_backend_from_environment(state->secondary_env_id, client_ip, client_port);
            if (selected_backend_id > 0) {
                update_traffic_stats(6); // automatic_failovers
            }
        }

        if (selected_backend_id == 0) {
            update_traffic_stats(4); // failed_requests
            return XDP_DROP; // No healthy backends available
        }
    }

    // Create session affinity if enabled
    if (primary_env && primary_env->sticky_sessions && client_port > 0) {
        create_session_affinity(client_ip, client_port, selected_backend_id, current_time);
    }

    // Update environment-specific statistics
    if (selected_env_id == state->primary_env_id) {
        update_traffic_stats(1); // blue_requests (assuming primary is blue)
    } else if (selected_env_id == state->secondary_env_id) {
        update_traffic_stats(2); // green_requests (assuming secondary is green)
    }

    // In a complete implementation, we would:
    // 1. Rewrite packet destination to selected backend
    // 2. Update connection tracking
    // 3. Handle response routing back to client
    // 4. For now, just pass through with metadata

    return XDP_PASS;
}

// XDP program for health monitoring
SEC("xdp")
int xdp_health_monitor(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;

    // Parse health check responses
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

    // This would implement health check response processing
    // For now, just monitor traffic patterns

    return XDP_PASS;
}

// XDP program for emergency failover
SEC("xdp")
int xdp_emergency_failover(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;

    __u32 state_key = 0;
    struct deployment_state *state = bpf_map_lookup_elem(&deployment_state_map, &state_key);
    if (!state || !state->emergency_mode) {
        return XDP_PASS; // Emergency mode not active
    }

    // In emergency mode, route all traffic to secondary environment
    // This would be triggered by external monitoring systems

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

    // Emergency routing logic would go here
    update_traffic_stats(6); // automatic_failovers

    return XDP_PASS;
}

char _license[] SEC("license") = "GPL";