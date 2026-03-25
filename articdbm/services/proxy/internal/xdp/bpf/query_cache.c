// ArticDBM XDP Query Cache
// High-performance kernel-level SQL query result caching

#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/in.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

// Cache configuration
#define MAX_CACHE_ENTRIES 65536
#define MAX_QUERY_SIZE 1024
#define MAX_RESULT_SIZE 4096
#define CACHE_TTL_SECONDS 300
#define NSEC_PER_SEC 1000000000ULL

// Hash seed for consistent hashing
#define HASH_SEED 0x12345678

// Cache entry structure
struct cache_entry {
    __u64 query_hash;       // Hash of normalized query
    __u64 param_hash;       // Hash of query parameters
    __u32 db_hash;          // Hash of database name
    __u32 table_hash;       // Hash of table name
    __u64 timestamp;        // Creation timestamp (nanoseconds)
    __u32 ttl_seconds;      // Time-to-live in seconds
    __u32 result_size;      // Size of cached result
    __u32 hit_count;        // Number of cache hits
    __u32 flags;            // Cache entry flags
    __u8  result_data[MAX_RESULT_SIZE]; // Cached result data
};

// Cache statistics structure
struct cache_stats {
    __u64 total_queries;
    __u64 cache_hits;
    __u64 cache_misses;
    __u64 cache_evictions;
    __u64 cache_invalidations;
    __u64 cache_size_bytes;
    __u64 avg_lookup_time_ns;
    __u32 current_entries;
    __u32 max_entries_reached;
};

// Query metadata for cache key generation
struct query_metadata {
    __u32 src_ip;
    __u16 src_port;
    __u16 dst_port;
    __u8  db_type;      // 1=MySQL, 2=PostgreSQL, 3=MSSQL, 4=MongoDB, 5=Redis
    __u8  operation;    // 1=SELECT, 2=INSERT, 3=UPDATE, 4=DELETE
    __u16 query_len;
    __u8  query_data[MAX_QUERY_SIZE];
};

// Cache hash table
struct {
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, MAX_CACHE_ENTRIES);
    __type(key, __u64);                 // Combined hash key
    __type(value, struct cache_entry);  // Cached result
    __uint(pinned, LIBBPF_PIN_BY_NAME);
} query_cache SEC(".maps");

// Cache statistics
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct cache_stats);
    __uint(pinned, LIBBPF_PIN_BY_NAME);
} cache_stats_map SEC(".maps");

// Cache configuration
struct cache_config {
    __u32 enabled;
    __u32 default_ttl;
    __u32 max_result_size;
    __u32 hash_seed;
    __u32 invalidate_on_write;
    __u32 numa_aware;
};

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct cache_config);
    __uint(pinned, LIBBPF_PIN_BY_NAME);
} cache_config_map SEC(".maps");

// Table invalidation tracking
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 4096);
    __type(key, __u32);        // Table hash
    __type(value, __u64);      // Last invalidation timestamp
    __uint(pinned, LIBBPF_PIN_BY_NAME);
} table_invalidation_map SEC(".maps");

// Helper function to update cache statistics
static __always_inline void update_cache_stats(__u32 stat_type) {
    __u32 key = 0;
    struct cache_stats *stats = bpf_map_lookup_elem(&cache_stats_map, &key);
    if (!stats) return;

    switch (stat_type) {
        case 0: // total_queries
            __sync_fetch_and_add(&stats->total_queries, 1);
            break;
        case 1: // cache_hits
            __sync_fetch_and_add(&stats->cache_hits, 1);
            break;
        case 2: // cache_misses
            __sync_fetch_and_add(&stats->cache_misses, 1);
            break;
        case 3: // cache_evictions
            __sync_fetch_and_add(&stats->cache_evictions, 1);
            break;
        case 4: // cache_invalidations
            __sync_fetch_and_add(&stats->cache_invalidations, 1);
            break;
    }
}

// Simple hash function for consistency
static __always_inline __u32 simple_hash(const void *data, __u32 len, __u32 seed) {
    const __u8 *bytes = (const __u8 *)data;
    __u32 hash = seed;

    #pragma unroll
    for (__u32 i = 0; i < 32 && i < len; i++) {
        hash = hash * 31 + bytes[i];
    }

    return hash;
}

