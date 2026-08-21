package controllers

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
	cloudprovider "github.com/penguintechinc/nest/pkg/cloudprovider"
)

const (
	// importedProbeTimeout bounds the TCP reachability probe so a black-holed
	// endpoint cannot stall the reconcile loop.
	importedProbeTimeout = 5 * time.Second

	// importedHealthInterval is how often an adopted resource is re-probed.
	importedHealthInterval = 60 * time.Second
)

// reconcileImported handles DataResources with origination: imported.
//
// An imported resource is a pre-existing engine that Nest ADOPTS but does not
// own — an RDS/Aurora instance, a Cloud SQL database, a self-hosted Postgres.
// Reconciliation is strictly READ-ONLY: it derives an endpoint, reports health,
// and optionally enriches status with provider metadata. It never provisions,
// mutates, or deletes the external resource.
//
// Before this existed, `origination: imported` had no handler at all and fell
// through to the engine dispatch below it, so adopting an existing RDS instance
// would instead provision a brand-new empty in-cluster cluster. Guarding that is
// the single most important thing this function does.
//
// Two layers, additive:
//
//	engine-level (always)  spec.import.connectionString yields a host:port that
//	                       is probed for reachability. Works for any engine,
//	                       cloud-managed or not.
//	cloud-level (optional) when spec.external is also set, the cloud API is
//	                       queried for engine type/version and provider-reported
//	                       health.
func (r *DataResourceReconciler) reconcileImported(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	if dr.Spec.Import == nil {
		return fmt.Errorf("imported DataResource %s/%s missing spec.import", dr.Namespace, dr.Name)
	}

	// Fail closed on management features that would MUTATE the adopted resource.
	// These are opt-in booleans on the CRD, but no implementation exists yet.
	// Silently ignoring them while reporting Ready would leave an operator
	// believing Nest is rotating credentials on their production database when
	// it is not — so refuse the resource outright instead.
	if dr.Spec.Import.ManagedCredentials {
		return fmt.Errorf("spec.import.managedCredentials is not yet supported: " +
			"Nest cannot rotate credentials on an imported resource; unset it to adopt read-only")
	}
	if dr.Spec.Import.ManagedFailover {
		return fmt.Errorf("spec.import.managedFailover is not yet supported: " +
			"Nest cannot perform failover on an imported resource; unset it to adopt read-only")
	}

	secretData, err := r.resolveImportCredentials(ctx, dr)
	if err != nil {
		return err
	}

	// Derive the wire endpoint. The connection string carries a password, so
	// only the host:port is ever retained or logged.
	endpoint, err := importedEndpoint(dr.Spec.Import.ConnectionString)
	if err != nil {
		return err
	}

	var info *cloudprovider.ExternalResourceInfo
	var cloudHealth *cloudprovider.HealthResult

	if dr.Spec.External != nil && dr.Spec.External.Provider != "" {
		info, cloudHealth, err = r.enrichFromCloud(ctx, dr, secretData)
		if err != nil {
			// Cloud enrichment is best-effort: an IAM gap or a provider outage
			// must not make an otherwise-reachable adopted resource look failed.
			logger.Info("cloud enrichment unavailable; falling back to endpoint probe",
				"provider", dr.Spec.External.Provider, "reason", err.Error())
		}
		if endpoint == "" && info != nil {
			endpoint = info.Endpoint
		}
	}

	if endpoint == "" {
		return fmt.Errorf("imported DataResource %s/%s: no endpoint available; "+
			"set spec.import.connectionString or spec.external for discovery", dr.Namespace, dr.Name)
	}

	state, message := importedHealth(ctx, endpoint, cloudHealth)

	patch := client.MergeFrom(dr.DeepCopy())
	if dr.Status.Endpoints == nil {
		dr.Status.Endpoints = &nestv1.ResourceEndpoints{}
	}
	dr.Status.Endpoints.Native = endpoint
	dr.Status.Health = &nestv1.HealthSignal{State: state, Message: message}
	dr.Status.Phase = phaseForHealth(state)
	dr.Status.ObservedGeneration = dr.Generation

	if err := r.Client.Status().Patch(ctx, dr, patch); err != nil {
		return fmt.Errorf("patch imported status: %w", err)
	}

	logger.Info("imported DataResource reconciled",
		"name", dr.Name,
		"namespace", dr.Namespace,
		"type", dr.Spec.Type,
		"endpoint", endpoint,
		"health", string(state),
	)
	return nil
}

// phaseForHealth maps an observed health state onto the resource phase. An
// adopted resource is never "Provisioning" — Nest does not create it — so the
// phase is purely a reflection of what the probe or provider reports.
func phaseForHealth(state nestv1.HealthState) nestv1.DataResourcePhase {
	switch state {
	case nestv1.HealthHealthy:
		return nestv1.PhaseReady
	case nestv1.HealthDegraded:
		return nestv1.PhaseDegraded
	default:
		return nestv1.PhaseFailed
	}
}

