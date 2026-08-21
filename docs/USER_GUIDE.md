# NEST User Guide

NEST is a cloud-native database infrastructure management platform. It manages database servers, databases, SQL files, security rules, threat intelligence feeds, cloud providers, scaling policies, temporary access grants, and teams across PostgreSQL, MariaDB, Redis, Valkey, Ceph, and SAN resources.

---

## Getting Started

### Logging In

Navigate to the NEST login page and enter your email and password. After successful authentication, you will be redirected to the Dashboard.

Default admin credentials for development:

- Email: `admin@localhost.local`
- Password: `admin123`

Your session is maintained via a JWT token stored in the browser. If your session expires, you will be redirected to the login page automatically. The token has a default lifetime of 1 hour.

### Dashboard Overview

The Dashboard provides a high-level view of your infrastructure:

- **Resource Statistics** (left panel): Aggregated counts and health metrics across all managed resources, including risk assessment levels (Critical, High, Medium, Low).
- **Resource List** (right panel): A compact view of all resources with lifecycle color coding:
  - **Blue**: Full lifecycle -- provisioned and managed by NEST via Kubernetes StatefulSets.
  - **Orange**: Partial lifecycle -- externally hosted, managed via connectors.
  - **Gray**: Monitor only -- read-only observation, no changes made.
- **Quick Actions**: Buttons for creating resources, viewing all resources, and managing teams.

### Navigation

The sidebar organizes features into four categories:

1. **Infrastructure**: Dashboard, Servers, Databases, Resources
2. **Operations**: SQL Files, Scaling Policies, Cloud Providers
3. **Security**: Security Rules, Threat Intelligence, Blocked Databases, Temporary Access
4. **Administration**: Teams

---

## Managing Database Servers

Navigate to **Infrastructure > Servers** to view and manage database server instances.

### Server List

The server table displays:

- **Name**: Server identifier
- **Host**: Hostname or IP address
- **Type**: Database engine (PostgreSQL, MariaDB, Redis, etc.)
- **Status**: Current state with color-coded badges
  - `active` (green): Server is healthy and accepting connections
  - `maintenance` (amber): Server is undergoing maintenance
  - `error` (red): Server has connectivity or health issues
- **Connections**: Current active connection count
- **Actions**: Edit and delete controls

### Adding a Server

Click **Add Server** to register a new database server. Provide the connection details including hostname, port, engine type, and credentials. NEST will verify connectivity before saving.

### Monitoring Servers

Server health is continuously monitored by the `db_health_checker` background worker. Connection counts, replication status, and disk usage are collected periodically and displayed on the server detail view.

---

## Managing Databases

Navigate to **Infrastructure > Databases** to manage individual database instances.

### Database States

Each database has a lifecycle status:

- `active` (green): Database is online and operational
- `suspended` (amber): Database is temporarily paused (data preserved)
- `archived` (gray): Database is archived for long-term storage

### Creating a Database

Click **Create Database** to provision a new database. Select the target server, database name, character set, and initial size allocation. For full-lifecycle resources, NEST provisions the database via Kubernetes StatefulSets.

### Database Size

Sizes are displayed in human-readable format (MB or GB). Monitor growth trends to plan capacity.

---

## Resource Types and Lifecycle Modes

NEST supports multiple resource types across three categories:

| Category     | Types                                             |
| ------------ | ------------------------------------------------- |
| **Database** | PostgreSQL, MariaDB, MySQL, Redis, Valkey         |
| **Storage**  | Ceph, SAN                                         |
| **BigData**  | Additional resource types for analytics workloads |

### Resource Lifecycle Modes

- **Full Lifecycle**: NEST provisions and manages the resource end-to-end via Kubernetes StatefulSets. Supported for PostgreSQL, Redis, MariaDB, and Valkey.
- **Partial Lifecycle**: Resource is hosted externally; NEST manages user sync, configuration, and backups via connectors. Supported for PostgreSQL, MariaDB, Redis, Ceph, and SAN.
- **Monitor Only**: NEST collects metrics and health data without making changes to the resource.

### Creating Resources

Use the Resource Wizard (accessible from Dashboard > Create Resource or **Infrastructure > Resources**) to create new resources. The wizard provides a visual mode selection interface for choosing the lifecycle mode and resource type.

---

## SQL File Management

Navigate to **Operations > SQL Files** to upload, review, and execute SQL scripts against managed databases.

### SQL File Workflow

1. **Upload**: Click **Upload SQL** to submit a new SQL file with a name and description.
2. **Review**: Uploaded files enter `pending` status for review. Reviewers can approve or reject.
3. **Execute**: Approved files can be executed against a target database.
4. **Track**: Execution results are recorded with status tracking.

### Status Indicators

