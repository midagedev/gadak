#!/usr/bin/env python3
"""Mint an App Store Connect API token (ES256 JWT) from a .p8 private key.

Standard library only, on purpose: this machine has neither `PyJWT` nor
`cryptography`, and a release script that needs `pip install` before it can
check whether a build processed is a release script that stops working the
week you need it. `openssl` does the signing; the DER→JOSE conversion below is
the only part worth writing by hand.

Usage:
    asc-jwt.py <path-to-AuthKey_XXXX.p8> <key-id> <issuer-id> [lifetime-seconds]

Prints the token on stdout and nothing else — the key id and issuer are
identifiers Apple ties to your account, so they must never reach a log.
"""

import base64
import json
import subprocess
import sys
import time


def b64url(raw: bytes) -> bytes:
    return base64.urlsafe_b64encode(raw).rstrip(b"=")


def der_signature_to_jose(der: bytes) -> bytes:
    """ECDSA signatures come out of `openssl dgst -sign` as DER
    `SEQUENCE { INTEGER r, INTEGER s }`; JOSE wants the raw 32-byte pair.

    The integers are signed, so a leading 0x00 pad appears whenever the high
    bit is set — stripping it and left-padding to 32 is the whole conversion.
    """
    if not der or der[0] != 0x30:
        raise ValueError("not a DER SEQUENCE")
    index = 2
    if der[1] & 0x80:
        index = 2 + (der[1] & 0x7F)

    parts = []
    for _ in range(2):
        if der[index] != 0x02:
            raise ValueError("expected a DER INTEGER")
        length = der[index + 1]
        value = der[index + 2 : index + 2 + length]
        index += 2 + length
        parts.append(value.lstrip(b"\x00").rjust(32, b"\x00"))
    return b"".join(parts)


def main() -> int:
    if len(sys.argv) < 4:
        sys.stderr.write("usage: asc-jwt.py <key.p8> <key-id> <issuer-id> [lifetime]\n")
        return 64

    key_path, key_id, issuer_id = sys.argv[1:4]
    # Apple rejects tokens with a lifetime over 20 minutes.
    lifetime = min(int(sys.argv[4]) if len(sys.argv) > 4 else 1200, 1200)

    issued_at = int(time.time())
    header = {"alg": "ES256", "kid": key_id, "typ": "JWT"}
    payload = {
        "iss": issuer_id,
        "iat": issued_at,
        "exp": issued_at + lifetime,
        "aud": "appstoreconnect-v1",
    }

    def compact(value: dict) -> bytes:
        return b64url(json.dumps(value, separators=(",", ":")).encode())

    signing_input = compact(header) + b"." + compact(payload)
    signed = subprocess.run(
        ["openssl", "dgst", "-sha256", "-sign", key_path],
        input=signing_input,
        capture_output=True,
        check=True,
    )
    token = signing_input + b"." + b64url(der_signature_to_jose(signed.stdout))
    sys.stdout.write(token.decode())
    return 0


if __name__ == "__main__":
    sys.exit(main())
