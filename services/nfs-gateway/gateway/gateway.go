// Package gateway manages NFS-Ganesha export configuration for Nest NFS resources.
// Each NFS DataResource maps to a Ganesha EXPORT block backed by a CephFS subvolume.
package gateway

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
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
	cfg     Config
	mu      sync.RWMutex
	exports map[string]*Export
	nextID  int
}

// exportTmpl is the Ganesha EXPORT block template.
var exportTmpl = template.Must(template.New("export").Parse(`
EXPORT {
    Export_Id = {{ .ExportID }};
    Path = "{{ .Path }}";
    Pseudo = "/nest/{{ .Tenant }}/{{ .Name }}";
    Protocols = 4;
    Transports = TCP;
    Access_Type = {{ .AccessType }};
    Squash = no_root_squash;
    FSAL {
        Name = CEPH;
        Filesystem = "nest-cephfs";
        User_Id = "admin";
        Secret_Access_Key = "";
    }
    CLIENT {
        Clients = {{ .Clients }};
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
	return &Gateway{
		cfg:     cfg,
		exports: make(map[string]*Export),
		nextID:  100, // start at 100 to avoid conflicts with system exports
	}
}

// CreateExportRequest is the request body for creating an NFS export.
type CreateExportRequest struct {
	Name       string `json:"name" binding:"required"`
	Tenant     string `json:"tenant" binding:"required"`
	Path       string `json:"path" binding:"required"`
	AccessMode string `json:"accessMode"` // ro | rw, default rw
	Clients    string `json:"clients"`    // default *
}

// CreateExport handles POST /api/v1/exports.
func (gw *Gateway) CreateExport(c *gin.Context) {
	var req CreateExportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "nest.nfs.invalid", "message": err.Error()})
		return
	}
	if req.AccessMode == "" {
		req.AccessMode = "rw"
	}
	if req.Clients == "" {
		req.Clients = "*"
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
	_ = os.Remove(configFile)
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
