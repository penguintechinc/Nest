"""Cryptographic utilities for field encryption and API key generation."""
import os
import secrets
import string
from cryptography.fernet import Fernet


def _get_fernet() -> Fernet:
    key = os.environ.get("FIELD_ENCRYPTION_KEY")
    if not key:
        raise RuntimeError("FIELD_ENCRYPTION_KEY environment variable not set")
    return Fernet(key.encode() if isinstance(key, str) else key)


def generate_api_key() -> str:
    """Generate a secure random API key."""
    alphabet = string.ascii_letters + string.digits
    return "nk_" + "".join(secrets.choice(alphabet) for _ in range(48))


def encrypt_field(value: str) -> str:
    """Encrypt a field value using Fernet symmetric encryption."""
    if not value:
        return value
    f = _get_fernet()
    return f.encrypt(value.encode()).decode()


def decrypt_field(encrypted_value: str) -> str:
    """Decrypt a Fernet-encrypted field value."""
    if not encrypted_value:
        return encrypted_value
    f = _get_fernet()
    return f.decrypt(encrypted_value.encode()).decode()
