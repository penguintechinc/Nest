// Package gateway manages NFS-Ganesha export configuration for Nest NFS resources.
// Each NFS DataResource maps to a Ganesha EXPORT block backed by a CephFS subvolume.
package gateway

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Config holds NFS gateway configuration.
type Config struct {
	GaneshaConfigPath string
	GaneshaSocketPath string
	CephFSMount       string
	Logger            *log.Logger
	Reloader          GaneshaReloader // Optional; if nil, a default DBus reloader is created
}

// Export represents a managed NFS export.
type Export struct {
	ID         string    `json:"id"`
	ExportID   int       `json:"exportId"`
	Name       string    `json:"name"`
	Tenant     string    `json:"tenant"`
	Path       string    `json:"path"`
	AccessMode string    `json:"accessMode"` // ro | rw
	Clients    string    `json:"clients"`    // CIDR or * for all
	CreatedAt  time.Time `json:"createdAt"`
}

// Gateway manages NFS exports via Ganesha configuration files.
type Gateway struct {
	cfg      Config
	mu       sync.RWMutex
	exports  map[string]*Export
	nextID   int
	reloader GaneshaReloader
}

// exportTmpl is the Ganesha EXPORT block template.
// Note: paths are validated to prevent injection; clients are restricted by default.
var exportTmpl = template.Must(template.New("export").Parse(`
EXPORT {
    Export_Id = {{ .ExportID }};
    Path = "{{ .Path }}";
    Pseudo = "/nest/{{ .Tenant }}/{{ .Name }}";
    Protocols = 4;
    Transports = TCP;
    Access_Type = {{ .AccessType }};
    Squash = root_squash;
    FSAL {
        Name = CEPH;
        Filesystem = "nest-cephfs";
        User_Id = "admin";
        Secret_Access_Key = "";
    }
    CLIENT {
        Clients = "{{ .Clients }}";
        Access_Type = {{ .AccessType }};
    }
}
`))

// exportTemplateData is passed to the export template.
type exportTemplateData struct {
	ExportID   int
	Path       string
	Tenant     string
	Name       string
	AccessType string
	Clients    string
}

// New creates a new Gateway.
func New(cfg Config) *Gateway {
	// Create default reloader if not provided
	if cfg.Reloader == nil {
		cfg.Reloader = NewDBusGaneshaReloader(cfg.GaneshaSocketPath)
	}

	return &Gateway{
		cfg:      cfg,
		exports:  make(map[string]*Export),
		nextID:   100, // start at 100 to avoid conflicts with system exports
		reloader: cfg.Reloader,
	}
}

// CreateExportRequest is the request body for creating an NFS export.
type CreateExportRequest struct {
	Name       string `json:"name" binding:"required"`
	Tenant     string `json:"tenant" binding:"required"`
	Path       string `json:"path" binding:"required"`
	AccessMode string `json:"accessMode"` // ro | rw, default rw
	Clients    string `json:"clients"`    // CIDR or list, default 127.0.0.1
}

// validatePath ensures the path is a safe CephFS subvolume reference and prevents path injection.
// Allowed format: /volumes/{tenant}/... (tenant scoped to prevent cross-tenant access)
func validatePath(path, tenant string) error {
	// Path must be absolute
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must be absolute")
	}

	// Path must not contain .. or other traversal sequences
	if strings.Contains(path, "..") || strings.Contains(path, "//") {
		return fmt.Errorf("path contains invalid traversal sequences")
	}

	// Path must be scoped to tenant to prevent cross-tenant access
	// Expected format: /volumes/{tenant}/... or similar tenant-scoped paths
	expectedPrefix := fmt.Sprintf("/volumes/%s/", tenant)
	if !strings.HasPrefix(path, expectedPrefix) && path != fmt.Sprintf("/volumes/%s", tenant) {
		return fmt.Errorf("path must be scoped to tenant (expected to start with %s)", expectedPrefix)
	}

	// Path must be normalized (no symlinks, etc.)
	// Clean it to catch any injection attempts
	cleaned := filepath.Clean(path)
	if cleaned != path {
		return fmt.Errorf("path is not normalized")
	}

	return nil
}

// validateClients ensures the Clients field is a valid CIDR or IP list and not an injection attempt.
func validateClients(clients string) error {
	if clients == "" {
		return fmt.Errorf("clients cannot be empty")
	}

	// Only allow CIDR notation (x.x.x.x/nn), IP addresses, and comma-separated lists
	// Pattern: IPv4 CIDR (x.x.x.x/y), IPv4 (x.x.x.x), or alphanumeric hostnames
	validPattern := regexp.MustCompile(`^[0-9a-zA-Z.,/:]*$`)
	if !validPattern.MatchString(clients) {
		return fmt.Errorf("clients field contains invalid characters")
	}

	return nil
}

