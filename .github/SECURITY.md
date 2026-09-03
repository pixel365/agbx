# Security Policy

## Supported Versions

We actively maintain and patch security issues for the latest release version of `agbx`.

## Reporting a Vulnerability

If you discover a security vulnerability, please report it **privately**.

You can contact the maintainer directly via email:

📧 **pixel.365.24@gmail.com**

Please include:

- A clear description of the issue
- Steps to reproduce (if applicable)
- Any potential impact

Do **not** create public GitHub issues for security concerns.
We aim to respond to all reports within **72 hours** and will coordinate disclosure responsibly.

## Verifying a Release

Every release archive is signed with GPG and accompanied by a detached
`.sig` file. The `checksums.txt` file contains SHA-256 checksums for release
archives.

Signing key:

```
Fingerprint: 59E2 D7E1 C4AE FA9F 891A  5845 C5EE 0737 09C7 B615
Key ID:      C5EE073709C7B615
Type:        ed25519, created 2025-01-16
```

Import the key, then verify the archive you downloaded:

```bash
gpg --keyserver keys.openpgp.org --recv-keys 59E2D7E1C4AEFA9F891A5845C5EE073709C7B615
gpg --verify agbx_v1.0.0_linux_amd64.tar.gz.sig agbx_v1.0.0_linux_amd64.tar.gz
```

Always compare the fingerprint reported by `gpg` against the one above rather
than trusting the key the keyserver happened to return.

Checksums can be checked on their own:

```bash
sha256sum --ignore-missing -c agbx_1.0.0_checksums.txt
```

Each archive also ships an SPDX SBOM as `<archive>.sbom.json`, listing every
dependency compiled into that binary.