// resolveImportCredentials reads the Secret referenced by spec.import, if any.
// A missing reference is not an error — an adopted resource may be reachable
// without Nest holding credentials for it.
func (r *DataResourceReconciler) resolveImportCredentials(ctx context.Context, dr *nestv1.DataResource) (map[string][]byte, error) {
	name := dr.Spec.Import.CredentialSecret
	if name == "" {
		return nil, nil
	}
	secret := &corev1.Secret{}
	key := client.ObjectKey{Name: name, Namespace: dr.Namespace}
	if err := r.Get(ctx, key, secret); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("import credential secret not found: %s/%s", dr.Namespace, name)
		}
		return nil, fmt.Errorf("failed to read import credential secret: %w", err)
	}
	return secret.Data, nil
}

// enrichFromCloud queries the cloud provider for metadata and provider-reported
// health. Read-only: only Validate, Discover, and CheckHealth are called —
// never SetupProxy, and never RotateCredential.
func (r *DataResourceReconciler) enrichFromCloud(
	ctx context.Context,
	dr *nestv1.DataResource,
	secretData map[string][]byte,
) (*cloudprovider.ExternalResourceInfo, *cloudprovider.HealthResult, error) {
	prov, err := cloudprovider.Get(dr.Spec.External.Provider)
	if err != nil {
		return nil, nil, fmt.Errorf("cloud provider %q unavailable: %w", dr.Spec.External.Provider, err)
	}

	// Copy spec.external.extra, then overlay credential material from the
	// Secret. Built fresh per call and never logged — cfg carries secrets.
	extra := make(map[string]string, len(dr.Spec.External.Extra)+len(secretData))
	for k, v := range dr.Spec.External.Extra {
		extra[k] = v
	}
	for k, v := range secretData {
		extra[k] = string(v)
	}

	cfg := cloudprovider.ExternalProviderConfig{
		Provider:         dr.Spec.External.Provider,
		Region:           dr.Spec.External.Region,
		ResourceID:       dr.Spec.External.ResourceID,
		CredentialSecret: dr.Spec.External.CredentialSecret,
		Endpoint:         dr.Spec.External.Endpoint,
		EngineType:       dr.Spec.External.EngineType,
		Extra:            extra,
	}

	if err := prov.Validate(ctx, cfg); err != nil {
		return nil, nil, fmt.Errorf("provider validate: %w", err)
	}

	info, err := prov.Discover(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("provider discover: %w", err)
	}

	// Health is advisory; a provider that cannot report it still yields metadata.
	health, healthErr := prov.CheckHealth(ctx, cfg)
	if healthErr != nil {
		return info, nil, nil
	}
	return info, health, nil
}

// importedEndpoint extracts host:port from an engine connection string.
// The connection string embeds a password, so only the host component is
// returned — the caller stores this in status, which is world-readable.
func importedEndpoint(connectionString string) (string, error) {
	if connectionString == "" {
		return "", nil
	}
	u, err := url.Parse(connectionString)
	if err != nil {
		// Deliberately does not wrap err: url.Parse errors echo the input,
		// which would leak the password into logs and status.
		return "", fmt.Errorf("spec.import.connectionString is not a valid URL")
	}
	if u.Host == "" {
		return "", fmt.Errorf("spec.import.connectionString has no host component")
	}
	return u.Host, nil
}

// importedHealth determines health for an adopted resource, preferring the
// provider's own report and falling back to a TCP reachability probe.
func importedHealth(ctx context.Context, endpoint string, cloudHealth *cloudprovider.HealthResult) (nestv1.HealthState, string) {
	if cloudHealth != nil {
		switch cloudHealth.State {
		case "healthy", "available", "running":
			return nestv1.HealthHealthy, cloudHealth.Message
		case "degraded", "modifying", "backing-up":
			return nestv1.HealthDegraded, cloudHealth.Message
		default:
			msg := cloudHealth.Message
			if msg == "" {
				msg = fmt.Sprintf("provider reports state %q", cloudHealth.State)
			}
			return nestv1.HealthDown, msg
		}
	}
	return probeEndpoint(ctx, endpoint)
}

// probeEndpoint performs a bounded TCP dial to test reachability. This is a
// liveness signal only — it deliberately does not authenticate or issue an
// engine-level query, because Nest must not act on an adopted resource.
func probeEndpoint(ctx context.Context, endpoint string) (nestv1.HealthState, string) {
	dialCtx, cancel := context.WithTimeout(ctx, importedProbeTimeout)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(dialCtx, "tcp", endpoint)
	if err != nil {
		return nestv1.HealthDown, fmt.Sprintf("endpoint %s unreachable", endpoint)
	}
	_ = conn.Close()
	return nestv1.HealthHealthy, fmt.Sprintf("endpoint %s reachable", endpoint)
}