// CreateExport handles POST /api/v1/exports.
// Validates path, clients, and enforces tenant-scoped access.
func (gw *Gateway) CreateExport(c *gin.Context) {
	var req CreateExportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "nest.nfs.invalid", "message": err.Error()})
		return
	}

	// Validate path to prevent injection and cross-tenant access
	if err := validatePath(req.Path, req.Tenant); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "nest.nfs.invalid_path", "message": err.Error()})
		return
	}

	// Set defaults with security in mind
	if req.AccessMode == "" {
		req.AccessMode = "rw"
	}
	if req.Clients == "" {
		req.Clients = "127.0.0.1" // Restrict to localhost by default, not *
	}

	// Validate clients field to prevent injection
	if err := validateClients(req.Clients); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "nest.nfs.invalid_clients", "message": err.Error()})
		return
	}

	gw.mu.Lock()
	id := uuid.New().String()
	exportID := gw.nextID
	gw.nextID++
	export := &Export{
		ID:         id,
		ExportID:   exportID,
		Name:       req.Name,
		Tenant:     req.Tenant,
		Path:       req.Path,
		AccessMode: req.AccessMode,
		Clients:    req.Clients,
		CreatedAt:  time.Now().UTC(),
	}
	gw.exports[id] = export
	gw.mu.Unlock()

	if err := gw.writeExportConfig(export); err != nil {
		gw.mu.Lock()
		delete(gw.exports, id)
		gw.mu.Unlock()
		c.JSON(http.StatusInternalServerError, gin.H{"code": "nest.nfs.config_write_failed", "message": err.Error()})
		return
	}

	// Reload Ganesha to activate the new export
	// Use a context with timeout for the reload
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	if err := gw.reloader.Reload(ctx); err != nil {
		// Reload failed; remove export config and reject the request
		gw.mu.Lock()
		delete(gw.exports, id)
		gw.mu.Unlock()
		configFile := filepath.Join(gw.cfg.GaneshaConfigPath, fmt.Sprintf("export-%s.conf", id))
		_ = os.Remove(configFile)
		gw.cfg.Logger.Printf("Error: failed to reload Ganesha after creating export config: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": "nest.nfs.reload_failed", "message": fmt.Sprintf("failed to activate export: %v", err)})
		return
	}

	gw.cfg.Logger.Printf("NFS export created: %s (%s/%s)", id, req.Tenant, req.Name)
	c.JSON(http.StatusCreated, export)
}

// DeleteExport handles DELETE /api/v1/exports/:exportId.
func (gw *Gateway) DeleteExport(c *gin.Context) {
	exportID := c.Param("exportId")
	gw.mu.Lock()
	export, ok := gw.exports[exportID]
	if !ok {
		gw.mu.Unlock()
		c.JSON(http.StatusNotFound, gin.H{"code": "nest.nfs.not_found", "message": "export not found"})
		return
	}
	delete(gw.exports, exportID)
	gw.mu.Unlock()

	configFile := filepath.Join(gw.cfg.GaneshaConfigPath, fmt.Sprintf("export-%s.conf", exportID))
	if err := os.Remove(configFile); err != nil && !os.IsNotExist(err) {
		// If config file removal failed (but file exists), restore export and reject
		gw.mu.Lock()
		gw.exports[exportID] = export
		gw.mu.Unlock()
		c.JSON(http.StatusInternalServerError, gin.H{"code": "nest.nfs.config_remove_failed", "message": err.Error()})
		return
	}

	// Reload Ganesha to remove the export
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	if err := gw.reloader.Reload(ctx); err != nil {
		// Reload failed after config file removal; restore export and reject
		gw.mu.Lock()
		gw.exports[exportID] = export
		gw.mu.Unlock()
		// Attempt to restore config file (write export config back)
		_ = gw.writeExportConfig(export)
		gw.cfg.Logger.Printf("Error: failed to reload Ganesha after deleting export config: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": "nest.nfs.reload_failed", "message": fmt.Sprintf("failed to remove export: %v", err)})
		return
	}

	gw.cfg.Logger.Printf("NFS export deleted: %s (%s/%s)", exportID, export.Tenant, export.Name)
	c.Status(http.StatusNoContent)
}

// ListExports handles GET /api/v1/exports.
func (gw *Gateway) ListExports(c *gin.Context) {
	tenant := c.Query("tenant")
	gw.mu.RLock()
	defer gw.mu.RUnlock()
	result := make([]*Export, 0, len(gw.exports))
	for _, e := range gw.exports {
		if tenant == "" || e.Tenant == tenant {
			result = append(result, e)
		}
	}
	c.JSON(http.StatusOK, gin.H{"items": result, "count": len(result)})
}

// GetExport handles GET /api/v1/exports/:exportId.
func (gw *Gateway) GetExport(c *gin.Context) {
	exportID := c.Param("exportId")
	gw.mu.RLock()
	defer gw.mu.RUnlock()
	export, ok := gw.exports[exportID]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": "nest.nfs.not_found", "message": "export not found"})
		return
	}
	c.JSON(http.StatusOK, export)
}

func (gw *Gateway) writeExportConfig(export *Export) error {
	if err := os.MkdirAll(gw.cfg.GaneshaConfigPath, 0o755); err != nil {
		return err
	}
	accessType := "RW"
	if export.AccessMode == "ro" {
		accessType = "RO"
	}
	data := exportTemplateData{
		ExportID:   export.ExportID,
		Path:       export.Path,
		Tenant:     export.Tenant,
		Name:       export.Name,
		AccessType: accessType,
		Clients:    export.Clients,
	}
	configFile := filepath.Join(gw.cfg.GaneshaConfigPath, fmt.Sprintf("export-%s.conf", export.ID))
	f, err := os.Create(configFile)
	if err != nil {
		return err
	}
	defer f.Close()
	return exportTmpl.Execute(f, data)
}
