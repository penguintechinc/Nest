"""EC P-256 key management for JWT signing and verification."""

import logging
import os
from typing import Optional

from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.backends import default_backend

log = logging.getLogger(__name__)


def _mask_secret(value: str, show_chars: int = 4) -> str:
    """Mask a secret string, showing only the last N characters."""
    if len(value) <= show_chars:
        return "*" * len(value)
    return "*" * (len(value) - show_chars) + value[-show_chars:]


def load_or_generate_ec_key() -> tuple[ec.EllipticCurvePrivateKey, ec.EllipticCurvePublicKey]:
    """Load EC P-256 private key from env or generate ephemeral key.

    Supports two env var formats:
    1. JWT_EC_KEY_PATH: path to PEM file with private key
    2. JWT_EC_KEY_PEM: PEM-formatted private key (multiline OK, base64 OK)

    If neither is set, generates an ephemeral P-256 keypair and logs a warning
    (safe for development, insecure for production).

    Returns:
        Tuple of (private_key, public_key) from cryptography library.
    """
    key_path = os.environ.get("JWT_EC_KEY_PATH", "").strip()
    key_pem = os.environ.get("JWT_EC_KEY_PEM", "").strip()

    # Try loading from file path
    if key_path:
        try:
            with open(key_path, "rb") as f:
                private_pem = f.read()
            private_key = serialization.load_pem_private_key(
                private_pem,
                password=None,
                backend=default_backend()
            )
            public_key = private_key.public_key()
            log.info(
                "Loaded EC P-256 private key from %s",
                key_path,
            )
            return private_key, public_key
        except Exception as e:
            log.error(
                "Failed to load EC key from JWT_EC_KEY_PATH=%s: %s",
                key_path,
                e,
            )
            raise RuntimeError(f"Failed to load EC key from {key_path}") from e

    # Try loading from PEM string
    if key_pem:
        try:
            private_pem_bytes = key_pem.encode("utf-8") if isinstance(key_pem, str) else key_pem
            private_key = serialization.load_pem_private_key(
                private_pem_bytes,
                password=None,
                backend=default_backend()
            )
            public_key = private_key.public_key()
            masked = _mask_secret(key_pem[:80] if len(key_pem) > 80 else key_pem)
            log.info(
                "Loaded EC P-256 private key from JWT_EC_KEY_PEM (first 80 chars: %s)",
                masked,
            )
            return private_key, public_key
        except Exception as e:
            log.error(
                "Failed to load EC key from JWT_EC_KEY_PEM: %s",
                e,
            )
            raise RuntimeError("Failed to load EC key from JWT_EC_KEY_PEM") from e

    # No key configured: generate ephemeral (dev only)
    log.warning(
        "No JWT_EC_KEY_PATH or JWT_EC_KEY_PEM set. Generating ephemeral P-256 keypair. "
        "⚠️  THIS IS INSECURE FOR PRODUCTION. Set JWT_EC_KEY_PATH (file) or JWT_EC_KEY_PEM (inline) "
        "to use a persistent key."
    )
    private_key = ec.generate_private_key(ec.SECP256R1(), default_backend())
    public_key = private_key.public_key()
    return private_key, public_key


def get_ec_public_key_pem(public_key: ec.EllipticCurvePublicKey) -> str:
    """Export EC public key as PEM-formatted string."""
    pem_bytes = public_key.public_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PublicFormat.SubjectPublicKeyInfo
    )
    return pem_bytes.decode("utf-8")


def get_ec_private_key_pem(private_key: ec.EllipticCurvePrivateKey) -> str:
    """Export EC private key as PEM-formatted string (no password)."""
    pem_bytes = private_key.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.PKCS8,
        encryption_algorithm=serialization.NoEncryption()
    )
    return pem_bytes.decode("utf-8")


# Global instance (loaded at module import time)
_PRIVATE_KEY: Optional[ec.EllipticCurvePrivateKey] = None
_PUBLIC_KEY: Optional[ec.EllipticCurvePublicKey] = None


def get_manager_ec_keys() -> tuple[ec.EllipticCurvePrivateKey, ec.EllipticCurvePublicKey]:
    """Get the cached manager EC P-256 keypair (singleton).

    Returns:
        Tuple of (private_key, public_key).
    """
    global _PRIVATE_KEY, _PUBLIC_KEY
    if _PRIVATE_KEY is None or _PUBLIC_KEY is None:
        _PRIVATE_KEY, _PUBLIC_KEY = load_or_generate_ec_key()
    return _PRIVATE_KEY, _PUBLIC_KEY
