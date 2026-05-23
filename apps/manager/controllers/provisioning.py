"""
Kubernetes Provisioning Controller

Manages the complete provisioning lifecycle for Kubernetes-based resources including:
- Resource validation and namespace creation
- Credential generation and secret management
- StatefulSet and Service creation from templated manifests
- Status tracking and error handling
- Deprovisioning and scaling operations
"""

import os
import json
import logging
import secrets
import asyncio
import time
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Optional, Any
from dataclasses import dataclass, asdict
import yaml

from jinja2 import Environment, FileSystemLoader, TemplateNotFound, select_autoescape
from cryptography.fernet import Fernet

from penguin_dal.quart_ext import get_db
from lib.k8s_client import K8sClient, K8sException


logger = logging.getLogger(__name__)


@dataclass
class ProvisioningStatus:
    """Data class for provisioning status representation"""
    resource_id: int
    status: str
    namespace: str
    k8s_resource_name: str
    connection_info: Optional[Dict[str, Any]] = None
    error_message: Optional[str] = None
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None


class EncryptionManager:
    """Manages encryption and decryption of sensitive credentials"""

    def __init__(self, key: Optional[str] = None):

        db = get_db()
        """Initialize encryption manager with Fernet key.

        Args:
            key: Base64-encoded encryption key. If None, reads from ENCRYPTION_KEY env var.
        """
        if key is None:
            key = os.getenv('ENCRYPTION_KEY')
            if not key:
                # Generate new key for testing/development
                logger.warning("ENCRYPTION_KEY not set, generating temporary key")
                key = Fernet.generate_key()

        try:
            if isinstance(key, str):
                key = key.encode()
            self.cipher = Fernet(key)
        except Exception as e:
            logger.error(f"Failed to initialize encryption: {e}")
            raise

    def encrypt(self, data: str) -> str:
        """Encrypt a string value.

        Args:
            data: Plain text data to encrypt

        Returns:
            Encrypted data as string
        """
        encrypted = self.cipher.encrypt(data.encode())
        return encrypted.decode()

    def decrypt(self, encrypted_data: str) -> str:
        """Decrypt an encrypted string.

        Args:
            encrypted_data: Encrypted data string

        Returns:
            Decrypted plain text
        """
        decrypted = self.cipher.decrypt(encrypted_data.encode())
        return decrypted.decode()


