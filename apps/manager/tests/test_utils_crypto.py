"""Tests for utils/crypto.py — generate_api_key, encrypt_field, decrypt_field."""
import os
import sys
import pytest
from cryptography.fernet import Fernet

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

# Provide a valid Fernet key for all tests
VALID_KEY = Fernet.generate_key().decode()
os.environ["FIELD_ENCRYPTION_KEY"] = VALID_KEY


# ---------------------------------------------------------------------------
# generate_api_key
# ---------------------------------------------------------------------------

def test_generate_api_key_prefix():
    from utils.crypto import generate_api_key
    key = generate_api_key()
    assert key.startswith("nk_")


def test_generate_api_key_length():
    from utils.crypto import generate_api_key
    key = generate_api_key()
    # "nk_" + 48 chars = 51 total
    assert len(key) == 51


def test_generate_api_key_uniqueness():
    from utils.crypto import generate_api_key
    keys = {generate_api_key() for _ in range(20)}
    assert len(keys) == 20, "All generated keys should be unique"


def test_generate_api_key_alphanumeric():
    import string
    from utils.crypto import generate_api_key
    key = generate_api_key()[3:]  # strip "nk_"
    valid = set(string.ascii_letters + string.digits)
    assert all(c in valid for c in key)


# ---------------------------------------------------------------------------
# encrypt_field / decrypt_field
# ---------------------------------------------------------------------------

def test_encrypt_returns_string():
    from utils.crypto import encrypt_field
    result = encrypt_field("hello")
    assert isinstance(result, str)
    assert result != "hello"


def test_encrypt_decrypt_roundtrip():
    from utils.crypto import encrypt_field, decrypt_field
    original = "super-secret-value"
    encrypted = encrypt_field(original)
    assert encrypted != original
    decrypted = decrypt_field(encrypted)
    assert decrypted == original


def test_encrypt_empty_string_passthrough():
    """Empty string should be returned as-is (no encryption)."""
    from utils.crypto import encrypt_field
    assert encrypt_field("") == ""


def test_decrypt_empty_string_passthrough():
    """Empty string should be returned as-is (no decryption)."""
    from utils.crypto import decrypt_field
    assert decrypt_field("") == ""


def test_encrypt_different_plaintext_different_ciphertext():
    from utils.crypto import encrypt_field
    c1 = encrypt_field("value1")
    c2 = encrypt_field("value2")
    assert c1 != c2


def test_encrypt_same_plaintext_different_ciphertext():
    """Fernet uses random IV so same plaintext produces different ciphertext each time."""
    from utils.crypto import encrypt_field
    c1 = encrypt_field("same")
    c2 = encrypt_field("same")
    assert c1 != c2


def test_decrypt_invalid_token_raises():
    """Corrupted ciphertext should raise an exception."""
    from utils.crypto import decrypt_field
    from cryptography.fernet import InvalidToken
    with pytest.raises((InvalidToken, Exception)):
        decrypt_field("not-a-valid-fernet-token")


# ---------------------------------------------------------------------------
# Missing FIELD_ENCRYPTION_KEY
# ---------------------------------------------------------------------------

def test_encrypt_missing_key_raises(monkeypatch):
    monkeypatch.delenv("FIELD_ENCRYPTION_KEY", raising=False)
    # Force reimport to pick up missing env var
    import importlib
    import utils.crypto as crypto_mod
    importlib.reload(crypto_mod)
    with pytest.raises(RuntimeError, match="FIELD_ENCRYPTION_KEY"):
        crypto_mod.encrypt_field("test")
    # Restore for other tests
    os.environ["FIELD_ENCRYPTION_KEY"] = VALID_KEY
    importlib.reload(crypto_mod)


def test_decrypt_missing_key_raises(monkeypatch):
    monkeypatch.delenv("FIELD_ENCRYPTION_KEY", raising=False)
    import importlib
    import utils.crypto as crypto_mod
    importlib.reload(crypto_mod)
    with pytest.raises(RuntimeError, match="FIELD_ENCRYPTION_KEY"):
        crypto_mod.decrypt_field("somevalue")
    os.environ["FIELD_ENCRYPTION_KEY"] = VALID_KEY
    importlib.reload(crypto_mod)
