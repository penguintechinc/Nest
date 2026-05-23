package security

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/penguintechinc/articdbm/proxy/internal/xdp"
	"github.com/go-redis/redis/v8"
)

// ThreatIntelligenceEngine manages automated threat intelligence feeds
type ThreatIntelligenceEngine struct {
	config      *ThreatIntelConfig
	redisClient *redis.Client
	xdpController *xdp.Controller
	logger      *zap.Logger

	// Feed management
	activeFeedsmu sync.RWMutex
	activeFeeds map[string]*ThreatFeed
	feedStats   map[string]*FeedStats

	// Threat data storage
	threatData *ThreatDatabase
	indicators map[string]*ThreatIndicator

	// Processing
	ctx         context.Context
	cancel      context.CancelFunc
	updateQueue chan *ThreatUpdate
	httpClient  *http.Client
}

type ThreatIntelConfig struct {
	Enabled           bool                    `json:"enabled"`
	UpdateInterval    time.Duration           `json:"update_interval"`
	MaxConcurrentFeeds int                    `json:"max_concurrent_feeds"`
	Feeds             map[string]*FeedConfig  `json:"feeds"`
	AutoBlock         AutoBlockConfig         `json:"auto_block"`
	Storage           StorageConfig           `json:"storage"`
	Processing        ProcessingConfig        `json:"processing"`
	Notifications     NotificationConfig      `json:"notifications"`
}

type FeedConfig struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"` // "stix", "taxii", "openIOC", "misp", "csv", "json"
	URL         string            `json:"url"`
	APIKey      string            `json:"api_key"`
	Username    string            `json:"username"`
	Password    string            `json:"password"`
	Headers     map[string]string `json:"headers"`
	Enabled     bool              `json:"enabled"`
	Priority    int               `json:"priority"`
	UpdateFreq  time.Duration     `json:"update_frequency"`
	Timeout     time.Duration     `json:"timeout"`
	RetryCount  int               `json:"retry_count"`
	TLSVerify   bool              `json:"tls_verify"`
	Categories  []string          `json:"categories"` // Filter by threat categories
	Confidence  float64           `json:"min_confidence"`
	Format      FeedFormat        `json:"format"`
}

type FeedFormat struct {
	Parser      string                 `json:"parser"`
	Fields      map[string]string      `json:"fields"` // Field mapping
	Transforms  []FieldTransform       `json:"transforms"`
	Filters     []FeedFilter           `json:"filters"`
	Validation  ValidationRules        `json:"validation"`
}

type FieldTransform struct {
	Field     string `json:"field"`
	Operation string `json:"operation"` // "lowercase", "uppercase", "trim", "extract_ip"
	Pattern   string `json:"pattern,omitempty"`
}

type FeedFilter struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"` // "equals", "contains", "matches", "gt", "lt"
	Value    interface{} `json:"value"`
}

type ValidationRules struct {
	RequiredFields []string          `json:"required_fields"`
	IPValidation   bool              `json:"ip_validation"`
	DomainValidation bool            `json:"domain_validation"`
	CustomRules    []ValidationRule  `json:"custom_rules"`
}

type ValidationRule struct {
	Field   string `json:"field"`
	Regex   string `json:"regex"`
	Message string `json:"message"`
}

type AutoBlockConfig struct {
	Enabled             bool              `json:"enabled"`
	HighConfidenceBlock bool              `json:"high_confidence_block"`
	ConfidenceThreshold float64           `json:"confidence_threshold"`
	Categories          []string          `json:"categories"`
	ExcludeFeeds        []string          `json:"exclude_feeds"`
	BlockDuration       time.Duration     `json:"block_duration"`
	MaxAutoBlocks       int               `json:"max_auto_blocks_per_hour"`
	NotificationWebhook string            `json:"notification_webhook"`
	ApprovalRequired    bool              `json:"approval_required"`
}

type StorageConfig struct {
	RetentionDays     int  `json:"retention_days"`
	CompressOldData   bool `json:"compress_old_data"`
	MaxMemoryMB       int  `json:"max_memory_mb"`
	BackupEnabled     bool `json:"backup_enabled"`
	BackupInterval    time.Duration `json:"backup_interval"`
	BackupLocation    string `json:"backup_location"`
}

type ProcessingConfig struct {
	Workers           int           `json:"workers"`
	BatchSize         int           `json:"batch_size"`
	DeduplicationEnabled bool       `json:"deduplication_enabled"`
	EnrichmentEnabled bool          `json:"enrichment_enabled"`
	GeolocationLookup bool          `json:"geolocation_lookup"`
	DNSResolution     bool          `json:"dns_resolution"`
	ProcessingTimeout time.Duration `json:"processing_timeout"`
}