- `pending` (amber): Awaiting review
- `approved` (blue): Reviewed and approved for execution
- `rejected` (red): Rejected by a reviewer
- `executed` (green): Successfully executed

---

## Security Rules Configuration

Navigate to **Security > Security Rules** to manage firewall and access control rules.

### Rule Properties

Each rule has:

- **Name**: Descriptive identifier
- **Type**: Rule category (e.g., firewall, ACL, rate-limit)
- **Action**: What happens when the rule matches (allow, deny, alert)
- **Priority**: Numeric priority (lower numbers evaluate first)
- **Enabled**: Toggle to activate or deactivate the rule

### Adding Rules

Click **Add Rule** to create a new security rule. Define the match criteria, action, and priority. Rules are evaluated in priority order against incoming connections and queries.

---

## Threat Intelligence Monitoring

Navigate to **Security > Threat Intelligence** to monitor threat indicators and alerts.

### Threat Feed

The threat intelligence view shows entries collected by the `threat_intel_poller` background worker from configured threat feeds. Each entry includes:

- **Source**: Origin feed or provider
- **Type**: Indicator type (IP, domain, hash, etc.)
- **Indicator**: The specific threat indicator value
- **Severity**: Risk level with color coding
  - `critical` (red): Immediate action required
  - `high` (orange): High-risk indicator
  - `medium` (amber): Moderate risk
  - `low` (gray): Low-risk or informational
- **First Seen**: When the indicator was first detected
- **Active**: Whether the threat is currently active

### Blocked Databases

Navigate to **Security > Blocked Databases** to view databases that have been blocked due to security policy violations or threat detections.

---

## Cloud Provider Integration

Navigate to **Operations > Cloud Providers** to manage connections to external cloud platforms.

### Provider Management

Register cloud providers (AWS, GCP, Azure, DigitalOcean, Vultr, Linode) to enable NEST to provision resources directly in cloud environments. Each provider entry tracks:

- **Name**: Provider identifier
- **Type**: Cloud platform type (AWS, GCP, Azure, etc.)
- **Region**: Deployment region
- **Status**: Connection state
  - `connected` (green): Active and authenticated
  - `disconnected` (gray): Not currently connected
  - `error` (red): Authentication or connectivity failure

Click **Add Provider** to register a new cloud provider with API credentials and region configuration.

### Relationship to Resource Lifecycle Modes

Cloud providers enable two resource lifecycle modes for DataResources:

**External Mode (Provisioning):**
When you create a DataResource with a cloud provider reference, NEST provisions the resource directly in that cloud. Supported provisioning:

- AWS: EBS block volumes and S3 buckets
- GCP: Persistent Disk and GCS buckets
- Azure: Managed Disk and Blob containers
- DigitalOcean, Vultr, Linode: Block volumes (token-auth REST API)

See the **Resource Types and Lifecycle Modes** section (lines 89-108) for details on full-lifecycle and partial-lifecycle resources.

**Imported Mode (Adoption):**
NEST can also adopt existing databases and storage systems running in cloud environments (RDS, Cloud SQL, Azure Database, etc.) without provisioning them. This is separate from the cloud provider configuration—an adopted resource uses a simple connection string to reach an existing endpoint. NEST periodically probes the endpoint (every 60 seconds) to monitor health and availability.

### Cloud Provider Examples

**Example: Provision a new AWS EBS volume**

1. Register AWS credentials in **Operations > Cloud Providers**
2. Create a DataResource and select AWS as the external provider
3. NEST provisions the EBS volume and mounts it for your workload

**Example: Adopt an existing RDS database**

1. No cloud provider configuration needed for adoption
2. Create a DataResource with `mode: imported` and provide the RDS connection string
3. NEST connects to the database, monitors its health, and enables backup/restore operations without mutating the database

---

## Scaling Policies

Navigate to **Operations > Scaling Policies** to configure auto-scaling rules for managed resources.

### Policy Configuration

Each scaling policy defines:

- **Resource target**: Which database or server to scale
- **Trigger conditions**: CPU, memory, connection count, or custom metric thresholds
- **Scale actions**: Scale up/down parameters (min/max instances, step size)
- **Status**: Policy state
  - `active` (green): Policy is evaluating and scaling
  - `inactive` (gray): Policy is disabled
  - `scaling` (blue): A scaling operation is in progress
  - `error` (red): Scaling failed

Click **Create Policy** to define a new auto-scaling policy. The `scaling_evaluator` background worker continuously evaluates active policies.

---

## Temporary Access Grants

Navigate to **Security > Temporary Access** to manage time-limited database access for users.

### Granting Access

Click **Grant Access** to create a temporary access grant. Specify:

