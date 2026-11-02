#!/usr/bin/env python3
"""Safely adapt the 3x-ui explicit TLS cipher overlay to the newer hiddify fork."""
import pathlib
import subprocess
import sys

TARGETS = (
    "common/tls/std_server.go",
    "common/tls/std_client.go",
    "common/tls/utls_client.go",
)

BEFORE = """\t\tids, legacy, err := resolveCipherSuites(options.CipherSuites)
\t\tif err != nil {
\t\t\treturn nil, err
\t\t}
\t\ttlsConfig.CipherSuites = ids
"""
AFTER = """\t\tids, legacy, err := resolveCipherSuites(options.CipherSuites)
\t\tif err != nil {
\t\t\treturn nil, err
\t\t}
\t\t// Retain the fork's legacy cipher audit while preserving the exact
\t\t// operator-specified order, including duplicates, for panel clients.
\t\tids, err = xuiExplicitCipherSuites(options.CipherSuites)
\t\tif err != nil {
\t\t\treturn nil, err
\t\t}
\t\ttlsConfig.CipherSuites = ids
"""

def main():
    if len(sys.argv) != 3:
        raise SystemExit("usage: rebase-panel-ciphers.py SINGBOX_CHECKOUT CIPHER_PATCH")
    source, patch = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
    if not source.is_dir() or not patch.is_file():
        raise RuntimeError("missing sing-box source or cipher patch")
    # The helper is extracted from the exact pinned panel patch rather than
    # silently inventing a substitute, so drift in that patch is an error.
    patch_data = patch.read_text()
    marker = "diff --git a/common/tls/xui_explicit_ciphers.go b/common/tls/xui_explicit_ciphers.go\\n"
    if patch_data.count(marker) != 1:
        raise RuntimeError("unexpected cipher helper patch")
    tail = patch_data.split(marker, 1)[1]
    if "diff --git " in tail:
        tail = tail.split("diff --git ", 1)[0]
    lines = [line[1:] for line in tail.splitlines() if line.startswith("+") and not line.startswith("+++")]
    helper = "\\n".join(lines) + "\\n"
    if not helper.startswith("package tls\\n") or "func xuiExplicitCipherSuites(" not in helper:
        raise RuntimeError("unrecognized cipher helper source")
    file = source / "common/tls/xui_explicit_ciphers.go"
    if file.exists():
        if file.read_text() != helper:
            raise RuntimeError("unexpected existing explicit cipher helper")
    else:
        file.write_text(helper)
    for path in TARGETS:
        file = source / path
        current = file.read_text()
        if current.count(BEFORE) != 1 or current.count("logger.Warn(\\\"Explicitly enabled legacy TLS cipher suites") != 1:
            raise RuntimeError("upstream TLS cipher implementation changed: " + path)
        file.write_text(current.replace(BEFORE, AFTER))
    subprocess.run(["gofmt", "-w", "common/tls/xui_explicit_ciphers.go", *TARGETS],
                   cwd=source, check=True)
    print("Explicit TLS ciphers rebased against fork resolver; legacy safety warnings retained.")


if __name__ == "__main__":
    main()