type NotificationConfig struct {
	Enabled     bool     `json:"enabled"`
	Webhooks    []string `json:"webhooks"`
	EmailAddresses []string `json:"email_addresses"`
	SlackWebhook string  `json:"slack_webhook"`
	Teams       string   `json:"teams_webhook"`
	OnNewThreats bool    `json:"on_new_threats"`
	OnBlocks    bool     `json:"on_blocks"`
	OnErrors    bool     `json:"on_errors"`
}

// Threat data structures
type ThreatFeed struct {
	ID           string            `json:"id"`
	Config       *FeedConfig       `json:"config"`
	Status       string            `json:"status"` // "active", "inactive", "error"
	LastUpdate   time.Time         `json:"last_update"`
	LastError    string            `json:"last_error"`
	RecordCount  int               `json:"record_count"`
	Hash         string            `json:"hash"` // Content hash for change detection
	Metadata     map[string]interface{} `json:"metadata"`
}

type FeedStats struct {
	TotalUpdates      int           `json:"total_updates"`
	SuccessfulUpdates int           `json:"successful_updates"`
	FailedUpdates     int           `json:"failed_updates"`
	LastUpdateTime    time.Time     `json:"last_update_time"`
	AverageUpdateTime time.Duration `json:"average_update_time"`
	TotalIndicators   int           `json:"total_indicators"`
	NewIndicators     int           `json:"new_indicators_last_update"`
	ErrorRate         float64       `json:"error_rate"`
}

type ThreatDatabase struct {
	IPAddresses    map[string]*IPThreat      `json:"ip_addresses"`
	Domains        map[string]*DomainThreat  `json:"domains"`
	URLs           map[string]*URLThreat     `json:"urls"`
	FileHashes     map[string]*FileThreat    `json:"file_hashes"`
	CVEs           map[string]*CVEThreat     `json:"cves"`
	Signatures     map[string]*SignatureThreat `json:"signatures"`
	LastUpdate     time.Time                 `json:"last_update"`
	TotalIndicators int                      `json:"total_indicators"`
}

type ThreatIndicator struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"` // "ip", "domain", "url", "hash", "cve", "signature"
	Value        string                 `json:"value"`
	Confidence   float64                `json:"confidence"`
	Severity     string                 `json:"severity"` // "low", "medium", "high", "critical"
	Categories   []string               `json:"categories"`
	Sources      []string               `json:"sources"`
	FirstSeen    time.Time              `json:"first_seen"`
	LastSeen     time.Time              `json:"last_seen"`
	ExpiresAt    *time.Time             `json:"expires_at,omitempty"`
	Blocked      bool                   `json:"blocked"`
	BlockedAt    *time.Time             `json:"blocked_at,omitempty"`
	TLP          string                 `json:"tlp"` // Traffic Light Protocol
	Tags         []string               `json:"tags"`
	Metadata     map[string]interface{} `json:"metadata"`
	Geolocation  *GeoLocation           `json:"geolocation,omitempty"`
	DNSInfo      *DNSInfo               `json:"dns_info,omitempty"`
}

type IPThreat struct {
	*ThreatIndicator
	IPAddress     string    `json:"ip_address"`
	Network       string    `json:"network,omitempty"` // CIDR if applicable
	ASN           int       `json:"asn,omitempty"`
	Organization  string    `json:"organization,omitempty"`
	Country       string    `json:"country,omitempty"`
	IsTor         bool      `json:"is_tor"`
	IsMalware     bool      `json:"is_malware"`
	IsBotnet      bool      `json:"is_botnet"`
}

type DomainThreat struct {
	*ThreatIndicator
	Domain        string    `json:"domain"`
	Subdomain     string    `json:"subdomain,omitempty"`
	TLD           string    `json:"tld"`
	IsPhishing    bool      `json:"is_phishing"`
	IsMalware     bool      `json:"is_malware"`
	IsSuspicious  bool      `json:"is_suspicious"`
	RegisteredAt  *time.Time `json:"registered_at,omitempty"`
}

type URLThreat struct {
	*ThreatIndicator
	URL           string    `json:"url"`
	Domain        string    `json:"domain"`
	Path          string    `json:"path"`
	IsPhishing    bool      `json:"is_phishing"`
	IsMalware     bool      `json:"is_malware"`
	IsSuspicious  bool      `json:"is_suspicious"`
}

type FileThreat struct {
	*ThreatIndicator
	Hash          string    `json:"hash"`
	HashType      string    `json:"hash_type"` // "md5", "sha1", "sha256"
	FileName      string    `json:"file_name,omitempty"`
	FileSize      int64     `json:"file_size,omitempty"`
	FileType      string    `json:"file_type,omitempty"`
	IsMalware     bool      `json:"is_malware"`
	Family        string    `json:"family,omitempty"`
}

type CVEThreat struct {
	*ThreatIndicator
	CVE           string    `json:"cve"`
	CVSSScore     float64   `json:"cvss_score"`
	CVSSVector    string    `json:"cvss_vector"`
	Description   string    `json:"description"`
	References    []string  `json:"references"`
	AffectedSystems []string `json:"affected_systems"`
}