- **User**: The recipient of the access
- **Database**: Target database
- **Permission**: Access level (read-only, read-write, admin)
- **Reason**: Justification for the access
- **Duration**: How long the access should last

### Access States

- `active` (green): Access is currently valid
- `expired` (gray): Access duration has elapsed
- `revoked` (red): Access was manually revoked before expiration

Grants include timestamps for when access was granted and when it expires. Expired grants are automatically cleaned up.

---

## Team Management

Navigate to **Administration > Teams** to manage teams and their members.

### Team Overview

Each team displays:

- **Name and description**
- **Member count**
- **Creation and update timestamps**

### Team Actions

Select a team to view its detail page with:

- **Manage Members**: Add or remove team members and assign team roles
- **Edit Team**: Update team name and description
- **Delete Team**: Remove the team (requires Admin role)

### Team Roles

Within each team, members can hold one of these roles:

- **Owner**: Full team control including deletion
- **Admin**: Manage members and team settings
- **Member**: Standard access to team resources
- **Viewer**: Read-only access to team resources

---

## Global Roles (RBAC)

NEST uses role-based access control at the global level:

| Role           | Permissions                                                                          |
| -------------- | ------------------------------------------------------------------------------------ |
| **Admin**      | Full system access: manage users, teams, resources, security rules, and all settings |
| **Maintainer** | Read and write access to resources but cannot manage users or system settings        |
| **Viewer**     | Read-only access to all resources and dashboards                                     |

Global roles determine what actions are available in the UI. Admin-only features (such as user management and system configuration) are hidden for non-admin users.

---

## Monitoring & Stats

Navigate to the Dashboard or individual resource detail pages to view monitoring data.

### Resource Statistics

The Dashboard left panel aggregates key metrics across all managed resources:

- **Total Resources**: Count by type (database, storage, BigData)
- **Health Summary**: Healthy, degraded, and offline counts
- **Risk Assessment**: Resources categorized by risk level (Critical, High, Medium, Low) based on security posture, patch status, and configuration drift

### Prometheus Metrics

NEST exposes Prometheus-compatible metrics at `/metrics` on the backend API. Key metrics include:

- `nest_resource_health_status` — gauge per resource (1 = healthy, 0 = unhealthy)
- `nest_db_connections_active` — active connection count per server
- `nest_db_replication_lag_seconds` — replication lag for replicated databases
- `nest_backup_last_success_timestamp` — epoch of last successful backup per resource
- `nest_scaling_events_total` — counter of scaling actions

### Grafana Dashboards

Connect Grafana to the NEST Prometheus endpoint to visualize:

- Resource health over time
- Connection pool utilization
- Backup success/failure trends
- Scaling event history
- Disk and memory usage per database server

---

## Backups

NEST manages automated backups for Full and Partial lifecycle resources.

### Backup Schedules

Configure backup schedules per resource on the resource detail page:

- **Daily**: Runs once per day at a configured time (default: 02:00 UTC)
- **Weekly**: Runs once per week on a configured day
- **Monthly**: Runs on the first day of each month

### Retention Policies

Each backup schedule has a retention policy:

- **Daily backups**: Retained for 7 days (default)
- **Weekly backups**: Retained for 4 weeks (default)
- **Monthly backups**: Retained for 12 months (default)

Retention periods are configurable per resource. Expired backups are automatically purged.

### Restore

To restore from a backup:

1. Navigate to the resource detail page
2. Open the **Backups** tab
3. Select the backup point you want to restore from
4. Click **Restore** and confirm

For Full lifecycle resources, NEST handles the restore operation end-to-end. For Partial lifecycle resources, NEST initiates the restore via the configured connector and reports status.

### Backup Status Indicators

- `completed` (green): Backup finished successfully
- `in_progress` (blue): Backup is currently running
- `failed` (red): Backup failed — check logs for details
- `expired` (gray): Backup has been purged per retention policy

---

## TLS Certificates

Navigate to the resource detail page to view TLS certificate status for database connections.

### Certificate Status

Each resource using TLS displays:

- **Issuer**: Certificate authority that issued the cert
- **Expiry Date**: When the certificate expires
- **Status**:
  - `valid` (green): Certificate is valid and not expiring soon
  - `expiring_soon` (amber): Certificate expires within 30 days — rotation recommended
  - `expired` (red): Certificate has expired — immediate rotation required

### Certificate Rotation

For Full lifecycle resources, NEST can rotate TLS certificates automatically:

1. Navigate to the resource detail page
2. Open the **Security** tab
3. Click **Rotate Certificate**
4. NEST generates a new certificate, applies it, and restarts connections gracefully

For Partial and Monitor lifecycle resources, certificate rotation must be performed externally. NEST will continue to monitor and report certificate expiry status.