class CredentialGenerator:
    """Generates random credentials for database resources"""

    @staticmethod
    def generate_password(length: int = 32) -> str:
        """Generate a cryptographically secure random password.

        Args:
            length: Password length in characters

        Returns:
            Random password string
        """
        charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
        return ''.join(secrets.choice(charset) for _ in range(length))

    @staticmethod
    def generate_username(prefix: str = "user", length: int = 8) -> str:
        """Generate a random username.

        Args:
            prefix: Username prefix
            length: Random suffix length

        Returns:
            Generated username
        """
        suffix = ''.join(secrets.choice("abcdefghijklmnopqrstuvwxyz0123456789") for _ in range(length))
        return f"{prefix}_{suffix}"

    @staticmethod
    def generate_api_token(length: int = 32) -> str:
        """Generate a random API token.

        Args:
            length: Token length in characters

        Returns:
            Random token string
        """
        return secrets.token_hex(length // 2)


class TemplateRenderer:
    """Manages Jinja2 template rendering for Kubernetes manifests"""

    def __init__(self, template_dir: Optional[str] = None):

        db = get_db()
        """Initialize template renderer.

        Args:
            template_dir: Path to templates directory. Defaults to ./templates
        """
        if template_dir is None:
            template_dir = os.path.join(os.path.dirname(__file__), '..', 'templates')

        self.template_dir = Path(template_dir)
        self.env = Environment(
            loader=FileSystemLoader(str(self.template_dir)),
            trim_blocks=True,
            lstrip_blocks=True,
            autoescape=select_autoescape(default_for_string=False, default=False)
        )

    def render_template(self, template_name: str, context: Dict[str, Any]) -> str:
        """Render a Jinja2 template with the given context.

        Args:
            template_name: Name of template file (relative to template_dir)
            context: Dictionary of variables for template rendering

        Returns:
            Rendered template string

        Raises:
            TemplateNotFound: If template file doesn't exist
        """
        try:
            template = self.env.get_template(template_name)
            return template.render(**context)
        except TemplateNotFound as e:
            logger.error(f"Template not found: {template_name}")
            raise
        except Exception as e:
            logger.error(f"Template rendering error: {e}")
            raise

    def render_statefulset_template(self, resource_type: str, context: Dict[str, Any]) -> str:
        """Render a StatefulSet template for the given resource type.

        Args:
            resource_type: Resource type (db-postgresql, db-redis, etc.)
            context: Template variables

        Returns:
            Rendered YAML manifest string
        """
        # Map resource types to template names
        type_mapping = {
            'db-postgresql': 'statefulset/postgresql.yaml',
            'db-redis': 'statefulset/redis.yaml',
            'db-mariadb': 'statefulset/mariadb.yaml',
            'db-valkey': 'statefulset/valkey.yaml',
        }

        template_name = type_mapping.get(resource_type)
        if not template_name:
            raise ValueError(f"Unsupported resource type: {resource_type}")

        return self.render_template(template_name, context)


class ProvisioningController:
    """Controller for managing Kubernetes resource provisioning lifecycle"""

    # Supported resource types
    SUPPORTED_RESOURCE_TYPES = [
        'db-postgresql',
        'db-redis',
        'db-mariadb',
        'db-valkey'
    ]

    # Service type mapping
    SERVICE_TYPE_MAPPING = {
        'db-postgresql': 'ClusterIP',
        'db-redis': 'ClusterIP',
        'db-mariadb': 'ClusterIP',
        'db-valkey': 'ClusterIP',
    }

    # Default port mapping
    DEFAULT_PORTS = {
        'db-postgresql': 5432,
        'db-redis': 6379,
        'db-mariadb': 3306,
        'db-valkey': 6379,
    }

    def __init__(self, k8s_client: Optional[K8sClient] = None,
                 template_renderer: Optional[TemplateRenderer] = None,
                 encryption_manager: Optional[EncryptionManager] = None):

        db = get_db()
        """Initialize provisioning controller.

        Args:
            k8s_client: K8sClient instance for Kubernetes operations
            template_renderer: TemplateRenderer for manifest generation
            encryption_manager: EncryptionManager for credential encryption
        """
        self.k8s_client = k8s_client or K8sClient()
        self.template_renderer = template_renderer or TemplateRenderer()
        self.encryption_manager = encryption_manager or EncryptionManager()
        self.credential_generator = CredentialGenerator()

    def provision_resource(self, resource_id: int, created_by_user_id: int) -> ProvisioningStatus:
        """Main provisioning workflow for a resource.

        Steps:
        1. Load resource from database
        2. Validate resource type supports full lifecycle
        3. Create namespace (team-based: team-{team_id})
        4. Generate credentials (random passwords)
        5. Create k8s Secret with credentials
        6. Load appropriate template (postgresql.yaml, redis.yaml, etc.)
        7. Render template with Jinja2 (inject resource config)
        8. Create StatefulSet via k8s_client
        9. Create Service (ClusterIP or LoadBalancer based on config)
        10. Wait for StatefulSet to be ready
        11. Extract connection info (service endpoint, port)
        12. Update resource in database with status, credentials, connection info
        13. Create provisioning_jobs record

        Args:
            resource_id: ID of resource to provision
            created_by_user_id: ID of user initiating provisioning

        Returns:
            ProvisioningStatus with final status and connection info

        Raises:
            ValueError: If resource not found or invalid type
            K8sException: If Kubernetes operation fails
        """
        job_record = None

        try:
            # Step 1: Load resource from database
            resource = db.resources[resource_id]
            if not resource:
                raise ValueError(f"Resource not found: {resource_id}")

            logger.info(f"Starting provisioning for resource: {resource.name} (ID: {resource_id})")

            # Get resource type info
            resource_type = db.resource_types[resource.resource_type_id]
            resource_type_name = resource_type.name

            # Step 2: Validate resource type supports full lifecycle
            if not resource_type.supports_full_lifecycle:
                raise ValueError(
                    f"Resource type '{resource_type_name}' does not support full lifecycle provisioning"
                )

            if resource_type_name not in self.SUPPORTED_RESOURCE_TYPES:
                raise ValueError(
                    f"Resource type '{resource_type_name}' is not supported by provisioning controller"
                )

            # Get team information
            team = db.teams[resource.team_id]

            # Step 3: Create namespace (team-based)
            namespace = f"team-{team.id}"
            logger.info(f"Creating namespace: {namespace}")
            self.k8s_client.create_namespace(namespace)

            # Step 4: Generate credentials
            credentials = self._generate_resource_credentials(resource_type_name)
            logger.info(f"Generated credentials for resource type: {resource_type_name}")

            # Step 5: Create Kubernetes Secret with credentials
            secret_name = f"{resource.name}-secret"
            secret_data = {
                key: str(value) for key, value in credentials.items()
            }
            logger.info(f"Creating secret: {secret_name}")
            self.k8s_client.create_secret(namespace, secret_name, secret_data)

            # Step 6-7: Load and render template
            template_context = self._build_template_context(
                resource=resource,
                resource_type_name=resource_type_name,
                namespace=namespace,
                credentials=credentials,
                secret_name=secret_name
            )

            logger.info(f"Rendering template for resource type: {resource_type_name}")
            manifest_yaml = self.template_renderer.render_statefulset_template(
                resource_type_name,
                template_context
            )

            # Parse YAML to separate Service and StatefulSet
            manifests = yaml.safe_load_all(manifest_yaml)
            service_manifest = None
            statefulset_manifest = None

            for manifest in manifests:
                if manifest['kind'] == 'Service':
                    service_manifest = manifest
                elif manifest['kind'] == 'StatefulSet':
                    statefulset_manifest = manifest

            if not statefulset_manifest:
                raise ValueError(f"No StatefulSet found in template for {resource_type_name}")

            # Step 8: Create StatefulSet
            k8s_resource_name = statefulset_manifest['metadata']['name']
            logger.info(f"Creating StatefulSet: {k8s_resource_name}")
            self.k8s_client.create_statefulset(namespace, statefulset_manifest)

            # Step 9: Create Service
            if service_manifest:
                service_name = service_manifest['metadata']['name']
                logger.info(f"Creating Service: {service_name}")
                self.k8s_client.create_service(namespace, service_manifest)

            # Step 10: Wait for StatefulSet to be ready
            logger.info(f"Waiting for StatefulSet {k8s_resource_name} to be ready...")
            max_wait_time = 300  # 5 minutes
            ready = self._wait_for_statefulset_ready(
                namespace,
                k8s_resource_name,
                max_wait_time
            )

            if not ready:
                raise RuntimeError(
                    f"StatefulSet {k8s_resource_name} failed to become ready within {max_wait_time} seconds"
                )

            logger.info(f"StatefulSet {k8s_resource_name} is ready")

            # Step 11: Extract connection info
            service_endpoint = self._get_service_endpoint(namespace, service_name)
            port = self.DEFAULT_PORTS.get(resource_type_name, 5432)

            connection_info = {
                'host': service_endpoint,
                'port': port,
                'namespace': namespace,
                'service_name': service_name or k8s_resource_name,
                'protocol': 'tcp'
            }

            logger.info(f"Connection info: {connection_info}")

            # Step 12: Update resource in database
            encrypted_credentials = json.dumps({
                key: self.encryption_manager.encrypt(str(value))
                for key, value in credentials.items()
            })

            db.resources.update_or_insert(
                db.resources.id == resource_id,
                status='active',
                k8s_namespace=namespace,
                k8s_resource_name=k8s_resource_name,
                k8s_resource_type='StatefulSet',
                connection_info=json.dumps(connection_info),
                credentials=encrypted_credentials,
                updated_at=datetime.now()
            )
            db.commit()

            logger.info(f"Resource {resource_id} updated in database")

            # Step 13: Create provisioning_jobs record
            job_record = db.provisioning_jobs.insert(
                resource_id=resource_id,
                job_type='provision',
                status='completed',
                started_at=datetime.now(),
                completed_at=datetime.now(),
                logs=f"Successfully provisioned resource {resource.name}",
                created_by=created_by_user_id
            )
            db.commit()

            logger.info(f"Provisioning completed for resource {resource_id}")

            return ProvisioningStatus(
                resource_id=resource_id,
                status='active',
                namespace=namespace,
                k8s_resource_name=k8s_resource_name,
                connection_info=connection_info,
                created_at=resource.created_at,
                updated_at=datetime.now()
            )

        except Exception as e:
            error_msg = f"Provisioning failed: {str(e)}"
            logger.error(error_msg, exc_info=True)

            # Update resource status to error
            try:
                db.resources.update_or_insert(
                    db.resources.id == resource_id,
                    status='error',
                    updated_at=datetime.now()
                )
                db.commit()
            except Exception as db_error:
                logger.error(f"Failed to update resource status: {db_error}")

            # Create failed provisioning job record
            try:
                db.provisioning_jobs.insert(
                    resource_id=resource_id,
                    job_type='provision',
                    status='failed',
                    started_at=datetime.now(),
                    completed_at=datetime.now(),
                    error_message=error_msg,
                    logs=error_msg,
                    created_by=created_by_user_id
                )
                db.commit()
            except Exception as job_error:
                logger.error(f"Failed to create job record: {job_error}")

            # Attempt rollback of Kubernetes resources
            try:
                resource = db.resources[resource_id]
                if resource and resource.k8s_namespace:
                    logger.info(f"Attempting rollback for namespace: {resource.k8s_namespace}")
                    self._rollback_provisioning(
                        resource.k8s_namespace,
                        resource.k8s_resource_name if resource.k8s_resource_name else None
                    )
            except Exception as rollback_error:
                logger.error(f"Rollback failed: {rollback_error}")

            raise

    def deprovision_resource(self, resource_id: int, created_by_user_id: int) -> ProvisioningStatus:
        """Deprovisioning workflow for a resource.

        Removes all Kubernetes resources associated with a provisioned resource and
        updates database status.

        Args:
            resource_id: ID of resource to deprovision
            created_by_user_id: ID of user initiating deprovisioning

        Returns:
            ProvisioningStatus with final status

        Raises:
            ValueError: If resource not found
            K8sException: If Kubernetes operation fails
        """
        try:
            resource = db.resources[resource_id]
            if not resource:
                raise ValueError(f"Resource not found: {resource_id}")

            if not resource.k8s_namespace:
                raise ValueError(f"Resource {resource_id} has no Kubernetes namespace associated")

            logger.info(f"Starting deprovisioning for resource: {resource.name} (ID: {resource_id})")

            # Delete Kubernetes namespace (cascades to all resources within)
            logger.info(f"Deleting namespace: {resource.k8s_namespace}")
            self.k8s_client.delete_namespace(resource.k8s_namespace)

            # Wait for namespace deletion
            self._wait_for_namespace_deletion(resource.k8s_namespace, timeout=60)

            # Update resource status
            db.resources.update_or_insert(
                db.resources.id == resource_id,
                status='deleted',
                k8s_namespace=None,
                k8s_resource_name=None,
                updated_at=datetime.now()
            )
            db.commit()

            # Create deprovisioning job record
            db.provisioning_jobs.insert(
                resource_id=resource_id,
                job_type='deprovision',
                status='completed',
                started_at=datetime.now(),
                completed_at=datetime.now(),
                logs=f"Successfully deprovisioned resource {resource.name}",
                created_by=created_by_user_id
            )
            db.commit()

            logger.info(f"Deprovisioning completed for resource {resource_id}")

            return ProvisioningStatus(
                resource_id=resource_id,
                status='deleted',
                namespace=None,
                k8s_resource_name=None,
                created_at=resource.created_at,
                updated_at=datetime.now()
            )

        except Exception as e:
            error_msg = f"Deprovisioning failed: {str(e)}"
            logger.error(error_msg, exc_info=True)

            # Create failed job record
            try:
                db.provisioning_jobs.insert(
                    resource_id=resource_id,
                    job_type='deprovision',
                    status='failed',
                    started_at=datetime.now(),
                    completed_at=datetime.now(),
                    error_message=error_msg,
                    logs=error_msg,
                    created_by=created_by_user_id
                )
                db.commit()
            except Exception as job_error:
                logger.error(f"Failed to create job record: {job_error}")

            raise

    def scale_resource(self, resource_id: int, replicas: int,
                       created_by_user_id: int) -> ProvisioningStatus:
        """Scale a StatefulSet resource to the specified number of replicas.

        Args:
            resource_id: ID of resource to scale
            replicas: Number of replicas to scale to
            created_by_user_id: ID of user initiating scaling

        Returns:
            ProvisioningStatus with updated replica count

        Raises:
            ValueError: If resource not found or invalid replica count
            K8sException: If Kubernetes operation fails
        """
        if replicas < 1:
            raise ValueError(f"Invalid replica count: {replicas}. Must be at least 1")

        try:
            resource = db.resources[resource_id]
            if not resource:
                raise ValueError(f"Resource not found: {resource_id}")

            if not resource.k8s_namespace or not resource.k8s_resource_name:
                raise ValueError(f"Resource {resource_id} is not provisioned in Kubernetes")

            if not resource.can_scale:
                raise ValueError(f"Resource {resource_id} does not support scaling")

            logger.info(
                f"Scaling resource {resource.name} (ID: {resource_id}) to {replicas} replicas"
            )

            # Scale StatefulSet
            self.k8s_client.scale_statefulset(
                resource.k8s_namespace,
                resource.k8s_resource_name,
                replicas
            )

            # Wait for new replicas to be ready
            self._wait_for_statefulset_replicas(
                resource.k8s_namespace,
                resource.k8s_resource_name,
                replicas,
                timeout=300
            )

            # Update resource config
            config = resource.config or {}
            config['replicas'] = replicas

            db.resources.update_or_insert(
                db.resources.id == resource_id,
                config=json.dumps(config),
                updated_at=datetime.now()
            )
            db.commit()

            # Create scaling job record
            db.provisioning_jobs.insert(
                resource_id=resource_id,
                job_type='scale',
                status='completed',
                started_at=datetime.now(),
                completed_at=datetime.now(),
                logs=f"Scaled resource to {replicas} replicas",
                created_by=created_by_user_id
            )
            db.commit()

            logger.info(f"Resource {resource_id} scaled to {replicas} replicas")

            return ProvisioningStatus(
                resource_id=resource_id,
                status=resource.status,
                namespace=resource.k8s_namespace,
                k8s_resource_name=resource.k8s_resource_name,
                created_at=resource.created_at,
                updated_at=datetime.now()
            )

        except Exception as e:
            error_msg = f"Scaling failed: {str(e)}"
            logger.error(error_msg, exc_info=True)

            try:
                db.provisioning_jobs.insert(
                    resource_id=resource_id,
                    job_type='scale',
                    status='failed',
                    started_at=datetime.now(),
                    completed_at=datetime.now(),
                    error_message=error_msg,
                    logs=error_msg,
                    created_by=created_by_user_id
                )
                db.commit()
            except Exception as job_error:
                logger.error(f"Failed to create job record: {job_error}")

            raise

    def update_resource_config(self, resource_id: int, config: Dict[str, Any],
                               created_by_user_id: int) -> ProvisioningStatus:
        """Update resource configuration.

        Updates resource-specific configuration and triggers reconciliation if needed.

        Args:
            resource_id: ID of resource to update
            config: New configuration dictionary
            created_by_user_id: ID of user initiating update

        Returns:
            ProvisioningStatus with updated status

        Raises:
            ValueError: If resource not found or update not allowed
        """
        try:
            resource = db.resources[resource_id]
            if not resource:
                raise ValueError(f"Resource not found: {resource_id}")

            if not resource.can_modify_config:
                raise ValueError(f"Resource {resource_id} does not allow configuration modification")

            logger.info(f"Updating configuration for resource {resource_id}")

            # Merge with existing config
            existing_config = resource.config or {}
            if isinstance(existing_config, str):
                existing_config = json.loads(existing_config)

            updated_config = {**existing_config, **config}

            # Update resource
            db.resources.update_or_insert(
                db.resources.id == resource_id,
                config=json.dumps(updated_config),
                status='updating',
                updated_at=datetime.now()
            )
            db.commit()

            # Create update job record
            db.provisioning_jobs.insert(
                resource_id=resource_id,
                job_type='update_config',
                status='completed',
                started_at=datetime.now(),
                completed_at=datetime.now(),
                logs=f"Updated resource configuration",
                created_by=created_by_user_id
            )
            db.commit()

            # Mark as active again
            db.resources.update_or_insert(
                db.resources.id == resource_id,
                status='active',
                updated_at=datetime.now()
            )
            db.commit()

            logger.info(f"Resource {resource_id} configuration updated")

            return ProvisioningStatus(
                resource_id=resource_id,
                status='active',
                namespace=resource.k8s_namespace,
                k8s_resource_name=resource.k8s_resource_name,
                created_at=resource.created_at,
                updated_at=datetime.now()
            )

        except Exception as e:
            error_msg = f"Configuration update failed: {str(e)}"
            logger.error(error_msg, exc_info=True)

            try:
                db.resources.update_or_insert(
                    db.resources.id == resource_id,
                    status='error',
                    updated_at=datetime.now()
                )
                db.provisioning_jobs.insert(
                    resource_id=resource_id,
                    job_type='update_config',
                    status='failed',
                    started_at=datetime.now(),
                    completed_at=datetime.now(),
                    error_message=error_msg,
                    logs=error_msg,
                    created_by=created_by_user_id
                )
                db.commit()
            except Exception as db_error:
                logger.error(f"Failed to update status: {db_error}")

            raise

    def get_provisioning_status(self, resource_id: int) -> ProvisioningStatus:
        """Get current provisioning status for a resource.

        Args:
            resource_id: ID of resource to get status for

        Returns:
            ProvisioningStatus with current status information

        Raises:
            ValueError: If resource not found
        """
        resource = db.resources[resource_id]
        if not resource:
            raise ValueError(f"Resource not found: {resource_id}")

        connection_info = None
        if resource.connection_info:
            if isinstance(resource.connection_info, str):
                connection_info = json.loads(resource.connection_info)
            else:
                connection_info = resource.connection_info

        return ProvisioningStatus(
            resource_id=resource_id,
            status=resource.status,
            namespace=resource.k8s_namespace or '',
            k8s_resource_name=resource.k8s_resource_name or '',
            connection_info=connection_info,
            created_at=resource.created_at,
            updated_at=resource.updated_at
        )

    # Private helper methods

    def _generate_resource_credentials(self, resource_type: str) -> Dict[str, str]:

        db = get_db()
        """Generate credentials for a resource type.

        Args:
            resource_type: Type of resource (db-postgresql, db-redis, etc.)

        Returns:
            Dictionary of generated credentials
        """
        if resource_type == 'db-postgresql':
            return {
                'username': self.credential_generator.generate_username('postgres'),
                'password': self.credential_generator.generate_password(),
                'database': self.credential_generator.generate_username('db')
            }
        elif resource_type in ['db-redis', 'db-valkey']:
            return {
                'password': self.credential_generator.generate_password()
            }
        elif resource_type == 'db-mariadb':
            return {
                'username': self.credential_generator.generate_username('maria'),
                'password': self.credential_generator.generate_password(),
                'root_password': self.credential_generator.generate_password(),
                'database': self.credential_generator.generate_username('db')
            }
        else:
            raise ValueError(f"Unsupported resource type: {resource_type}")

    def _build_template_context(self, resource: Any, resource_type_name: str,
                                namespace: str, credentials: Dict[str, str],
                                secret_name: str) -> Dict[str, Any]:

        db = get_db()
        """Build template context for Jinja2 rendering.

        Args:
            resource: Resource record from database
            resource_type_name: Type of resource
            namespace: Kubernetes namespace
            credentials: Generated credentials
            secret_name: Name of Kubernetes secret

        Returns:
            Context dictionary for template rendering
        """
        # Extract resource type prefix
        resource_prefix = resource_type_name.split('-')[1]  # e.g., 'postgresql' from 'db-postgresql'

        context = {
            'namespace': namespace,
            f'{resource_prefix}_name': resource.name,
            f'{resource_prefix}_secret_name': secret_name,
            f'{resource_prefix}_replicas': 1,
            'storage_class': 'standard',
            f'{resource_prefix}_storage_size': '10Gi',
        }

        # Add credentials to context
        for key, value in credentials.items():
            context[f'{resource_prefix}_{key}'] = value

        # Add resource config if present
        if resource.config:
            config = resource.config
            if isinstance(config, str):
                config = json.loads(config)
            context.update(config)

        return context

    def _wait_for_statefulset_ready(self, namespace: str, name: str,
                                    timeout: int = 300) -> bool:

        db = get_db()
        """Wait for a StatefulSet to become ready.

        Args:
            namespace: Kubernetes namespace
            name: StatefulSet name
            timeout: Maximum wait time in seconds

        Returns:
            True if ready, False if timeout
        """
        start_time = time.time()

        while time.time() - start_time < timeout:
            try:
                statefulset = self.k8s_client.get_statefulset(namespace, name)

                # Check if ready replicas equals desired replicas
                status = statefulset.get('status', {})
                desired_replicas = status.get('replicas', 0)
                ready_replicas = status.get('readyReplicas', 0)

                if desired_replicas > 0 and ready_replicas >= desired_replicas:
                    logger.info(f"StatefulSet {name} is ready with {ready_replicas} replicas")
                    return True

                logger.debug(
                    f"StatefulSet {name} status: {ready_replicas}/{desired_replicas} ready"
                )

            except Exception as e:
                logger.warning(f"Error checking StatefulSet status: {e}")

            time.sleep(5)

        logger.error(f"StatefulSet {name} did not become ready within {timeout} seconds")
        return False

    def _wait_for_statefulset_replicas(self, namespace: str, name: str,
                                       replicas: int, timeout: int = 300) -> bool:

        db = get_db()
        """Wait for a StatefulSet to have the desired number of replicas ready.

        Args:
            namespace: Kubernetes namespace
            name: StatefulSet name
            replicas: Desired number of replicas
            timeout: Maximum wait time in seconds

        Returns:
            True if all replicas ready, False if timeout
        """
        start_time = time.time()

        while time.time() - start_time < timeout:
            try:
                statefulset = self.k8s_client.get_statefulset(namespace, name)
                status = statefulset.get('status', {})
                ready_replicas = status.get('readyReplicas', 0)

                if ready_replicas >= replicas:
                    logger.info(f"StatefulSet {name} has {ready_replicas} replicas ready")
                    return True

                logger.debug(
                    f"StatefulSet {name} replicas status: {ready_replicas}/{replicas} ready"
                )

            except Exception as e:
                logger.warning(f"Error checking StatefulSet replicas: {e}")

            time.sleep(5)

        logger.error(f"StatefulSet {name} failed to reach {replicas} replicas within {timeout} seconds")
        return False

    def _wait_for_namespace_deletion(self, namespace: str, timeout: int = 60) -> bool:

        db = get_db()
        """Wait for a namespace to be deleted.

        Args:
            namespace: Kubernetes namespace name
            timeout: Maximum wait time in seconds

        Returns:
            True if deleted, False if timeout
        """
        start_time = time.time()

        while time.time() - start_time < timeout:
            try:
                self.k8s_client.get_namespace(namespace)
                logger.debug(f"Namespace {namespace} still exists, waiting...")
            except Exception:
                # Namespace doesn't exist anymore
                logger.info(f"Namespace {namespace} deleted successfully")
                return True

            time.sleep(2)

        logger.error(f"Namespace {namespace} was not deleted within {timeout} seconds")
        return False

    def _get_service_endpoint(self, namespace: str, service_name: str) -> str:

        db = get_db()
        """Get the DNS endpoint for a Kubernetes service.

        Args:
            namespace: Kubernetes namespace
            service_name: Service name

        Returns:
            Service endpoint (FQDN or IP)
        """
        try:
            service = self.k8s_client.get_service(namespace, service_name)
            # Kubernetes DNS: service_name.namespace.svc.cluster.local
            return f"{service_name}.{namespace}.svc.cluster.local"
        except Exception as e:
            logger.warning(f"Could not retrieve service endpoint: {e}")
            # Return fallback endpoint
            return f"{service_name}.{namespace}.svc.cluster.local"

    def _rollback_provisioning(self, namespace: Optional[str],
                               statefulset_name: Optional[str]) -> None:

        db = get_db()
        """Rollback provisioning by cleaning up Kubernetes resources.

        Args:
            namespace: Kubernetes namespace to clean up
            statefulset_name: StatefulSet name to clean up (optional)
        """
        try:
            if namespace:
                logger.info(f"Deleting namespace {namespace} for rollback")
                try:
                    self.k8s_client.delete_namespace(namespace)
                except Exception as e:
                    logger.error(f"Failed to delete namespace {namespace}: {e}")

            if statefulset_name and namespace:
                logger.info(f"Deleting StatefulSet {statefulset_name} for rollback")
                try:
                    self.k8s_client.delete_statefulset(namespace, statefulset_name)
                except Exception as e:
                    logger.error(f"Failed to delete StatefulSet {statefulset_name}: {e}")

        except Exception as e:
            logger.error(f"Rollback cleanup failed: {e}")