type SignatureThreat struct {
	*ThreatIndicator
	Name          string    `json:"name"`
	Pattern       string    `json:"pattern"`
	Protocol      string    `json:"protocol,omitempty"`
	Port          int       `json:"port,omitempty"`
	Direction     string    `json:"direction,omitempty"` // "inbound", "outbound", "both"
}

type GeoLocation struct {
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	Region      string  `json:"region"`
	City        string  `json:"city"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Timezone    string  `json:"timezone"`
}

type DNSInfo struct {
	Domain    string   `json:"domain"`
	Records   []string `json:"records"`
	TTL       int      `json:"ttl"`
	Nameservers []string `json:"nameservers"`
}

type ThreatUpdate struct {
	FeedID      string              `json:"feed_id"`
	Indicators  []*ThreatIndicator  `json:"indicators"`
	Timestamp   time.Time           `json:"timestamp"`
	UpdateType  string              `json:"update_type"` // "full", "incremental", "delete"
}

func NewThreatIntelligenceEngine(config *ThreatIntelConfig, redisClient *redis.Client, xdpController *xdp.Controller, logger *zap.Logger) *ThreatIntelligenceEngine {
	ctx, cancel := context.WithCancel(context.Background())

	engine := &ThreatIntelligenceEngine{
		config:        config,
		redisClient:   redisClient,
		xdpController: xdpController,
		logger:        logger,
		activeFeeds:   make(map[string]*ThreatFeed),
		feedStats:     make(map[string]*FeedStats),
		indicators:    make(map[string]*ThreatIndicator),
		ctx:           ctx,
		cancel:        cancel,
		updateQueue:   make(chan *ThreatUpdate, 1000),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		threatData: &ThreatDatabase{
			IPAddresses:    make(map[string]*IPThreat),
			Domains:        make(map[string]*DomainThreat),
			URLs:           make(map[string]*URLThreat),
			FileHashes:     make(map[string]*FileThreat),
			CVEs:           make(map[string]*CVEThreat),
			Signatures:     make(map[string]*SignatureThreat),
		},
	}

	// Initialize feeds
	for feedID, feedConfig := range config.Feeds {
		if feedConfig.Enabled {
			feed := &ThreatFeed{
				ID:     feedID,
				Config: feedConfig,
				Status: "inactive",
			}
			engine.activeFeeds[feedID] = feed
			engine.feedStats[feedID] = &FeedStats{}
		}
	}

	// Start background processes
	go engine.feedUpdateLoop()
	go engine.processingWorkers()
	go engine.cleanupExpiredThreats()
	go engine.backupLoop()

	// Load existing threat data from Redis
	engine.loadThreatData()

	logger.Info("Threat Intelligence Engine initialized",
		zap.Int("feeds", len(engine.activeFeeds)),
		zap.Bool("auto_block", config.AutoBlock.Enabled))

	return engine
}

func (tie *ThreatIntelligenceEngine) feedUpdateLoop() {
	ticker := time.NewTicker(tie.config.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-tie.ctx.Done():
			return
		case <-ticker.C:
			tie.updateAllFeeds()
		}
	}
}

func (tie *ThreatIntelligenceEngine) updateAllFeeds() {
	tie.activeFeedsmu.RLock()
	feeds := make([]*ThreatFeed, 0, len(tie.activeFeeds))
	for _, feed := range tie.activeFeeds {
		if feed.Config.Enabled {
			feeds = append(feeds, feed)
		}
	}
	tie.activeFeedsmu.RUnlock()

	// Limit concurrent feed updates
	semaphore := make(chan struct{}, tie.config.MaxConcurrentFeeds)

	for _, feed := range feeds {
		semaphore <- struct{}{}
		go func(f *ThreatFeed) {
			defer func() { <-semaphore }()

			tie.logger.Debug("Updating threat feed", zap.String("feed", f.ID))
			start := time.Now()

			if err := tie.updateFeed(f); err != nil {
				tie.logger.Error("Failed to update threat feed",
					zap.String("feed", f.ID),
					zap.Error(err))
				f.Status = "error"
				f.LastError = err.Error()

				stats := tie.feedStats[f.ID]
				stats.FailedUpdates++
				stats.ErrorRate = float64(stats.FailedUpdates) / float64(stats.TotalUpdates)
			} else {
				f.Status = "active"
				f.LastError = ""
				f.LastUpdate = time.Now()

				stats := tie.feedStats[f.ID]
				stats.SuccessfulUpdates++
				stats.LastUpdateTime = time.Now()

				// Update average update time
				duration := time.Since(start)
				if stats.AverageUpdateTime == 0 {
					stats.AverageUpdateTime = duration
				} else {
					stats.AverageUpdateTime = (stats.AverageUpdateTime + duration) / 2
				}
			}

			stats := tie.feedStats[f.ID]
			stats.TotalUpdates++
			if stats.TotalUpdates > 0 {
				stats.ErrorRate = float64(stats.FailedUpdates) / float64(stats.TotalUpdates)
			}
		}(feed)
	}
}

func (tie *ThreatIntelligenceEngine) updateFeed(feed *ThreatFeed) error {
	ctx, cancel := context.WithTimeout(tie.ctx, feed.Config.Timeout)
	defer cancel()

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", feed.Config.URL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication and headers
	if feed.Config.APIKey != "" {
		req.Header.Set("X-API-Key", feed.Config.APIKey)
	}
	if feed.Config.Username != "" && feed.Config.Password != "" {
		req.SetBasicAuth(feed.Config.Username, feed.Config.Password)
	}
	for key, value := range feed.Config.Headers {
		req.Header.Set(key, value)
	}

	// Make request
	resp, err := tie.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Check if content has changed
	newHash := fmt.Sprintf("%x", md5.Sum(body))
	if feed.Hash == newHash {
		tie.logger.Debug("Feed content unchanged", zap.String("feed", feed.ID))
		return nil
	}

	// Parse feed data
	indicators, err := tie.parseFeedData(feed, body)
	if err != nil {
		return fmt.Errorf("failed to parse feed data: %w", err)
	}

	// Queue indicators for processing
	update := &ThreatUpdate{
		FeedID:     feed.ID,
		Indicators: indicators,
		Timestamp:  time.Now(),
		UpdateType: "full",
	}

	select {
	case tie.updateQueue <- update:
		feed.Hash = newHash
		feed.RecordCount = len(indicators)
		tie.feedStats[feed.ID].NewIndicators = len(indicators)
		tie.logger.Info("Queued threat indicators for processing",
			zap.String("feed", feed.ID),
			zap.Int("count", len(indicators)))
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("update queue full, dropping update")
	}

	return nil
}

func (tie *ThreatIntelligenceEngine) parseFeedData(feed *ThreatFeed, data []byte) ([]*ThreatIndicator, error) {
	var indicators []*ThreatIndicator

	switch feed.Config.Type {
	case "json":
		indicators = tie.parseJSONFeed(feed, data)
	case "csv":
		indicators = tie.parseCSVFeed(feed, data)
	case "stix":
		indicators = tie.parseSTIXFeed(feed, data)
	case "taxii":
		indicators = tie.parseTAXIIFeed(feed, data)
	case "openIOC":
		indicators = tie.parseOpenIOCFeed(feed, data)
	case "misp":
		indicators = tie.parseMISPFeed(feed, data)
	default:
		return nil, fmt.Errorf("unsupported feed type: %s", feed.Config.Type)
	}

	// Apply filters and transformations
	return tie.applyFeedProcessing(feed, indicators), nil
}

func (tie *ThreatIntelligenceEngine) parseJSONFeed(feed *ThreatFeed, data []byte) []*ThreatIndicator {
	var rawData []map[string]interface{}
	if err := json.Unmarshal(data, &rawData); err != nil {
		tie.logger.Error("Failed to parse JSON feed", zap.String("feed", feed.ID), zap.Error(err))
		return nil
	}

	var indicators []*ThreatIndicator
	for _, item := range rawData {
		indicator := tie.mapJSONToIndicator(feed, item)
		if indicator != nil {
			indicators = append(indicators, indicator)
		}
	}

	return indicators
}

func (tie *ThreatIntelligenceEngine) parseCSVFeed(feed *ThreatFeed, data []byte) []*ThreatIndicator {
	// Implement CSV parsing logic
	lines := strings.Split(string(data), "\n")
	var indicators []*ThreatIndicator

	if len(lines) < 2 {
		return indicators
	}

	headers := strings.Split(lines[0], ",")
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}

		fields := strings.Split(lines[i], ",")
		if len(fields) != len(headers) {
			continue
		}

		// Create indicator from CSV row
		item := make(map[string]interface{})
		for j, header := range headers {
			item[strings.TrimSpace(header)] = strings.TrimSpace(fields[j])
		}

		indicator := tie.mapJSONToIndicator(feed, item)
		if indicator != nil {
			indicators = append(indicators, indicator)
		}
	}

	return indicators
}

func (tie *ThreatIntelligenceEngine) parseSTIXFeed(feed *ThreatFeed, data []byte) []*ThreatIndicator {
	// Implement STIX/TAXII parsing
	// This would parse STIX 2.1 format
	return []*ThreatIndicator{}
}

func (tie *ThreatIntelligenceEngine) parseTAXIIFeed(feed *ThreatFeed, data []byte) []*ThreatIndicator {
	// Implement TAXII parsing
	return []*ThreatIndicator{}
}

func (tie *ThreatIntelligenceEngine) parseOpenIOCFeed(feed *ThreatFeed, data []byte) []*ThreatIndicator {
	// Implement OpenIOC XML parsing
	return []*ThreatIndicator{}
}

func (tie *ThreatIntelligenceEngine) parseMISPFeed(feed *ThreatFeed, data []byte) []*ThreatIndicator {
	// Implement MISP format parsing
	return []*ThreatIndicator{}
}

func (tie *ThreatIntelligenceEngine) mapJSONToIndicator(feed *ThreatFeed, item map[string]interface{}) *ThreatIndicator {
	fieldMapping := feed.Config.Format.Fields

	// Extract basic fields
	indicatorType, _ := item[fieldMapping["type"]].(string)
	value, _ := item[fieldMapping["value"]].(string)
	confidence, _ := item[fieldMapping["confidence"]].(float64)
	severity, _ := item[fieldMapping["severity"]].(string)

	if indicatorType == "" || value == "" {
		return nil
	}

	indicator := &ThreatIndicator{
		ID:         fmt.Sprintf("%s_%s_%x", feed.ID, indicatorType, md5.Sum([]byte(value))),
		Type:       indicatorType,
		Value:      value,
		Confidence: confidence,
		Severity:   severity,
		Sources:    []string{feed.ID},
		FirstSeen:  time.Now(),
		LastSeen:   time.Now(),
		TLP:        "amber", // Default TLP
		Tags:       []string{},
		Metadata:   make(map[string]interface{}),
	}

	// Extract categories if available
	if categoriesData, ok := item[fieldMapping["categories"]]; ok {
		if categoriesStr, ok := categoriesData.(string); ok {
			indicator.Categories = strings.Split(categoriesStr, ",")
		}
	}

	// Extract tags if available
	if tagsData, ok := item[fieldMapping["tags"]]; ok {
		if tagsStr, ok := tagsData.(string); ok {
			indicator.Tags = strings.Split(tagsStr, ",")
		}
	}

	// Extract expiration if available
	if expiresData, ok := item[fieldMapping["expires_at"]]; ok {
		if expiresStr, ok := expiresData.(string); ok {
			if expiresTime, err := time.Parse(time.RFC3339, expiresStr); err == nil {
				indicator.ExpiresAt = &expiresTime
			}
		}
	}

	// Store additional metadata
	for key, value := range item {
		if !strings.HasPrefix(key, "_") {
			indicator.Metadata[key] = value
		}
	}

	return indicator
}

func (tie *ThreatIntelligenceEngine) applyFeedProcessing(feed *ThreatFeed, indicators []*ThreatIndicator) []*ThreatIndicator {
	var processed []*ThreatIndicator

	for _, indicator := range indicators {
		// Apply transformations
		for _, transform := range feed.Config.Format.Transforms {
			tie.applyTransform(indicator, transform)
		}

		// Apply filters
		if tie.passesFilters(indicator, feed.Config.Format.Filters) {
			// Apply validation
			if tie.validateIndicator(indicator, feed.Config.Format.Validation) {
				processed = append(processed, indicator)
			}
		}
	}

	return processed
}

func (tie *ThreatIntelligenceEngine) applyTransform(indicator *ThreatIndicator, transform FieldTransform) {
	switch transform.Operation {
	case "lowercase":
		if transform.Field == "value" {
			indicator.Value = strings.ToLower(indicator.Value)
		}
	case "uppercase":
		if transform.Field == "value" {
			indicator.Value = strings.ToUpper(indicator.Value)
		}
	case "trim":
		if transform.Field == "value" {
			indicator.Value = strings.TrimSpace(indicator.Value)
		}
	}
}

func (tie *ThreatIntelligenceEngine) passesFilters(indicator *ThreatIndicator, filters []FeedFilter) bool {
	for _, filter := range filters {
		if !tie.evaluateFilter(indicator, filter) {
			return false
		}
	}
	return true
}

func (tie *ThreatIntelligenceEngine) evaluateFilter(indicator *ThreatIndicator, filter FeedFilter) bool {
	var fieldValue interface{}

	switch filter.Field {
	case "type":
		fieldValue = indicator.Type
	case "confidence":
		fieldValue = indicator.Confidence
	case "severity":
		fieldValue = indicator.Severity
	default:
		fieldValue = indicator.Metadata[filter.Field]
	}

	switch filter.Operator {
	case "equals":
		return fieldValue == filter.Value
	case "contains":
		if str, ok := fieldValue.(string); ok {
			if filterStr, ok := filter.Value.(string); ok {
				return strings.Contains(str, filterStr)
			}
		}
	case "gt":
		if num, ok := fieldValue.(float64); ok {
			if filterNum, ok := filter.Value.(float64); ok {
				return num > filterNum
			}
		}
	case "lt":
		if num, ok := fieldValue.(float64); ok {
			if filterNum, ok := filter.Value.(float64); ok {
				return num < filterNum
			}
		}
	}

	return false
}

func (tie *ThreatIntelligenceEngine) validateIndicator(indicator *ThreatIndicator, validation ValidationRules) bool {
	// Check required fields
	for _, field := range validation.RequiredFields {
		switch field {
		case "type":
			if indicator.Type == "" {
				return false
			}
		case "value":
			if indicator.Value == "" {
				return false
			}
		case "confidence":
			if indicator.Confidence == 0 {
				return false
			}
		}
	}

	// IP validation
	if validation.IPValidation && indicator.Type == "ip" {
		// Basic IP validation
		parts := strings.Split(indicator.Value, ".")
		if len(parts) != 4 {
			return false
		}
	}

	// Domain validation
	if validation.DomainValidation && indicator.Type == "domain" {
		// Basic domain validation
		if !strings.Contains(indicator.Value, ".") {
			return false
		}
	}

	return true
}

func (tie *ThreatIntelligenceEngine) processingWorkers() {
	for i := 0; i < tie.config.Processing.Workers; i++ {
		go tie.processingWorker(i)
	}
}

func (tie *ThreatIntelligenceEngine) processingWorker(workerID int) {
	tie.logger.Info("Started threat processing worker", zap.Int("worker_id", workerID))

	for {
		select {
		case <-tie.ctx.Done():
			return
		case update := <-tie.updateQueue:
			tie.processUpdate(update)
		}
	}
}

func (tie *ThreatIntelligenceEngine) processUpdate(update *ThreatUpdate) {
	tie.logger.Debug("Processing threat update",
		zap.String("feed", update.FeedID),
		zap.Int("indicators", len(update.Indicators)))

	newThreats := 0
	autoBlocked := 0

	for _, indicator := range update.Indicators {
		// Check for duplicates if deduplication is enabled
		if tie.config.Processing.DeduplicationEnabled {
			if existing, exists := tie.indicators[indicator.ID]; exists {
				// Update existing indicator
				existing.LastSeen = time.Now()
				existing.Sources = tie.mergeSlices(existing.Sources, indicator.Sources)
				continue
			}
		}

		// Enrich indicator if enabled
		if tie.config.Processing.EnrichmentEnabled {
			tie.enrichIndicator(indicator)
		}

		// Store indicator
		tie.storeIndicator(indicator)
		newThreats++

		// Check for auto-blocking
		if tie.shouldAutoBlock(indicator) {
			if err := tie.autoBlockIndicator(indicator); err != nil {
				tie.logger.Error("Failed to auto-block indicator",
					zap.String("indicator", indicator.ID),
					zap.Error(err))
			} else {
				autoBlocked++
			}
		}
	}

	tie.logger.Info("Processed threat update",
		zap.String("feed", update.FeedID),
		zap.Int("new_threats", newThreats),
		zap.Int("auto_blocked", autoBlocked))

	// Send notifications if enabled
	if tie.config.Notifications.Enabled && tie.config.Notifications.OnNewThreats && newThreats > 0 {
		tie.sendNotification(fmt.Sprintf("New threats detected: %d from feed %s", newThreats, update.FeedID))
	}
}

func (tie *ThreatIntelligenceEngine) enrichIndicator(indicator *ThreatIndicator) {
	// Geolocation lookup for IPs
	if tie.config.Processing.GeolocationLookup && indicator.Type == "ip" {
		// This would integrate with a geolocation service
		// For now, we'll add placeholder enrichment
		indicator.Geolocation = &GeoLocation{
			Country: "Unknown",
		}
	}

	// DNS resolution
	if tie.config.Processing.DNSResolution && indicator.Type == "domain" {
		// This would perform DNS lookups
		indicator.DNSInfo = &DNSInfo{
			Domain: indicator.Value,
		}
	}
}

func (tie *ThreatIntelligenceEngine) storeIndicator(indicator *ThreatIndicator) {
	tie.indicators[indicator.ID] = indicator

	// Store in appropriate threat database collection
	switch indicator.Type {
	case "ip":
		ipThreat := &IPThreat{
			ThreatIndicator: indicator,
			IPAddress:       indicator.Value,
		}
		tie.threatData.IPAddresses[indicator.Value] = ipThreat
	case "domain":
		domainThreat := &DomainThreat{
			ThreatIndicator: indicator,
			Domain:          indicator.Value,
		}
		tie.threatData.Domains[indicator.Value] = domainThreat
	case "url":
		urlThreat := &URLThreat{
			ThreatIndicator: indicator,
			URL:             indicator.Value,
		}
		tie.threatData.URLs[indicator.Value] = urlThreat
	case "hash":
		fileThreat := &FileThreat{
			ThreatIndicator: indicator,
			Hash:            indicator.Value,
		}
		tie.threatData.FileHashes[indicator.Value] = fileThreat
	}

	tie.threatData.TotalIndicators++
	tie.threatData.LastUpdate = time.Now()

	// Store in Redis for persistence
	tie.storeThreatInRedis(indicator)
}

func (tie *ThreatIntelligenceEngine) shouldAutoBlock(indicator *ThreatIndicator) bool {
	if !tie.config.AutoBlock.Enabled {
		return false
	}

	// Check confidence threshold
	if indicator.Confidence < tie.config.AutoBlock.ConfidenceThreshold {
		return false
	}

	// Check if high confidence blocking is required
	if tie.config.AutoBlock.HighConfidenceBlock && indicator.Confidence < 0.9 {
		return false
	}

	// Check categories
	if len(tie.config.AutoBlock.Categories) > 0 {
		hasMatchingCategory := false
		for _, category := range indicator.Categories {
			for _, allowedCategory := range tie.config.AutoBlock.Categories {
				if category == allowedCategory {
					hasMatchingCategory = true
					break
				}
			}
		}
		if !hasMatchingCategory {
			return false
		}
	}

	// Check excluded feeds
	for _, source := range indicator.Sources {
		for _, excludedFeed := range tie.config.AutoBlock.ExcludeFeeds {
			if source == excludedFeed {
				return false
			}
		}
	}

	// Check rate limiting
	// This would implement hourly rate limiting for auto-blocks

	return true
}

func (tie *ThreatIntelligenceEngine) autoBlockIndicator(indicator *ThreatIndicator) error {
	if indicator.Type != "ip" {
		return nil // Only auto-block IPs for now
	}

	// Add to XDP blocklist
	if tie.xdpController != nil {
		rule := map[string]interface{}{
			"ip_address":  indicator.Value,
			"reason":      fmt.Sprintf("Auto-blocked: %s", strings.Join(indicator.Categories, ",")),
			"blocked_at":  time.Now().Format(time.RFC3339),
			"blocked_by":  "threat_intelligence",
			"expires_at":  time.Now().Add(tie.config.AutoBlock.BlockDuration).Format(time.RFC3339),
			"rule_type":   "auto",
			"confidence":  indicator.Confidence,
			"sources":     strings.Join(indicator.Sources, ","),
		}

		if err := tie.xdpController.AddBlockingRule(indicator.Value, rule); err != nil {
			return err
		}
	}

	// Mark as blocked
	indicator.Blocked = true
	now := time.Now()
	indicator.BlockedAt = &now

	tie.logger.Info("Auto-blocked threat indicator",
		zap.String("type", indicator.Type),
		zap.String("value", indicator.Value),
		zap.Float64("confidence", indicator.Confidence))

	// Send notification
	if tie.config.Notifications.Enabled && tie.config.Notifications.OnBlocks {
		message := fmt.Sprintf("Auto-blocked threat: %s (%s) - Confidence: %.2f",
			indicator.Value, indicator.Type, indicator.Confidence)
		tie.sendNotification(message)
	}

	return nil
}

func (tie *ThreatIntelligenceEngine) cleanupExpiredThreats() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-tie.ctx.Done():
			return
		case <-ticker.C:
			tie.performCleanup()
		}
	}
}

func (tie *ThreatIntelligenceEngine) performCleanup() {
	now := time.Now()
	expired := 0

	for id, indicator := range tie.indicators {
		if indicator.ExpiresAt != nil && now.After(*indicator.ExpiresAt) {
			delete(tie.indicators, id)

			// Remove from specific collections
			switch indicator.Type {
			case "ip":
				delete(tie.threatData.IPAddresses, indicator.Value)
			case "domain":
				delete(tie.threatData.Domains, indicator.Value)
			case "url":
				delete(tie.threatData.URLs, indicator.Value)
			case "hash":
				delete(tie.threatData.FileHashes, indicator.Value)
			}

			// Remove from XDP if blocked
			if indicator.Blocked && tie.xdpController != nil {
				tie.xdpController.RemoveBlockingRule(indicator.Value)
			}

			expired++
		}
	}

	tie.threatData.TotalIndicators -= expired
	tie.threatData.LastUpdate = now

	if expired > 0 {
		tie.logger.Info("Cleaned up expired threats", zap.Int("count", expired))
	}
}

func (tie *ThreatIntelligenceEngine) backupLoop() {
	if !tie.config.Storage.BackupEnabled {
		return
	}

	ticker := time.NewTicker(tie.config.Storage.BackupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-tie.ctx.Done():
			return
		case <-ticker.C:
			tie.performBackup()
		}
	}
}

func (tie *ThreatIntelligenceEngine) performBackup() {
	tie.logger.Info("Performing threat data backup")

	// This would implement backup logic to configured storage
	backupData := map[string]interface{}{
		"threat_data": tie.threatData,
		"indicators":  tie.indicators,
		"feed_stats":  tie.feedStats,
		"timestamp":   time.Now(),
	}

	backupJSON, err := json.Marshal(backupData)
	if err != nil {
		tie.logger.Error("Failed to marshal backup data", zap.Error(err))
		return
	}

	// Store backup in Redis with expiration
	backupKey := fmt.Sprintf("articdbm:threat_intel:backup:%d", time.Now().Unix())
	tie.redisClient.Set(tie.ctx, backupKey, backupJSON, 7*24*time.Hour) // 7 days retention
}

func (tie *ThreatIntelligenceEngine) loadThreatData() {
	// Load existing threat data from Redis
	keys, err := tie.redisClient.Keys(tie.ctx, "articdbm:threat_intel:indicator:*").Result()
	if err != nil {
		tie.logger.Error("Failed to load threat data keys", zap.Error(err))
		return
	}

	loaded := 0
	for _, key := range keys {
		data, err := tie.redisClient.Get(tie.ctx, key).Result()
		if err != nil {
			continue
		}

		var indicator ThreatIndicator
		if err := json.Unmarshal([]byte(data), &indicator); err != nil {
			continue
		}

		tie.indicators[indicator.ID] = &indicator
		loaded++
	}

	tie.logger.Info("Loaded existing threat indicators", zap.Int("count", loaded))
}

func (tie *ThreatIntelligenceEngine) storeThreatInRedis(indicator *ThreatIndicator) {
	data, err := json.Marshal(indicator)
	if err != nil {
		return
	}

	key := fmt.Sprintf("articdbm:threat_intel:indicator:%s", indicator.ID)
	expiration := time.Duration(tie.config.Storage.RetentionDays) * 24 * time.Hour
	tie.redisClient.Set(tie.ctx, key, data, expiration)
}

func (tie *ThreatIntelligenceEngine) sendNotification(message string) {
	// Implement notification sending (webhook, email, Slack, etc.)
	tie.logger.Info("Threat Intelligence Notification", zap.String("message", message))

	// This would integrate with actual notification services
}

func (tie *ThreatIntelligenceEngine) mergeSlices(a, b []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(a)+len(b))

	for _, v := range a {
		if !seen[v] {
			result = append(result, v)
			seen[v] = true
		}
	}

	for _, v := range b {
		if !seen[v] {
			result = append(result, v)
			seen[v] = true
		}
	}

	return result
}

// Public API methods

// GetThreatByValue checks if a value (IP, domain, etc.) is a known threat
func (tie *ThreatIntelligenceEngine) GetThreatByValue(value string) *ThreatIndicator {
	return tie.indicators[value]
}

// IsIPThreat checks if an IP address is a known threat
func (tie *ThreatIntelligenceEngine) IsIPThreat(ip string) bool {
	if threat, exists := tie.threatData.IPAddresses[ip]; exists {
		if threat.ExpiresAt == nil || time.Now().Before(*threat.ExpiresAt) {
			return true
		}
	}
	return false
}

// IsDomainThreat checks if a domain is a known threat
func (tie *ThreatIntelligenceEngine) IsDomainThreat(domain string) bool {
	if threat, exists := tie.threatData.Domains[domain]; exists {
		if threat.ExpiresAt == nil || time.Now().Before(*threat.ExpiresAt) {
			return true
		}
	}
	return false
}

// GetThreatStats returns current threat intelligence statistics
func (tie *ThreatIntelligenceEngine) GetThreatStats() map[string]interface{} {
	return map[string]interface{}{
		"total_indicators":  tie.threatData.TotalIndicators,
		"ip_threats":       len(tie.threatData.IPAddresses),
		"domain_threats":   len(tie.threatData.Domains),
		"url_threats":      len(tie.threatData.URLs),
		"file_threats":     len(tie.threatData.FileHashes),
		"active_feeds":     len(tie.activeFeeds),
		"last_update":      tie.threatData.LastUpdate,
		"feed_stats":       tie.feedStats,
	}
}

// UpdateFeedConfig updates configuration for a specific feed
func (tie *ThreatIntelligenceEngine) UpdateFeedConfig(feedID string, config *FeedConfig) error {
	tie.activeFeedsmu.Lock()
	defer tie.activeFeedsmu.Unlock()

	if feed, exists := tie.activeFeeds[feedID]; exists {
		feed.Config = config
		tie.logger.Info("Updated feed configuration", zap.String("feed", feedID))
		return nil
	}

	return fmt.Errorf("feed not found: %s", feedID)
}

// AddCustomThreat manually adds a threat indicator
func (tie *ThreatIntelligenceEngine) AddCustomThreat(indicator *ThreatIndicator) error {
	indicator.Sources = append(indicator.Sources, "manual")
	indicator.FirstSeen = time.Now()
	indicator.LastSeen = time.Now()

	tie.storeIndicator(indicator)

	tie.logger.Info("Added custom threat indicator",
		zap.String("type", indicator.Type),
		zap.String("value", indicator.Value))

	return nil
}

// Close shuts down the threat intelligence engine
func (tie *ThreatIntelligenceEngine) Close() error {
	tie.cancel()
	tie.performBackup()
	tie.logger.Info("Threat Intelligence Engine stopped")
	return nil
}