// Generate cache key from query metadata
static __always_inline __u64 generate_cache_key(struct query_metadata *meta, __u32 seed) {
    // Hash the query text
    __u32 query_hash = simple_hash(meta->query_data, meta->query_len, seed);

    // Combine with database and operation info
    __u32 context_hash = simple_hash(&meta->db_type, sizeof(__u8), seed) ^
                         simple_hash(&meta->operation, sizeof(__u8), seed + 1) ^
                         simple_hash(&meta->dst_port, sizeof(__u16), seed + 2);

    // Create 64-bit key
    return ((__u64)query_hash << 32) | context_hash;
}

// Check if cache entry is valid
static __always_inline int is_cache_entry_valid(struct cache_entry *entry, __u64 current_time) {
    if (!entry) return 0;

    // Check TTL
    __u64 age_ns = current_time - entry->timestamp;
    __u64 ttl_ns = (__u64)entry->ttl_seconds * NSEC_PER_SEC;

    return age_ns < ttl_ns;
}

// Extract query metadata from packet
static __always_inline int extract_query_metadata(void *data, void *data_end,
                                                  struct query_metadata *meta) {
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end) return -1;

    if (eth->h_proto != bpf_htons(ETH_P_IP)) return -1;

    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end) return -1;

    meta->src_ip = ip->saddr;

    if (ip->protocol != IPPROTO_TCP) return -1;

    struct tcphdr *tcp = (void *)ip + (ip->ihl * 4);
    if ((void *)(tcp + 1) > data_end) return -1;

    meta->src_port = bpf_ntohs(tcp->source);
    meta->dst_port = bpf_ntohs(tcp->dest);

    // Determine database type by port
    switch (meta->dst_port) {
        case 3306:
            meta->db_type = 1; // MySQL
            break;
        case 5432:
            meta->db_type = 2; // PostgreSQL
            break;
        case 1433:
            meta->db_type = 3; // MSSQL
            break;
        case 27017:
            meta->db_type = 4; // MongoDB
            break;
        case 6379:
            meta->db_type = 5; // Redis
            break;
        default:
            return -1; // Not a database port
    }

    // Extract payload (simplified - real implementation would parse protocol)
    void *payload = (void *)tcp + (tcp->doff * 4);
    if (payload >= data_end) return -1;

    __u32 payload_len = data_end - payload;
    if (payload_len > MAX_QUERY_SIZE) {
        payload_len = MAX_QUERY_SIZE;
    }

    meta->query_len = payload_len;

    // Copy query data (bounded loop for verifier)
    #pragma unroll
    for (__u32 i = 0; i < 64 && i < payload_len && (payload + i) < data_end; i++) {
        meta->query_data[i] = *((__u8 *)payload + i);
    }

    // Simplified operation detection (look for SQL keywords)
    meta->operation = 1; // Default to SELECT
    if (payload_len >= 6) {
        // Check for INSERT
        if (meta->query_data[0] == 'I' && meta->query_data[1] == 'N' &&
            meta->query_data[2] == 'S' && meta->query_data[3] == 'E') {
            meta->operation = 2;
        }
        // Check for UPDATE
        else if (meta->query_data[0] == 'U' && meta->query_data[1] == 'P' &&
                 meta->query_data[2] == 'D' && meta->query_data[3] == 'A') {
            meta->operation = 3;
        }
        // Check for DELETE
        else if (meta->query_data[0] == 'D' && meta->query_data[1] == 'E' &&
                 meta->query_data[2] == 'L' && meta->query_data[3] == 'E') {
            meta->operation = 4;
        }
    }

    return 0;
}

// Main XDP cache lookup program
SEC("xdp")
int xdp_cache_lookup(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;

    // Get cache configuration
    __u32 config_key = 0;
    struct cache_config *config = bpf_map_lookup_elem(&cache_config_map, &config_key);
    if (!config || !config->enabled) {
        return XDP_PASS; // Cache disabled
    }

    update_cache_stats(0); // total_queries

    // Extract query metadata from packet
    struct query_metadata meta = {0};
    if (extract_query_metadata(data, data_end, &meta) != 0) {
        return XDP_PASS; // Not a database query or parse failed
    }

    // Skip caching for write operations if configured
    if (config->invalidate_on_write && meta.operation > 1) {
        return XDP_PASS; // Let write operations through
    }

    // Generate cache key
    __u64 cache_key = generate_cache_key(&meta, config->hash_seed);
    __u64 current_time = bpf_ktime_get_ns();

    // Look up cached result
    struct cache_entry *cached = bpf_map_lookup_elem(&query_cache, &cache_key);
    if (cached && is_cache_entry_valid(cached, current_time)) {
        // Cache hit - increment hit count
        __sync_fetch_and_add(&cached->hit_count, 1);
        update_cache_stats(1); // cache_hits

        // TODO: In a complete implementation, we would:
        // 1. Construct response packet with cached data
        // 2. Update packet headers
        // 3. Return XDP_TX to send response immediately

        // For now, mark as cache hit in metadata and pass through
        return XDP_PASS;
    }

    // Cache miss
    update_cache_stats(2); // cache_misses
    return XDP_PASS;
}

// XDP program for cache invalidation on write operations
SEC("xdp")
int xdp_cache_invalidator(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;

    struct query_metadata meta = {0};
    if (extract_query_metadata(data, data_end, &meta) != 0) {
        return XDP_PASS;
    }

    // Only process write operations
    if (meta.operation <= 1) { // SELECT operations
        return XDP_PASS;
    }

    __u64 current_time = bpf_ktime_get_ns();

    // Invalidate cache entries for affected tables
    // This is a simplified version - real implementation would:
    // 1. Parse SQL to extract affected table names
    // 2. Hash table names
    // 3. Invalidate all cache entries for those tables

    // For demonstration, invalidate based on simple heuristics
    __u32 table_hash = simple_hash(meta.query_data, meta.query_len, HASH_SEED);

    // Record invalidation timestamp
    bpf_map_update_elem(&table_invalidation_map, &table_hash, &current_time, BPF_ANY);

    update_cache_stats(4); // cache_invalidations

    return XDP_PASS;
}

// XDP program for cache warming (preloading frequently accessed queries)
SEC("xdp")
int xdp_cache_warmer(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;

    struct cache_config *config = bpf_map_lookup_elem(&cache_config_map, 0);
    if (!config || !config->enabled) {
        return XDP_PASS;
    }

    struct query_metadata meta = {0};
    if (extract_query_metadata(data, data_end, &meta) != 0) {
        return XDP_PASS;
    }

    // Only warm cache for SELECT operations
    if (meta.operation != 1) {
        return XDP_PASS;
    }

    __u64 cache_key = generate_cache_key(&meta, config->hash_seed);
    __u64 current_time = bpf_ktime_get_ns();

    // Check if we should warm this query (based on frequency heuristics)
    struct cache_entry *existing = bpf_map_lookup_elem(&query_cache, &cache_key);
    if (!existing) {
        // Create placeholder entry for warming
        struct cache_entry new_entry = {
            .query_hash = cache_key >> 32,
            .param_hash = cache_key & 0xFFFFFFFF,
            .timestamp = current_time,
            .ttl_seconds = config->default_ttl,
            .result_size = 0, // Will be filled by userspace
            .hit_count = 0,
            .flags = 1, // Mark as warming entry
        };

        bpf_map_update_elem(&query_cache, &cache_key, &new_entry, BPF_NOEXIST);
    }

    return XDP_PASS;
}

// XDP program for NUMA-aware cache placement
SEC("xdp")
int xdp_numa_cache_manager(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;

    struct cache_config *config = bpf_map_lookup_elem(&cache_config_map, 0);
    if (!config || !config->enabled || !config->numa_aware) {
        return XDP_PASS;
    }

    // NUMA-aware cache management logic
    // This would integrate with the NUMA topology information
    // to place cache entries optimally based on:
    // 1. Source IP NUMA affinity
    // 2. Worker thread NUMA placement
    // 3. Memory locality considerations

    return XDP_PASS;
}

char _license[] SEC("license") = "GPL";